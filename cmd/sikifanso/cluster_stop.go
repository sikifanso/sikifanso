package main

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/preflight"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func clusterStopCmd() *cli.Command {
	return &cli.Command{
		Name:          "stop",
		Usage:         "Stop a running k3d cluster",
		ArgsUsage:     "[NAME]",
		Action:        clusterStopAction,
		ShellComplete: clusterNameShellComplete,
	}
}

func clusterStopAction(ctx context.Context, cmd *cli.Command) error {
	name := defaultClusterName
	if cmd.Args().Present() {
		name = cmd.Args().First()
	}

	zapLogger.Info("running preflight checks")
	if err := preflight.CheckDocker(ctx); err != nil {
		zapLogger.Error("preflight check failed", zap.Error(err))
		return err
	}
	zapLogger.Info("all preflight checks passed")

	return cluster.Stop(ctx, zapLogger, name)
}
