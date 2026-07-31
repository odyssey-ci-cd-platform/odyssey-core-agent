package runner

import (
	"context"

	"codeberg.org/odyssey/odyssey-core-agent/internal/domain"
)

type Runner interface {
	Run(ctx context.Context, job domain.Job, projectPath string) (domain.JobResult, error)
}
