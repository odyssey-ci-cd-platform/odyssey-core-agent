package config

import (
	"errors"
	"fmt"
)

// StepConfig represents a single step in a job as defined in pipeline.toml.
type StepConfig struct {
	Name string `toml:"name"`
	Run  string `toml:"run"`
}

// JobConfig represents a job as defined in pipeline.toml.
type JobConfig struct {
	Stage string            `toml:"stage"`
	Image string            `toml:"image"`
	Setup []string          `toml:"setup"`
	Env   map[string]string `toml:"env"`
	Steps []StepConfig      `toml:"steps"`
}

// PipelineConfig represents the [pipeline] table in pipeline.toml.
type PipelineConfig struct {
	Name   string   `toml:"name"`
	Stages []string `toml:"stages"`
}

// EnvConfig represents the [env] table in env.toml
type EnvConfig struct {
	Env map[string]string `toml:"env"`
}

// RootConfig is the top-level struct the TOML parser deserializes pipeline.toml into.
type RootConfig struct {
	Pipeline PipelineConfig       `toml:"pipeline"`
	Jobs     map[string]JobConfig `toml:"jobs"`
}

// Validate checks the RootConfig for all errors and returns them joined.
func (root RootConfig) Validate() error {
	var errs []error

	// Pipeline-level checks
	if len(root.Pipeline.Stages) == 0 {
		errs = append(errs, errors.New("pipeline must declare at least one stage"))
	}

	seen := make(map[string]bool)
	for _, stage := range root.Pipeline.Stages {
		if seen[stage] {
			errs = append(errs, fmt.Errorf("duplicate stage name: %q", stage))
		}
		seen[stage] = true
	}

	// Job-level checks
	if len(root.Jobs) == 0 {
		errs = append(errs, errors.New("pipeline must define at lease one job"))
	}

	for jobName, job := range root.Jobs {
		if job.Stage == "" {
			errs = append(errs, fmt.Errorf("job %q: stage must not be empty", jobName))
		} else if !seen[job.Stage] {
			errs = append(errs, fmt.Errorf("job %q: stage %q is not declared in pipeline.stages", jobName, job.Stage))
		}
		if job.Image == "" {
			errs = append(errs, fmt.Errorf("job %q: image must not be empty", jobName))
		}
		if len(job.Steps) == 0 {
			errs = append(errs, fmt.Errorf("job %q: must define at lease one step", jobName))
		}
		for stepNumber, step := range job.Steps {
			if step.Name == "" {
				errs = append(errs, fmt.Errorf("job %q: step %d: name must not be empty", jobName, stepNumber+1))
			}
			if step.Run == "" {
				errs = append(errs, fmt.Errorf("job %q: step %d (%q): run must not be empty", jobName, stepNumber+1, step.Name))
			}
		}
	}
	return errors.Join(errs...)
}
