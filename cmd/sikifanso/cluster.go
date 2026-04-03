package main

import "github.com/urfave/cli/v3"

const defaultClusterName = "default"

func clusterCmd() *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "Manage local Kubernetes clusters",
		Commands: []*cli.Command{
			clusterCreateCmd(),
			clusterDeleteCmd(),
			clusterInfoCmd(),
			clusterStopCmd(),
			clusterStartCmd(),
			clusterDoctorCmd(),
			clusterDashboardCmd(),
			clusterUpgradeCmd(),
			clusterProfilesCmd(),
		},
	}
}
