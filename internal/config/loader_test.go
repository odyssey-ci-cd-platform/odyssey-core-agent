package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/config"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

// writeOdysseyConfig creates a .odyssey directory under dir, writes
// pipeline.toml with content, and optionally writes env.toml if envContent
// is non-empty.
func writeOdysseyConfig(t *testing.T, dir, pipelineContent, envContent string) {
	t.Helper()
	odysseyDir := filepath.Join(dir, ".odyssey")
	if err := os.MkdirAll(odysseyDir, 0o755); err != nil {
		t.Fatalf("failed to create .odyssey dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(odysseyDir, "pipeline.toml"), []byte(pipelineContent), 0o644); err != nil {
		t.Fatalf("failed to write pipeline.toml: %v", err)
	}
	if envContent != "" {
		if err := os.WriteFile(filepath.Join(odysseyDir, "env.toml"), []byte(envContent), 0o644); err != nil {
			t.Fatalf("failed to write env.toml: %v", err)
		}
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name         string
		pipelineTOML string
		envTOML      string
		want         domain.Pipeline
		wantErr      bool
		errContains  string
	}{
		{
			name: "valid minimal pipeline",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "run tests", run = "go test ./..." },
]
`),
			want: domain.Pipeline{
				Name: "ci",
				Stages: []domain.Stage{
					{
						Name: "test",
						Jobs: []domain.Job{
							{
								Name:  "unit",
								Image: "alpine:latest",
								Env:   map[string]string{},
								Steps: []domain.Step{
									{Name: "run tests", Run: "go test ./..."},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "valid multi-stage pipeline",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "full-ci"
stages = ["build", "test", "deploy"]

[jobs.compile]
name = "compile"
stage = "build"
image = "golang:1.21"
steps = [
  { name = "build binary", run = "go build -o app ./cmd/server" },
]

[jobs.unit-tests]
name = "unit-tests"
stage = "test"
image = "golang:1.21"
setup = ["go mod download"]
env = { DB_HOST = "localhost" }
steps = [
  { name = "unit tests", run = "go test ./..." },
  { name = "race tests", run = "go test -race ./..." },
]

[jobs.deploy-app]
name = "deploy-app"
stage = "deploy"
image = "alpine:latest"
steps = [
  { name = "deploy", run = "echo deployed" },
]
`),
			want: domain.Pipeline{
				Name: "full-ci",
				Stages: []domain.Stage{
					{
						Name: "build",
						Jobs: []domain.Job{
							{
								Name:  "compile",
								Image: "golang:1.21",
								Env:   map[string]string{},
								Steps: []domain.Step{
									{Name: "build binary", Run: "go build -o app ./cmd/server"},
								},
							},
						},
					},
					{
						Name: "test",
						Jobs: []domain.Job{
							{
								Name:  "unit-tests",
								Image: "golang:1.21",
								Setup: []string{"go mod download"},
								Env:   map[string]string{"DB_HOST": "localhost"},
								Steps: []domain.Step{
									{Name: "unit tests", Run: "go test ./..."},
									{Name: "race tests", Run: "go test -race ./..."},
								},
							},
						},
					},
					{
						Name: "deploy",
						Jobs: []domain.Job{
							{
								Name:  "deploy-app",
								Image: "alpine:latest",
								Env:   map[string]string{},
								Steps: []domain.Step{
									{Name: "deploy", Run: "echo deployed"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "env merging - job overrides global",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
env = { FOO = "job-value", BAR = "from-job" }
steps = [
  { name = "step", run = "echo hello" },
]
`),
			envTOML: strings.TrimSpace(`
[env]
FOO = "global-value"
BAZ = "from-global"
`),
			want: domain.Pipeline{
				Name: "ci",
				Stages: []domain.Stage{
					{
						Name: "test",
						Jobs: []domain.Job{
							{
								Name:  "unit",
								Image: "alpine:latest",
								Env: map[string]string{
									"FOO": "job-value",
									"BAR": "from-job",
									"BAZ": "from-global",
								},
								Steps: []domain.Step{
									{Name: "step", Run: "echo hello"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "env.toml only - no job env",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			envTOML: strings.TrimSpace(`
[env]
SHARED_SECRET = "secret123"
`),
			want: domain.Pipeline{
				Name: "ci",
				Stages: []domain.Stage{
					{
						Name: "test",
						Jobs: []domain.Job{
							{
								Name:  "unit",
								Image: "alpine:latest",
								Env:   map[string]string{"SHARED_SECRET": "secret123"},
								Steps: []domain.Step{
									{Name: "step", Run: "echo hello"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "without env.toml - jobs work fine",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			want: domain.Pipeline{
				Name: "ci",
				Stages: []domain.Stage{
					{
						Name: "test",
						Jobs: []domain.Job{
							{
								Name:  "unit",
								Image: "alpine:latest",
								Env:   map[string]string{},
								Steps: []domain.Step{
									{Name: "step", Run: "echo hello"},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "empty stages in pipeline",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = []

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: "at least one stage",
		},
		{
			name: "duplicate stage names",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test", "test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: `duplicate stage name: "test"`,
		},
		{
			name: "no jobs defined",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]
`),
			wantErr:     true,
			errContains: "at lease one job",
		},
		{
			name: "job references undeclared stage",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["build"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: `stage "test" is not declared`,
		},
		{
			name: "job with empty stage",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = ""
image = "alpine:latest"
steps = [
  { name = "step", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: `stage must not be empty`,
		},
		{
			name: "job with empty image",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = ""
steps = [
  { name = "step", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: `image must not be empty`,
		},
		{
			name: "job with no steps",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = []
`),
			wantErr:     true,
			errContains: `must define at lease one step`,
		},
		{
			name: "step with empty name",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "", run = "echo hello" },
]
`),
			wantErr:     true,
			errContains: `name must not be empty`,
		},
		{
			name: "step with empty run command",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = "alpine:latest"
steps = [
  { name = "step", run = "" },
]
`),
			wantErr:     true,
			errContains: `run must not be empty`,
		},
		{
			name:         "missing pipeline.toml",
			pipelineTOML: "",
			wantErr:      true,
			errContains:  "failed to read pipeline.toml",
		},
		{
			name:         "invalid TOML syntax",
			pipelineTOML: `this is not valid toml {{{`,
			wantErr:      true,
			errContains:  "failed to read pipeline.toml",
		},
		{
			name: "multiple validation errors reported together",
			pipelineTOML: strings.TrimSpace(`
[pipeline]
name = "ci"
stages = ["test"]

[jobs.unit]
name = "unit"
stage = "test"
image = ""
steps = []
`),
			wantErr:     true,
			errContains: "image must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// For the missing file case, don't create anything
			if tt.pipelineTOML != "" {
				writeOdysseyConfig(t, dir, tt.pipelineTOML, tt.envTOML)
			}

			got, err := config.Load(dir)
			if err != nil {
				if !tt.wantErr {
					t.Errorf("Load() unexpected error: %v", err)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Load() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Load() succeeded unexpectedly")
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
