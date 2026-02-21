package main

import "github.com/urfave/cli/v3"

func argocdCmd() *cli.Command {
	return &cli.Command{
		Name:  "argocd",
		Usage: "Manage ArgoCD",
		Commands: []*cli.Command{
			argocdSyncCmd(),
		},
	}
}
