package runner

import (
	"context"
	"errors"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
)

// LocalRunner executes jobs on the local machine without a container.
// This is a stub and not yet implemented.
type LocalRunner struct{}

// Run is not yet implemented.
func (r *LocalRunner) Run(ctx context.Context, job domain.Job, projectPath string) (domain.JobResult, error) {
	return domain.JobResult{}, errors.New("local runner not yet implemented")
}
