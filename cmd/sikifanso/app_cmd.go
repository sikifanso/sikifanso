package main

import (
	"context"
	"fmt"
	"os"
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
	"k8s.io/utils/ptr"
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
		Name:  "list",
		Usage: "List installed apps (use --all to include full catalog)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Show all catalog entries including disabled",
			},
		},
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
	Enabled   *bool  `json:"enabled,omitempty"`
}

func buildAppListItems(apps []app.AppInfo, catalogEntries []catalog.Entry, showAll bool) []appListItem {
	items := make([]appListItem, 0, len(apps)+len(catalogEntries))
	for _, a := range apps {
		item := appListItem{Name: a.Name, Chart: a.Chart, Version: a.Version, Namespace: a.Namespace, Source: "custom"}
		if showAll {
			item.Enabled = ptr.To(true)
		}
		items = append(items, item)
	}
	for _, e := range catalogEntries {
		if showAll || e.Enabled {
			item := appListItem{Name: e.Name, Chart: e.Chart, Version: e.TargetRevision, Namespace: e.Namespace, Source: "catalog"}
			if showAll {
				item.Enabled = ptr.To(e.Enabled)
			}
			items = append(items, item)
		}
	}
	return items
}

func appListAction(_ context.Context, cmd *cli.Command, sess *session.Session) error {
	showAll := cmd.Bool("all")

	apps, err := app.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing apps: %w", err)
	}

	catalogEntries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("listing catalog: %w", err)
	}

	items := buildAppListItems(apps, catalogEntries, showAll)

	if cmd.String("output") == outputFormatJSON {
		outputJSON(cmd, items)
		return nil
	}

	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "No apps found — add one with: sikifanso app add")
		return nil
	}

	headers := []string{"NAME", "CHART", "VERSION", "NAMESPACE", "SOURCE"}
	if showAll {
		headers = append(headers, "ENABLED")
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		row := []string{item.Name, item.Chart, item.Version, item.Namespace, item.Source}
		if showAll {
			row = append(row, fmt.Sprintf("%v", ptr.Deref(item.Enabled, false)))
		}
		rows = append(rows, row)
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
		Flags: append(waitSyncFlags(), &cli.BoolFlag{
			Name:  "force",
			Usage: "Bypass dependent-app safety check",
		}),
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

	force := cmd.Bool("force")
	result, err := catalog.ToggleWithDeps(sess.GitOpsPath, name, enable, force)
	if err != nil {
		return err
	}
	if result.NoChange {
		fmt.Fprintf(os.Stderr, "%s is already %s\n", name, past)
		return nil
	}

	if len(result.AutoDeps) > 0 {
		fmt.Fprintf(os.Stderr, "auto-enabled: %s\n", strings.Join(result.AutoDeps, ", "))
	}

	fmt.Fprintf(os.Stderr, "%s committed to gitops repo\n", name)

	op := grpcsync.OpEnable
	if !enable {
		op = grpcsync.OpDisable
	}

	syncApps := []string{name}
	if len(result.AutoDeps) > 0 {
		syncApps = append(append([]string{}, result.AutoDeps...), name)
	}

	if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
		Operation:  op,
		Apps:       syncApps,
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
