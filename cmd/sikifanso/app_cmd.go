package main

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/app"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/prompt"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/alicanalbayrak/sikifanso/internal/tui"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"golang.org/x/term"
)

func appCmd() *cli.Command {
	return &cli.Command{
		Name:  "app",
		Usage: "Manage applications (catalog and custom Helm charts)",
		Commands: []*cli.Command{
			appAddCmd(),
			appListCmd(),
			appRemoveCmd(),
			appEnableCmd(),
			appDisableCmd(),
			appSyncCmd(),
			appStatusCmd(),
			appDiffCmd(),
			appLogsCmd(),
			appRollbackCmd(),
		},
	}
}

func appAddCmd() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Add a Helm app to the GitOps repo",
		ArgsUsage: "[NAME]",
		Flags: append([]cli.Flag{
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
		}, waitSyncFlags()...),
		Action: withSession(appAddAction),
	}
}

func appListCmd() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List installed apps",
		Action: withSession(appListAction),
	}
}

func appRemoveCmd() *cli.Command {
	return &cli.Command{
		Name:          "remove",
		Usage:         "Remove an app from the GitOps repo",
		ArgsUsage:     "NAME",
		Flags:         waitSyncFlags(),
		Action:        withSession(appRemoveAction),
		ShellComplete: appNameShellComplete,
	}
}

func isTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func appAddAction(ctx context.Context, cmd *cli.Command, sess *session.Session) error { //nolint:gocyclo // interactive flow with multiple input paths
	// If no args and no flags set and stdin is a TTY, launch interactive catalog browser.
	if cmd.Args().Len() == 0 && !cmd.IsSet("repo") && !cmd.IsSet("chart") && isTerminal() {
		entries, err := catalog.List(sess.GitOpsPath)
		if err != nil {
			return fmt.Errorf("listing catalog: %w", err)
		}
		if len(entries) == 0 {
			fmt.Fprintln(os.Stderr, "Catalog is empty — use 'sikifanso app add NAME' to add a custom app")
			return nil
		}

		toggled, err := tui.Browse(tui.BrowseOpts{Entries: entries})
		if err != nil {
			return fmt.Errorf("catalog browser: %w", err)
		}

		if len(toggled) == 0 {
			return nil
		}

		// Apply toggled changes to disk and collect paths for commit.
		var paths, names []string
		for name, enabled := range toggled {
			if err := catalog.SetEnabled(sess.GitOpsPath, name, enabled); err != nil {
				return fmt.Errorf("setting %s enabled=%v: %w", name, enabled, err)
			}
			paths = append(paths, fmt.Sprintf("catalog/%s.yaml", name))
			names = append(names, name)
		}
		sort.Strings(names)

		commitMsg := fmt.Sprintf("catalog: toggle %s", strings.Join(names, ", "))
		if err := gitops.Commit(sess.GitOpsPath, commitMsg, paths...); err != nil {
			zapLogger.Error("failed to commit catalog changes", zap.Error(err))
			return fmt.Errorf("committing changes: %w", err)
		}

		fmt.Fprintf(os.Stderr, "%s toggled: %s\n", color.GreenString("catalog"), strings.Join(names, ", "))
		fmt.Fprintln(os.Stderr, "committed to gitops repo")

		if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
			Operation:  grpcsync.OpEnable,
			Apps:       names,
			AppSetName: "catalog",
		}); err != nil {
			return err
		}
		return nil
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

	if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
		Operation:  grpcsync.OpEnable,
		Apps:       []string{name},
		AppSetName: "root",
	}); err != nil {
		return err
	}

	return nil
}

type appListItem struct {
	Name      string `json:"name"`
	Chart     string `json:"chart"`
	Version   string `json:"version"`
	Namespace string `json:"namespace"`
	Source    string `json:"source"`
}

func appListAction(_ context.Context, cmd *cli.Command, sess *session.Session) error {
	apps, err := app.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing apps: %w", err)
	}

	catalogEntries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing catalog: %w", err)
	}

	if cmd.String("output") == outputFormatJSON {
		items := make([]appListItem, 0, len(apps)+len(catalogEntries))
		for _, a := range apps {
			items = append(items, appListItem{Name: a.Name, Chart: a.Chart, Version: a.Version, Namespace: a.Namespace, Source: "custom"})
		}
		for _, e := range catalogEntries {
			if e.Enabled {
				items = append(items, appListItem{Name: e.Name, Chart: e.Chart, Version: e.TargetRevision, Namespace: e.Namespace, Source: "catalog"})
			}
		}
		outputJSON(cmd, items)
		return nil
	}

	hasEnabledCatalog := slices.ContainsFunc(catalogEntries, func(e catalog.Entry) bool {
		return e.Enabled
	})

	if len(apps) == 0 && !hasEnabledCatalog {
		fmt.Fprintln(os.Stderr, "No apps installed — add one with: sikifanso app add")
		return nil
	}

	headers := []string{"NAME", "CHART", "VERSION", "NAMESPACE", "SOURCE"}
	rows := make([][]string, 0, len(apps)+len(catalogEntries))
	for _, a := range apps {
		rows = append(rows, []string{a.Name, a.Chart, a.Version, a.Namespace, "custom"})
	}
	for _, e := range catalogEntries {
		if e.Enabled {
			rows = append(rows, []string{e.Name, e.Chart, e.TargetRevision, e.Namespace, "catalog"})
		}
	}
	printTable(os.Stderr, headers, rows)
	return nil
}

func appRemoveAction(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso app remove NAME")
	}

	if err := app.Remove(sess.GitOpsPath, name); err != nil {
		zapLogger.Error("failed to remove app", zap.Error(err))
		return err
	}

	fmt.Fprintf(os.Stderr, "%s removed from gitops repo\n", color.GreenString(name))

	if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
		Operation:  grpcsync.OpDisable,
		Apps:       []string{name},
		AppSetName: "root",
	}); err != nil {
		return err
	}

	return nil
}

func appEnableCmd() *cli.Command {
	return &cli.Command{
		Name:      "enable",
		Usage:     "Enable a catalog application",
		ArgsUsage: "NAME",
		Flags:     waitSyncFlags(),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			return appToggleAction(ctx, cmd, sess, true)
		}),
		ShellComplete: catalogDisabledNamesComplete,
	}
}

func appDisableCmd() *cli.Command {
	return &cli.Command{
		Name:      "disable",
		Usage:     "Disable a catalog application",
		ArgsUsage: "NAME",
		Flags:     waitSyncFlags(),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			return appToggleAction(ctx, cmd, sess, false)
		}),
		ShellComplete: catalogEnabledNamesComplete,
	}
}

func appToggleAction(ctx context.Context, cmd *cli.Command, sess *session.Session, enable bool) error {
	verb := "enable"
	past := "enabled"
	if !enable {
		verb = "disable"
		past = "disabled"
	}

	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso app %s NAME", verb)
	}

	result, err := catalog.Toggle(sess.GitOpsPath, name, enable)
	if err != nil {
		return err
	}
	if result.NoChange {
		fmt.Fprintf(os.Stderr, "%s is already %s\n", name, past)
		return nil
	}

	fmt.Fprintf(os.Stderr, "%s committed to gitops repo\n", name)

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

func catalogDisabledNamesComplete(ctx context.Context, cmd *cli.Command) {
	catalogNamesComplete(ctx, cmd, func(e catalog.Entry) bool { return !e.Enabled })
}

func catalogEnabledNamesComplete(ctx context.Context, cmd *cli.Command) {
	catalogNamesComplete(ctx, cmd, func(e catalog.Entry) bool { return e.Enabled })
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
