package main

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func clusterInfoCmd() *cli.Command {
	return &cli.Command{
		Name:          "info",
		Usage:         "Show cluster access info and service details",
		ArgsUsage:     "[NAME]",
		Action:        clusterInfoAction,
		ShellComplete: clusterNameShellComplete,
	}
}

func clusterInfoAction(ctx context.Context, cmd *cli.Command) error {
	// If a name is given, show only that cluster.
	if cmd.Args().Present() {
		return showClusterInfo(ctx, cmd, cmd.Args().First())
	}

	// No name — show all clusters.
	sessions, err := session.ListAll()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		_, _ = fmt.Fprintln(cmd.Root().Writer, "no clusters found — create one with: sikifanso cluster create")
		return nil
	}

	for _, sess := range sessions {
		live, err := cluster.Exists(ctx, sess.ClusterName)
		if err != nil {
			zapLogger.Warn("could not check cluster status", zap.String("cluster", sess.ClusterName), zap.Error(err))
		} else if !live {
			_, _ = fmt.Fprintf(cmd.Root().Writer, "warning: cluster %q is not currently running\n", sess.ClusterName)
		}
		printClusterInfo(sess)
	}
	return nil
}

func showClusterInfo(ctx context.Context, cmd *cli.Command, name string) error {
	sess, err := session.Load(name)
	if err != nil {
		zapLogger.Error("failed to load session", zap.String("cluster", name), zap.Error(err))
		return fmt.Errorf("no session found for cluster %q — was it created with sikifanso?", name)
	}

	live, err := cluster.Exists(ctx, name)
	if err != nil {
		zapLogger.Warn("could not check cluster status", zap.Error(err))
	} else if !live {
		_, _ = fmt.Fprintf(cmd.Root().Writer, "warning: cluster %q is not currently running\n", name)
	}

	printClusterInfo(sess)
	return nil
}
