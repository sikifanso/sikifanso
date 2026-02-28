package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func catalogCmd() *cli.Command {
	return &cli.Command{
		Name:  "catalog",
		Usage: "Manage curated catalog applications",
		Commands: []*cli.Command{
			catalogListCmd(),
			catalogEnableCmd(),
			catalogDisableCmd(),
		},
	}
}

func catalogListCmd() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List all catalog applications",
		Action: catalogListAction,
	}
}

func catalogEnableCmd() *cli.Command {
	return &cli.Command{
		Name:          "enable",
		Usage:         "Enable a catalog application",
		ArgsUsage:     "NAME",
		Action:        catalogEnableAction,
		ShellComplete: catalogAllNamesComplete,
	}
}

func catalogDisableCmd() *cli.Command {
	return &cli.Command{
		Name:          "disable",
		Usage:         "Disable a catalog application",
		ArgsUsage:     "NAME",
		Action:        catalogDisableAction,
		ShellComplete: catalogEnabledNamesComplete,
	}
}

func catalogListAction(_ context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	entries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing catalog: %w", err)
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "Catalog is empty")
		return nil
	}

	// Build rows for two-pass alignment.
	headers := []string{"NAME", "CATEGORY", "ENABLED", "DESCRIPTION"}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		enabled := "false"
		if e.Enabled {
			enabled = "true"
		}
		rows = append(rows, []string{e.Name, e.Category, enabled, e.Description})
	}

	// Compute max column widths from headers and data.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	// Print header (bold). Pad the plain text first, then wrap in color
	// so ANSI escape codes don't break width calculation.
	bold := color.New(color.Bold).SprintFunc()
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(os.Stderr, "  ")
		}
		padded := fmt.Sprintf("%-*s", widths[i], h)
		fmt.Fprint(os.Stderr, bold(padded))
	}
	fmt.Fprintln(os.Stderr)

	// Print data rows with colored ENABLED column.
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(os.Stderr, "  ")
			}
			padded := fmt.Sprintf("%-*s", widths[i], cell)
			if i == 2 {
				if cell == "true" {
					fmt.Fprint(os.Stderr, green(padded))
				} else {
					fmt.Fprint(os.Stderr, red(padded))
				}
			} else {
				fmt.Fprint(os.Stderr, padded)
			}
		}
		fmt.Fprintln(os.Stderr)
	}

	return nil
}

func catalogEnableAction(ctx context.Context, cmd *cli.Command) error {
	return catalogToggleAction(ctx, cmd, true)
}

func catalogDisableAction(ctx context.Context, cmd *cli.Command) error {
	return catalogToggleAction(ctx, cmd, false)
}

func catalogToggleAction(ctx context.Context, cmd *cli.Command, enable bool) error {
	verb := "enable"
	past := "enabled"
	if !enable {
		verb = "disable"
		past = "disabled"
	}

	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso catalog %s NAME", verb)
	}

	entry, err := catalog.Find(sess.GitOpsPath, name)
	if err != nil {
		return err
	}

	if entry.Enabled == enable {
		fmt.Fprintf(os.Stderr, "%s is already %s\n", name, past)
		return nil
	}

	if err := catalog.SetEnabled(sess.GitOpsPath, name, enable); err != nil {
		zapLogger.Error("failed to set enabled", zap.String("app", name), zap.Bool("enabled", enable), zap.Error(err))
		return fmt.Errorf("setting enabled=%v for %s: %w", enable, name, err)
	}

	commitMsg := fmt.Sprintf("catalog: %s %s", verb, name)
	commitPath := fmt.Sprintf("catalog/%s.yaml", name)
	if err := gitops.Commit(sess.GitOpsPath, commitMsg, commitPath); err != nil {
		zapLogger.Error("failed to commit catalog change", zap.String("app", name), zap.Error(err))
		return fmt.Errorf("committing change: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s %s\n", color.GreenString(name), past)

	zapLogger.Info("triggering argocd sync")
	if err := argocd.Sync(ctx, zapLogger, clusterName, sess.Services.ArgoCD.URL); err != nil {
		zapLogger.Warn("argocd sync failed — app will reconcile on next poll", zap.Error(err))
	}

	return nil
}

// catalogNamesComplete loads the session and catalog, then writes to stdout the
// name of every entry for which filter returns true.
func catalogNamesComplete(_ context.Context, cmd *cli.Command, filter func(catalog.Entry) bool) {
	clusterName := cmd.String("cluster")
	sess, err := session.Load(clusterName)
	if err != nil {
		return
	}

	entries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filter(e) {
			_, _ = fmt.Fprintln(cmd.Root().Writer, e.Name)
		}
	}
}

func catalogAllNamesComplete(ctx context.Context, cmd *cli.Command) {
	catalogNamesComplete(ctx, cmd, func(_ catalog.Entry) bool { return true })
}

func catalogEnabledNamesComplete(ctx context.Context, cmd *cli.Command) {
	catalogNamesComplete(ctx, cmd, func(e catalog.Entry) bool { return e.Enabled })
}
