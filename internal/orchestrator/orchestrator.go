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

// EventSink publishes lifecycle events to the event bus. Nil means the
// event bus is disabled; the engine stays runnable without Redis (ADR 0001).
type EventSink interface {
	Publish(ctx context.Context, event domain.Event) error
}

// Orchestrator executes a Pipeline by running stages sequentially and
// jobs within each stage concurrently.
type Orchestrator struct {
	runner runner.Runner
	sink   EventSink
	logger *slog.Logger
}

// New returns an Orchestrator that delegates job execution to r and emits
// lifecycle events to sink (may be nil to disable emission).
func New(r runner.Runner, sink EventSink, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Orchestrator{runner: r, sink: sink, logger: logger}
}

// emit publishes one lifecycle event. Emission failures are logged, never
// fatal — pipeline execution proceeds unaffected (ADR 0001).
func (o *Orchestrator) emit(ctx context.Context, event domain.Event) {
	if o.sink == nil {
		return
	}
	if err := o.sink.Publish(ctx, event); err != nil {
		o.logger.Warn("event emit failed", "type", event.Type, "pipeline", event.Pipeline, "err", err)
	}
}

// stepSink returns the sink handed to the runner: nil when the bus is
// disabled, otherwise a view stamping runner-built events with the pipeline
// name before emission — the runner knows steps, not which pipeline it
// serves (ADR 0001: envelopes carry the pipeline).
func (o *Orchestrator) stepSink(pipelineName string) runner.StepSink {
	if o.sink == nil {
		return nil
	}
	return &stampedSink{sink: o.sink, pipeline: pipelineName}
}

// stampedSink adapts the orchestrator's EventSink to runner.StepSink,
// filling in the pipeline name on every event that passes through.
type stampedSink struct {
	sink     EventSink
	pipeline string
}

func (s *stampedSink) Publish(ctx context.Context, event domain.Event) error {
	event.Pipeline = s.pipeline
	return s.sink.Publish(ctx, event)
}

// Run executes every stage in the pipeline. Jobs within a stage run
// concurrently. All stages are executed regardless of failures.
func (o *Orchestrator) Run(ctx context.Context, pipeline domain.Pipeline, projectPath string) (domain.PipelineResult, error) {
	result := domain.PipelineResult{
		PipelineName: pipeline.Name,
		StageResults: make([]domain.StageResult, 0, len(pipeline.Stages)),
	}
	o.emit(ctx, domain.Event{Type: domain.EventPipelineStarted, OccurredAt: time.Now(), Pipeline: pipeline.Name})
	for _, stage := range pipeline.Stages {
		stageResult := o.runStage(ctx, stage, pipeline.Name, projectPath)
		result.StageResults = append(result.StageResults, stageResult)
	}
	o.emit(ctx, domain.Event{
		Type:       domain.EventPipelineFinished,
		OccurredAt: time.Now(),
		Pipeline:   pipeline.Name,
		Payload:    map[string]string{"status": result.Status().String()},
	})
	return result, nil
}

// runStage executes all jobs in a stage concurrently and returns the
// aggregated StageResult.
func (o *Orchestrator) runStage(ctx context.Context, stage domain.Stage, pipelineName string, projectPath string) domain.StageResult {
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

			o.emit(jobCtx, domain.Event{Type: domain.EventJobStarted, OccurredAt: time.Now(), Pipeline: pipelineName, Job: job.Name})
			jobResult, err := o.runner.Run(jobCtx, job, projectPath, o.stepSink(pipelineName))
			o.emit(jobCtx, domain.Event{
				Type:       domain.EventJobFinished,
				OccurredAt: time.Now(),
				Pipeline:   pipelineName,
				Job:        job.Name,
				Payload:    map[string]string{"status": jobResult.Status().String()},
			})
			if err != nil {
				jobLogger.Error("job failed",
					"job", job.Name,
					"stepCount", len(jobResult.StepResults),
					"error", err,
				)
			}
			mu.Lock()
			jobResults[i] = jobResult
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	stageResult := domain.StageResult{
		StageName:  stage.Name,
		JobResults: jobResults,
	}
	stageLogger.Info("stage finished",
		"status", stageResult.Status().String(),
		"elapsed", time.Since(start).String(),
	)
	return stageResult
}
