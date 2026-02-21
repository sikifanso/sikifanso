package main

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func argocdSyncCmd() *cli.Command {
	return &cli.Command{
		Name:   "sync",
		Usage:  "Force immediate ArgoCD reconciliation",
		Action: argocdSyncAction,
	}
}

func argocdSyncAction(ctx context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")

	sess, err := session.Load(clusterName)
	if err != nil {
		zapLogger.Error("failed to load session", zap.String("cluster", clusterName), zap.Error(err))
		return err
	}

	if err := argocd.Sync(ctx, zapLogger, clusterName, sess.Services.ArgoCD.URL); err != nil {
		zapLogger.Error("sync failed", zap.Error(err))
		return err
	}

	zapLogger.Info("argocd sync completed", zap.String("cluster", clusterName))
	return nil
}
