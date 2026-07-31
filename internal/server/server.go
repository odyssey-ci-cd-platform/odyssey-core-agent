package server

import (
	"context"
	"log/slog"

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
// Logger is the root logger passed down to the orchestrator and runner.
// When nil, a no-op logger is used.
type Server struct {
	odysseyv1.UnimplementedOdysseyServiceServer
	Runner runner.Runner
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
	orch := orchestrator.New(r, logger)

	pipelineResult, _ := orch.Run(ctx, pipeline, request.ProjectPath)
	response := domainPipelineResultToProto(pipelineResult)

	logger.Info("pipeline finished",
		"pipeline", response.PipelineName,
		"status", response.Status.String(),
	)
	return &response, nil
}

