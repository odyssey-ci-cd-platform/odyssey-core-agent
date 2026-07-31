package domain_test

import (
	"errors"
	"testing"
	"time"

	"codeberg.org/odyssey/odyssey-core-agent/internal/domain"
)

func TestStepResultStatus(t *testing.T) {
	tests := []struct {
		name string
		r    domain.StepResult
		want domain.Status
	}{
		{
			name: "success exit code returns passed",
			r:    domain.StepResult{ExitCode: domain.ExitSuccess},
			want: domain.StatusPassed,
		},
		{
			name: "failure exit code returns failed",
			r:    domain.StepResult{ExitCode: domain.ExitFailure},
			want: domain.StatusFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobResultStatus(t *testing.T) {
	tests := []struct {
		name string
		r    domain.JobResult
		want domain.Status
	}{
		{
			name: "setup error returns errored",
			r:    domain.JobResult{SetupErr: errors.New("container creation failed")},
			want: domain.StatusErrored,
		},
		{
			name: "no steps and no error returns pending",
			r:    domain.JobResult{},
			want: domain.StatusPending,
		},
		{
			name: "all steps passed returns passed",
			r: domain.JobResult{
				StepResults: []domain.StepResult{
					{ExitCode: domain.ExitSuccess},
					{ExitCode: domain.ExitSuccess},
				},
			},
			want: domain.StatusPassed,
		},
		{
			name: "any step failed returns failed",
			r: domain.JobResult{
				StepResults: []domain.StepResult{
					{ExitCode: domain.ExitSuccess},
					{ExitCode: domain.ExitFailure},
				},
			},
			want: domain.StatusFailed,
		},
		{
			name: "first step failed returns failed",
			r: domain.JobResult{
				StepResults: []domain.StepResult{
					{ExitCode: domain.ExitFailure},
					{ExitCode: domain.ExitSuccess},
				},
			},
			want: domain.StatusFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJobResultDuration(t *testing.T) {
	tests := []struct {
		name string
		r    domain.JobResult
		want time.Duration
	}{
		{
			name: "no steps returns zero",
			r:    domain.JobResult{},
			want: 0,
		},
		{
			name: "sums step durations",
			r: domain.JobResult{
				StepResults: []domain.StepResult{
					{Duration: 1 * time.Second},
					{Duration: 2500 * time.Millisecond},
					{Duration: 500 * time.Millisecond},
				},
			},
			want: 4 * time.Second,
		},
		{
			name: "single step returns its duration",
			r: domain.JobResult{
				StepResults: []domain.StepResult{
					{Duration: 3 * time.Second},
				},
			},
			want: 3 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Duration()
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStageResultStatus(t *testing.T) {
	tests := []struct {
		name string
		r    domain.StageResult
		want domain.Status
	}{
		{
			name: "no jobs returns pending",
			r:    domain.StageResult{},
			want: domain.StatusPending,
		},
		{
			name: "all jobs passed returns passed",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}},
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}},
				},
			},
			want: domain.StatusPassed,
		},
		{
			name: "any job failed returns failed over passed",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}},
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitFailure}}},
				},
			},
			want: domain.StatusFailed,
		},
		{
			name: "errored takes precedence over failed",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitFailure}}},
					{SetupErr: errors.New("setup failed")},
				},
			},
			want: domain.StatusErrored,
		},
		{
			name: "errored takes precedence over passed",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}},
					{SetupErr: errors.New("image pull failed")},
				},
			},
			want: domain.StatusErrored,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStageResultDuration(t *testing.T) {
	tests := []struct {
		name string
		r    domain.StageResult
		want time.Duration
	}{
		{
			name: "no jobs returns zero",
			r:    domain.StageResult{},
			want: 0,
		},
		{
			name: "sums job durations",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{Duration: 1 * time.Second}}},
					{StepResults: []domain.StepResult{
						{Duration: 2 * time.Second},
						{Duration: 500 * time.Millisecond},
					}},
				},
			},
			want: 3500 * time.Millisecond,
		},
		{
			name: "single job returns its duration",
			r: domain.StageResult{
				JobResults: []domain.JobResult{
					{StepResults: []domain.StepResult{{Duration: 5 * time.Second}}},
				},
			},
			want: 5 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Duration()
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPipelineResultStatus(t *testing.T) {
	tests := []struct {
		name string
		r    domain.PipelineResult
		want domain.Status
	}{
		{
			name: "no stages returns pending",
			r:    domain.PipelineResult{},
			want: domain.StatusPending,
		},
		{
			name: "all stages passed returns passed",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}}}},
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}}}},
				},
			},
			want: domain.StatusPassed,
		},
		{
			name: "any stage failed returns failed over passed",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}}}},
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitFailure}}}}},
				},
			},
			want: domain.StatusFailed,
		},
		{
			name: "errored takes precedence over failed",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitFailure}}}}},
					{JobResults: []domain.JobResult{{SetupErr: errors.New("setup failed")}}},
				},
			},
			want: domain.StatusErrored,
		},
		{
			name: "errored takes precedence over passed",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{ExitCode: domain.ExitSuccess}}}}},
					{JobResults: []domain.JobResult{{SetupErr: errors.New("image pull failed")}}},
				},
			},
			want: domain.StatusErrored,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Status()
			if got != tt.want {
				t.Errorf("Status() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPipelineResultDuration(t *testing.T) {
	tests := []struct {
		name string
		r    domain.PipelineResult
		want time.Duration
	}{
		{
			name: "no stages returns zero",
			r:    domain.PipelineResult{},
			want: 0,
		},
		{
			name: "sums stage durations",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{Duration: 1 * time.Second}}}}},
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{
						{Duration: 2 * time.Second},
						{Duration: 1500 * time.Millisecond},
					}}}},
				},
			},
			want: 4500 * time.Millisecond,
		},
		{
			name: "single stage returns its duration",
			r: domain.PipelineResult{
				StageResults: []domain.StageResult{
					{JobResults: []domain.JobResult{{StepResults: []domain.StepResult{{Duration: 10 * time.Second}}}}},
				},
			},
			want: 10 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.r.Duration()
			if got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}
