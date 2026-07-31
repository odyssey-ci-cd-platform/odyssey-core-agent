package orchestrator

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/common"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/domain"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/runner"
	"golang.org/x/sync/errgroup"
)

// Orchestrator executes a Pipeline by running stages sequentially and
// jobs within each stage concurrently.
type Orchestrator struct {
	runner runner.Runner
	logger *slog.Logger
}

// New returns an Orchestrator that delegates job execution to r.
func New(r runner.Runner, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Orchestrator{runner: r, logger: logger}
}

// Run executes every stage in the pipeline. Jobs within a stage run
// concurrently. All stages are executed regardless of failures.
func (o *Orchestrator) Run(ctx context.Context, pipeline domain.Pipeline, projectPath string) (domain.PipelineResult, error) {
	result := domain.PipelineResult{
		PipelineName: pipeline.Name,
		StageResults: make([]domain.StageResult, 0, len(pipeline.Stages)),
	}
	for _, stage := range pipeline.Stages {
		stageResult := o.runStage(ctx, stage, projectPath)
		result.StageResults = append(result.StageResults, stageResult)
	}
	return result, nil
}

// runStage executes all jobs in a stage concurrently and returns the
// aggregated StageResult.
func (o *Orchestrator) runStage(ctx context.Context, stage domain.Stage, projectPath string) domain.StageResult {
	stageLogger := o.logger.With("stage", stage.Name)
	stageLogger.Info("stage started")
	start := time.Now()

	jobResults := make([]domain.JobResult, len(stage.Jobs))
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)

	for i, job := range stage.Jobs {
		g.Go(func() error {
			jobLogger := stageLogger.With("job", job.Name)
			jobCtx := common.ContextWithLogger(ctx, jobLogger)

			jobResult, _ := o.runner.Run(jobCtx, job, projectPath)
			mu.Lock()
			jobResults[i] = jobResult
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	stageResult := domain.StageResult{
		StageName: stage.Name,
		JobResults: jobResults,
	}
	stageLogger.Info("stage finished",
		"status", stageResult.Status().String(),
		"elapsed", time.Since(start).String(),
	)
	return stageResult
}

