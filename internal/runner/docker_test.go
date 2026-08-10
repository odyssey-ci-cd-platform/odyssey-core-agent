package runner_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/runner"
)

func TestNewDockerRunner(t *testing.T) {
	r, err := runner.NewDockerRunner(nil)
	if err != nil {
		t.Skipf("Docker client not available (this is fine): %v", err)
	}
	if r == nil {
		t.Error("NewDockerRunner() returned nil runner with no error")
	}
}

// requireDocker returns a DockerRunner, skipping the test if Docker
// is not available.
func requireDocker(t *testing.T) *runner.DockerRunner {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	r, err := runner.NewDockerRunner(nil)
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	return r
}

func TestDockerRunnerRunEcho(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "echo-test",
		Image: "alpine:latest",
		Steps: []domain.Step{
			{Name: "hello", Run: "echo hello-world"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	status := result.Status()
	if status != domain.StatusPassed {
		t.Errorf("expected StatusPassed, got %v", status)
	}

	if len(result.StepResults) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.StepResults))
	}

	step := result.StepResults[0]
	if step.ExitCode != domain.ExitSuccess {
		t.Errorf("expected exit success, got %v", step.ExitCode)
	}

	if !strings.Contains(step.Stdout, "hello-world") {
		t.Errorf("expected stdout to contain 'hello-world', got %q", step.Stdout)
	}
}

func TestDockerRunnerRunFailingCommand(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "fail-test",
		Image: "alpine:latest",
		Steps: []domain.Step{
			{Name: "failing step", Run: "exit 42"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	// Run returns the step error; the job itself still completed
	if err == nil {
		t.Log("Run() returned nil error for failing command")
	}

	status := result.Status()
	if status != domain.StatusFailed {
		t.Errorf("expected StatusFailed, got %v", status)
	}

	if len(result.StepResults) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.StepResults))
	}

	step := result.StepResults[0]
	if step.ExitCode != domain.ExitFailure {
		t.Errorf("expected exit failure, got %v", step.ExitCode)
	}
}

func TestDockerRunnerRunWithEnvVars(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "env-test",
		Image: "alpine:latest",
		Env: map[string]string{
			"MY_VAR": "hello-env",
		},
		Steps: []domain.Step{
			{Name: "print env", Run: "echo $MY_VAR"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StepResults) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.StepResults))
	}

	if !strings.Contains(result.StepResults[0].Stdout, "hello-env") {
		t.Errorf("expected stdout to contain 'hello-env', got %q", result.StepResults[0].Stdout)
	}
}

func TestDockerRunnerRunWithSetup(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	// Create a file that the setup will modify
	if err := os.WriteFile(dir+"/input.txt", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	job := domain.Job{
		Name:  "setup-test",
		Image: "alpine:latest",
		Setup: []string{
			"echo 'setup-ran' > /app/setup-marker.txt",
		},
		Steps: []domain.Step{
			{Name: "check setup", Run: "cat /app/setup-marker.txt"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StepResults) != 1 {
		t.Fatalf("expected 1 step result, got %d", len(result.StepResults))
	}

	if !strings.Contains(result.StepResults[0].Stdout, "setup-ran") {
		t.Errorf("expected stdout to contain 'setup-ran', got %q", result.StepResults[0].Stdout)
	}
}

func TestDockerRunnerRunMultipleSteps(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "multi-step-test",
		Image: "alpine:latest",
		Steps: []domain.Step{
			{Name: "step one", Run: "echo one"},
			{Name: "step two", Run: "echo two"},
			{Name: "step three", Run: "echo three"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StepResults) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.StepResults))
	}

	for i, want := range []string{"one", "two", "three"} {
		if !strings.Contains(result.StepResults[i].Stdout, want) {
			t.Errorf("step %d: expected stdout to contain %q, got %q", i+1, want, result.StepResults[i].Stdout)
		}
		if result.StepResults[i].ExitCode != domain.ExitSuccess {
			t.Errorf("step %d: expected exit success, got %v", i+1, result.StepResults[i].ExitCode)
		}
	}
}

func TestDockerRunnerExportsEnvBetweenSteps(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "export-env-test",
		Image: "alpine:latest",
		Steps: []domain.Step{
			{Name: "export", Run: `echo "SHARED=from-step-one" >> "$ODYSSEY_ENV"`},
			{Name: "consume", Run: "echo $SHARED"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(result.StepResults) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.StepResults))
	}
	if got := result.StepResults[1].Stdout; !strings.Contains(got, "from-step-one") {
		t.Errorf("expected second step to see exported var, got %q", got)
	}
}

func TestDockerRunnerExportedEnvOverridesJobEnv(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "override-env-test",
		Image: "alpine:latest",
		Env:   map[string]string{"SHARED": "job-level"},
		Steps: []domain.Step{
			{Name: "before override", Run: "echo $SHARED"},
			{Name: "override", Run: `echo "SHARED=step-level" >> "$ODYSSEY_ENV"`},
			{Name: "after override", Run: "echo $SHARED"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}
	if len(result.StepResults) != 3 {
		t.Fatalf("expected 3 step results, got %d", len(result.StepResults))
	}
	if got := result.StepResults[0].Stdout; !strings.Contains(got, "job-level") {
		t.Errorf("first step: expected job-level value, got %q", got)
	}
	if got := result.StepResults[2].Stdout; !strings.Contains(got, "step-level") {
		t.Errorf("last step: expected exported value to override job env, got %q", got)
	}
}

func TestDockerRunnerRunSetupError(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "bad-setup-test",
		Image: "alpine:latest",
		Setup: []string{
			"exit 1",
		},
		Steps: []domain.Step{
			{Name: "should not run", Run: "echo nope"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err == nil {
		t.Error("Run() expected error for failed setup, got nil")
	}

	if result.SetupErr == nil {
		t.Error("expected SetupErr to be set")
	}

	if result.Status() != domain.StatusErrored {
		t.Errorf("expected StatusErrored, got %v", result.Status())
	}

	if len(result.StepResults) != 0 {
		t.Errorf("expected 0 step results when setup fails, got %d", len(result.StepResults))
	}
}

func TestDockerRunnerRunStepDuration(t *testing.T) {
	r := requireDocker(t)
	dir := t.TempDir()

	job := domain.Job{
		Name:  "duration-test",
		Image: "alpine:latest",
		Steps: []domain.Step{
			{Name: "quick", Run: "echo quick"},
			{Name: "slow", Run: "sleep 1"},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := r.Run(ctx, job, dir)
	if err != nil {
		t.Fatalf("Run() unexpected error: %v", err)
	}

	if len(result.StepResults) != 2 {
		t.Fatalf("expected 2 step results, got %d", len(result.StepResults))
	}

	// Each step should have a non-zero duration
	for _, sr := range result.StepResults {
		if sr.Duration <= 0 {
			t.Errorf("step %q: expected positive duration, got %v", sr.StepName, sr.Duration)
		}
	}

	// The "slow" step should take at least 1 second
	slowDuration := result.StepResults[1].Duration
	if slowDuration < time.Second {
		t.Errorf("slow step duration %v is less than 1s", slowDuration)
	}

	// Job duration should be sum of step durations
	expectedTotal := result.StepResults[0].Duration + result.StepResults[1].Duration
	if result.Duration() != expectedTotal {
		t.Errorf("job Duration() = %v, want %v", result.Duration(), expectedTotal)
	}
}
