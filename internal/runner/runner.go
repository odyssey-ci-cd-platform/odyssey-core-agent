package runner

import (
	"context"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

type Runner interface {
	Run(ctx context.Context, job domain.Job, projectPath string) (domain.JobResult, error)
}
