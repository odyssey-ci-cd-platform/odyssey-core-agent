package runner

import (
	"context"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

type Runner interface {
	Run(ctx context.Context, job domain.Job, projectPath string, events StepSink) (domain.JobResult, error)
}

// StepSink receives step lifecycle events emitted from inside job
// execution. Nil means event emission is disabled; emission failures are
// logged by the runner and never fail the job (ADR 0001).
type StepSink interface {
	Publish(ctx context.Context, event domain.Event) error
}
