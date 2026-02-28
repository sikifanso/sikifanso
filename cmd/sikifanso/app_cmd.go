package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/alicanalbayrak/sikifanso/internal/app"
	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/prompt"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func appCmd() *cli.Command {
	return &cli.Command{
		Name:  "app",
		Usage: "Manage GitOps applications",
		Commands: []*cli.Command{
			appAddCmd(),
			appListCmd(),
			appRemoveCmd(),
		},
	}
}

func appAddCmd() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Add a Helm app to the GitOps repo",
		ArgsUsage: "[NAME]",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "repo",
				Usage: "Helm repository URL",
			},
			&cli.StringFlag{
				Name:  "chart",
				Usage: "Helm chart name",
			},
			&cli.StringFlag{
				Name:  "version",
				Usage: "Chart version",
				Value: "*",
			},
			&cli.StringFlag{
				Name:  "namespace",
				Usage: "Target namespace",
			},
		},
		Action: appAddAction,
	}
}

func appListCmd() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List installed apps",
		Action: appListAction,
	}
}

func appRemoveCmd() *cli.Command {
	return &cli.Command{
		Name:          "remove",
		Usage:         "Remove an app from the GitOps repo",
		ArgsUsage:     "NAME",
		Action:        appRemoveAction,
		ShellComplete: appNameShellComplete,
	}
}

func appAddAction(ctx context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	name := cmd.Args().First()
	if name == "" {
		name = prompt.String("App name", "")
	}
	if name == "" {
		return fmt.Errorf("app name is required")
	}

	repoURL := cmd.String("repo")
	if !cmd.IsSet("repo") {
		repoURL = prompt.String("Helm repo URL", "")
	}
	if repoURL == "" {
		return fmt.Errorf("helm repo URL is required")
	}

	chart := cmd.String("chart")
	if !cmd.IsSet("chart") {
		chart = prompt.String("Chart name", name)
	}

	version := cmd.String("version")
	if !cmd.IsSet("version") {
		version = prompt.String("Chart version", "*")
	}

	namespace := cmd.String("namespace")
	if !cmd.IsSet("namespace") {
		namespace = prompt.String("Target namespace", name)
	}

	opts := app.AddOpts{
		GitOpsPath: sess.GitOpsPath,
		Name:       name,
		RepoURL:    repoURL,
		Chart:      chart,
		Version:    version,
		Namespace:  namespace,
	}

	if err := app.Add(opts); err != nil {
		zapLogger.Error("failed to add app", zap.Error(err))
		return err
	}

	fmt.Fprintf(os.Stderr, "%s added to gitops repo\n", color.GreenString(name))

	zapLogger.Info("triggering argocd sync")
	if err := argocd.Sync(ctx, zapLogger, clusterName, sess.Services.ArgoCD.URL); err != nil {
		zapLogger.Warn("argocd sync failed — app will reconcile on next poll", zap.Error(err))
	}

	return nil
}

func appListAction(_ context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	apps, err := app.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing apps: %w", err)
	}

	catalogEntries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing catalog: %w", err)
	}

	hasEnabledCatalog := slices.ContainsFunc(catalogEntries, func(e catalog.Entry) bool {
		return e.Enabled
	})

	if len(apps) == 0 && !hasEnabledCatalog {
		fmt.Fprintln(os.Stderr, "No apps installed — add one with: sikifanso app add")
		return nil
	}

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCHART\tVERSION\tNAMESPACE\tSOURCE")
	for _, a := range apps {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", a.Name, a.Chart, a.Version, a.Namespace, "custom")
	}
	for _, e := range catalogEntries {
		if e.Enabled {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.Name, e.Chart, e.TargetRevision, e.Namespace, "catalog")
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func appRemoveAction(ctx context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso app remove NAME")
	}

	if err := app.Remove(sess.GitOpsPath, name); err != nil {
		zapLogger.Error("failed to remove app", zap.Error(err))
		return err
	}

	fmt.Fprintf(os.Stderr, "%s removed from gitops repo\n", color.GreenString(name))

	zapLogger.Info("triggering argocd sync")
	if err := argocd.Sync(ctx, zapLogger, clusterName, sess.Services.ArgoCD.URL); err != nil {
		zapLogger.Warn("argocd sync failed — app will reconcile on next poll", zap.Error(err))
	}

	return nil
}

func appNameShellComplete(_ context.Context, cmd *cli.Command) {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return
	}

	apps, err := app.List(sess.GitOpsPath)
	if err != nil {
		return
	}
	for _, a := range apps {
		_, _ = fmt.Fprintln(cmd.Root().Writer, a.Name)
	}
}
