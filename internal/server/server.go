package server

import (
	"context"
	"log/slog"
	"os"

	odysseyv1 "bitbucket.org/odyssey-ci/odyssey-core-agent/gen/proto/v1"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/config"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/orchestrator"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/runner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements the gRPC OdysseyService by wiring together the
// config loader, runner, and orchestrator.
//
// The Runner field may be set to inject a runner implementation for
// testing. When nil, RunPipeline creates a DockerRunner.
//
// Events is the event sink lifecycle events are emitted to. When nil,
// runs emit nothing (ADR 0001: the engine stays runnable without Redis).
//
// Logger is the root logger passed down to the orchestrator and runner.
// When nil, a no-op logger is used.
type Server struct {
	odysseyv1.UnimplementedOdysseyServiceServer
	Runner runner.Runner
	Events orchestrator.EventSink
	Logger *slog.Logger
}

func (s Server) logger() *slog.Logger {
	if s.Logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Logger
}

// RunPipeline loads the pipeline config from the .odyssey/ directory at
// request.ProjectPath, executes every stage and job, and returns the
// full result tree. Config load failures return InvalidArgument; runner
// creation failures return Internal.
func (s Server) RunPipeline(ctx context.Context, request *odysseyv1.RunPipelineRequest) (*odysseyv1.RunPipelineResponse, error) {
	logger := s.logger().With("project_path", request.ProjectPath)

	// When a project root is configured, every request must stay under it;
	// without one the server is a local-development tool (AUD-005).
	if root := os.Getenv("ODYSSEY_PROJECT_ROOT"); root != "" && !projectPathAllowed(root, request.ProjectPath) {
		logger.Error("project path outside the configured project root", "root", root)
		return &odysseyv1.RunPipelineResponse{}, status.Errorf(codes.InvalidArgument, "project_path %q is outside the configured project root", request.ProjectPath)
	}

	pipeline, err := config.Load(request.ProjectPath)
	if err != nil {
		logger.Error("config load failed", "error", err)
		return &odysseyv1.RunPipelineResponse{}, status.Errorf(codes.InvalidArgument, "failed to load config: %v", err)
	}

	r := s.Runner
	if r == nil {
		dockerRunner, err := runner.NewDockerRunner(logger)
		if err != nil {
			logger.Error("runner creation failed", "error", err)
			return &odysseyv1.RunPipelineResponse{}, status.Errorf(codes.Internal, "could not instantiate runner: %v", err)
		}
		r = dockerRunner
	}

	logger.Info("pipeline started", "pipeline", pipeline.Name)
	orch := orchestrator.New(r, s.Events, logger)

	pipelineResult := orch.Run(ctx, pipeline, request.ProjectPath)
	response := domainPipelineResultToProto(pipelineResult)

	logger.Info("pipeline finished",
		"pipeline", response.PipelineName,
		"status", response.Status.String(),
	)
	return &response, nil
}
