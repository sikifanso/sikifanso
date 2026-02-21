package main

import (
	"context"
	"fmt"
	"runtime"

	"github.com/alicanalbayrak/sikifanso/internal/logger"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	zapLogger  *zap.Logger
	logCleanup func()
)

// setupAction initializes the logger and logs startup info.
// It runs as a Before hook so --help works without Docker.
func setupAction(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	var consoleLevel zapcore.Level
	if err := consoleLevel.UnmarshalText([]byte(cmd.String("log-level"))); err != nil {
		return ctx, fmt.Errorf("invalid log level %q: %w", cmd.String("log-level"), err)
	}

	var err error
	zapLogger, logCleanup, err = logger.New(cmd.String("log-file"), consoleLevel)
	if err != nil {
		return ctx, err
	}
	logger.RedirectLogrus(zapLogger)

	zapLogger.Info("starting sikifanso",
		zap.String("version", version),
		zap.String("commit", commit),
		zap.String("date", date),
		zap.String("go", runtime.Version()),
		zap.String("os", runtime.GOOS),
		zap.String("arch", runtime.GOARCH),
	)

	return ctx, nil
}
