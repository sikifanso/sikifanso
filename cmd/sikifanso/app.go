package main

import "github.com/urfave/cli/v3"

func newApp() *cli.Command {
	return &cli.Command{
		Name:                  "sikifanso",
		Usage:                 "Bootstrap Kubernetes clusters for AI agent infrastructure",
		Version:               version,
		EnableShellCompletion: true,
		ConfigureShellCompletionCommand: func(cmd *cli.Command) {
			cmd.Hidden = false
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "Console log level (debug, info, warn, error)",
				Value: "info",
			},
			&cli.StringFlag{
				Name:    "cluster",
				Aliases: []string{"c"},
				Usage:   "Target cluster name",
				Value:   defaultClusterName,
				Sources: cli.EnvVars("SIKIFANSO_CLUSTER"),
			},
		},
		Before:   setupAction,
		Commands: []*cli.Command{clusterCmd(), argocdCmd(), appCmd(), catalogCmd(), profileCmd(), agentCmd(), mcpCmd(), statusCmd(), doctorCmd(), snapshotCmd(), restoreCmd(), dashboardCmd(), upgradeCmd()},
	}
}
