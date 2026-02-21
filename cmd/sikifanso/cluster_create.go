package main

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/preflight"
	"github.com/alicanalbayrak/sikifanso/internal/prompt"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func clusterCreateCmd() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Create a new k3d cluster",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Cluster name",
				Value: defaultClusterName,
			},
			&cli.StringFlag{
				Name:  "bootstrap",
				Usage: "Bootstrap template repo URL",
				Value: gitops.DefaultBootstrapURL,
			},
		},
		Action: clusterCreateAction,
	}
}

func clusterCreateAction(ctx context.Context, cmd *cli.Command) error {
	name := cmd.String("name")
	if !cmd.IsSet("name") {
		name = prompt.String("Cluster name", defaultClusterName)
	}

	bootstrap := cmd.String("bootstrap")
	if !cmd.IsSet("bootstrap") {
		bootstrap = prompt.String("Bootstrap repo", gitops.DefaultBootstrapURL)
	}

	zapLogger.Info("running preflight checks")
	if err := preflight.CheckDocker(ctx); err != nil {
		zapLogger.Error("preflight check failed", zap.Error(err))
		return err
	}
	zapLogger.Info("all preflight checks passed")

	sess, err := cluster.Create(ctx, zapLogger, name, cluster.Options{
		BootstrapURL: bootstrap,
	})
	if err != nil {
		zapLogger.Error("cluster creation failed", zap.Error(err))
		return err
	}

	printClusterInfo(sess)
	return nil
}
