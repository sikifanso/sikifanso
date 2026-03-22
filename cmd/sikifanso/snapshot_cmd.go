package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/alicanalbayrak/sikifanso/internal/snapshot"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

func snapshotCmd() *cli.Command {
	return &cli.Command{
		Name:  "snapshot",
		Usage: "Capture and manage cluster state snapshots",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Snapshot name",
			},
		},
		Action: snapshotCaptureAction,
		Commands: []*cli.Command{
			snapshotListCmd(),
			snapshotDeleteCmd(),
		},
	}
}

func snapshotCaptureAction(_ context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	snapshotName := cmd.String("name")
	if snapshotName == "" {
		return fmt.Errorf("snapshot name is required: sikifanso snapshot --name NAME")
	}

	path, err := snapshot.Capture(clusterName, snapshotName, version)
	if err != nil {
		return fmt.Errorf("capturing snapshot: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Snapshot %s saved to %s\n", color.GreenString(snapshotName), path)
	return nil
}

func snapshotListCmd() *cli.Command {
	return &cli.Command{
		Name:   "list",
		Usage:  "List available snapshots",
		Action: snapshotListAction,
	}
}

func snapshotListAction(_ context.Context, cmd *cli.Command) error {
	metas, err := snapshot.List()
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}
	if metas == nil {
		metas = []snapshot.Meta{}
	}
	if outputJSON(cmd, metas) {
		return nil
	}

	if len(metas) == 0 {
		fmt.Fprintln(os.Stderr, "No snapshots found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tCLUSTER\tCREATED\tCLI VERSION")
	for _, m := range metas {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			m.Name, m.ClusterName, m.CreatedAt.Format("2006-01-02 15:04:05"), m.CLIVersion)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing output: %w", err)
	}

	return nil
}

func snapshotDeleteCmd() *cli.Command {
	return &cli.Command{
		Name:          "delete",
		Usage:         "Remove a snapshot",
		ArgsUsage:     "NAME",
		Action:        snapshotDeleteAction,
		ShellComplete: snapshotNameComplete,
	}
}

func snapshotDeleteAction(_ context.Context, cmd *cli.Command) error {
	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("snapshot name is required: sikifanso snapshot delete NAME")
	}

	if err := snapshot.Delete(name); err != nil {
		return fmt.Errorf("deleting snapshot: %w", err)
	}

	fmt.Fprintf(os.Stderr, "%s deleted\n", color.GreenString(name))
	return nil
}

func restoreCmd() *cli.Command {
	return &cli.Command{
		Name:          "restore",
		Usage:         "Restore a cluster from a snapshot",
		ArgsUsage:     "NAME",
		Action:        restoreAction,
		ShellComplete: snapshotNameComplete,
	}
}

func restoreAction(_ context.Context, cmd *cli.Command) error {
	snapshotName := cmd.Args().First()
	if snapshotName == "" {
		return fmt.Errorf("snapshot name is required: sikifanso restore NAME")
	}

	sess, gitOpsPath, err := snapshot.Restore(snapshotName)
	if err != nil {
		return fmt.Errorf("restoring snapshot: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Cluster %s restored from snapshot %s\n",
		color.GreenString(sess.ClusterName), color.GreenString(snapshotName))
	fmt.Fprintf(os.Stderr, "GitOps path: %s\n", gitOpsPath)
	fmt.Fprintln(os.Stderr, "Run 'sikifanso cluster create' to recreate the cluster infrastructure")
	return nil
}

func snapshotNameComplete(_ context.Context, cmd *cli.Command) {
	metas, err := snapshot.List()
	if err != nil {
		return
	}
	for _, m := range metas {
		_, _ = fmt.Fprintln(cmd.Root().Writer, m.Name)
	}
}
