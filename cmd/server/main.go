package main

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"

	odysseyv1 "bitbucket.org/odyssey-ci/odyssey-core-agent/gen/proto/v1"
	"bitbucket.org/odyssey-ci/odyssey-core-agent/internal/server"
)

func main() {
	logger := newLogger()

	if err := godotenv.Load(); err != nil {
		logger.Warn(".env file not found, skipping", "error", err)
	}

	addr := ":50051"
	if v := os.Getenv("ODYSSEY_ADDR"); v != "" {
		addr = fmt.Sprintf(":%s", v)
	}

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", "addr", addr, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	odysseyv1.RegisterOdysseyServiceServer(grpcServer, &server.Server{Logger: logger})

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("shutting down", "signal", sig.String())
		grpcServer.GracefulStop()
	}()

	logger.Info("server listening", "addr", addr)
	if err := grpcServer.Serve(lis); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// newLogger returns a structured logger whose format and level are
// controlled by ODYSSEY_ENV and ODYSSEY_LOG_FORMAT.
//
//	ODYSSEY_ENV=production  → JSON, Info  level
//	otherwise               → Text, Debug level
//
// ODYSSEY_LOG_FORMAT overrides the format: "text" or "json".
func newLogger() *slog.Logger {
	level := slog.LevelDebug
	if os.Getenv("ODYSSEY_ENV") == "production" {
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}

	if logFormat := os.Getenv("ODYSSEY_LOG_FORMAT"); logFormat == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	} else {
		return slog.New(slog.NewTextHandler(os.Stderr, opts))
	}
}
