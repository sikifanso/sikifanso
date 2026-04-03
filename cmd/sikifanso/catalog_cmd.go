package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
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
		Action: withSession(catalogListAction),
	}
}

func catalogEnableCmd() *cli.Command {
	return &cli.Command{
		Name:      "enable",
		Usage:     "Enable a catalog application",
		ArgsUsage: "NAME",
		Flags:     waitSyncFlags(),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			return catalogToggleAction(ctx, cmd, sess, true)
		}),
		ShellComplete: catalogAllNamesComplete,
	}
}

func catalogDisableCmd() *cli.Command {
	return &cli.Command{
		Name:      "disable",
		Usage:     "Disable a catalog application",
		ArgsUsage: "NAME",
		Flags:     waitSyncFlags(),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			return catalogToggleAction(ctx, cmd, sess, false)
		}),
		ShellComplete: catalogEnabledNamesComplete,
	}
}

func catalogListAction(_ context.Context, cmd *cli.Command, sess *session.Session) error {
	entries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing catalog: %w", err)
	}
	if entries == nil {
		entries = []catalog.Entry{}
	}
	if outputJSON(cmd, entries) {
		return nil
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "Catalog is empty")
		return nil
	}

	headers := []string{"NAME", "CATEGORY", "ENABLED", "DESCRIPTION"}
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		enabled := "false"
		if e.Enabled {
			enabled = "true"
		}
		rows = append(rows, []string{e.Name, e.Category, enabled, e.Description})
	}
	printTable(os.Stderr, headers, rows)
	return nil
}

func catalogToggleAction(ctx context.Context, cmd *cli.Command, sess *session.Session, enable bool) error {
	verb := "enable"
	past := "enabled"
	if !enable {
		verb = "disable"
		past = "disabled"
	}

	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso catalog %s NAME", verb)
	}

	result, err := catalog.Toggle(sess.GitOpsPath, name, enable)
	if err != nil {
		return err
	}
	if result.NoChange {
		fmt.Fprintf(os.Stderr, "%s is already %s\n", name, past)
		return nil
	}

	fmt.Fprintf(os.Stderr, "%s  committed to gitops repo\n", name)

	op := grpcsync.OpEnable
	if !enable {
		op = grpcsync.OpDisable
	}
	if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
		Operation:  op,
		Apps:       []string{name},
		AppSetName: "catalog",
	}); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s %s ✓\n", color.GreenString(name), past)
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
