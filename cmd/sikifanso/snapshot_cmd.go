package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/snapshot"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

func snapshotCmd() *cli.Command {
	return &cli.Command{
		Name:  "snapshot",
		Usage: "Capture, restore, and manage cluster snapshots",
		Commands: []*cli.Command{
			snapshotCaptureCmd(),
			snapshotListCmd(),
			snapshotRestoreCmd(),
			snapshotDeleteCmd(),
		},
	}
}

func snapshotCaptureCmd() *cli.Command {
	return &cli.Command{
		Name:  "capture",
		Usage: "Capture current cluster state",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Snapshot name",
			},
		},
		Action: snapshotCaptureAction,
	}
}

func snapshotCaptureAction(_ context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	snapshotName := cmd.String("name")
	if snapshotName == "" {
		return fmt.Errorf("snapshot name is required: sikifanso snapshot capture --name NAME")
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

	headers := []string{"NAME", "CLUSTER", "CREATED", "CLI VERSION"}
	rows := make([][]string, 0, len(metas))
	for _, m := range metas {
		rows = append(rows, []string{m.Name, m.ClusterName, m.CreatedAt.Format("2006-01-02 15:04:05"), m.CLIVersion})
	}
	printTable(os.Stderr, headers, rows)
	return nil
}

func snapshotRestoreCmd() *cli.Command {
	return &cli.Command{
		Name:          "restore",
		Usage:         "Restore a cluster from a snapshot",
		ArgsUsage:     "NAME",
		Action:        snapshotRestoreAction,
		ShellComplete: snapshotNameComplete,
	}
}

func snapshotRestoreAction(_ context.Context, cmd *cli.Command) error {
	snapshotName := cmd.Args().First()
	if snapshotName == "" {
		return fmt.Errorf("snapshot name is required: sikifanso snapshot restore NAME")
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

func snapshotNameComplete(_ context.Context, cmd *cli.Command) {
	metas, err := snapshot.List()
	if err != nil {
		return
	}
	for _, m := range metas {
		_, _ = fmt.Fprintln(cmd.Root().Writer, m.Name)
	}
}
