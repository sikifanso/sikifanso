package main

import "github.com/urfave/cli/v3"

const defaultClusterName = "default"

func clusterCmd() *cli.Command {
	return &cli.Command{
		Name:  "cluster",
		Usage: "Manage k3d clusters",
		Commands: []*cli.Command{
			clusterCreateCmd(),
			clusterDeleteCmd(),
			clusterInfoCmd(),
			clusterStopCmd(),
			clusterStartCmd(),
		},
	}
}
