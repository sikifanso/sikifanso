package main

import "github.com/urfave/cli/v3"

func newApp() *cli.Command {
	return &cli.Command{
		Name:    "sikifanso",
		Usage:   "A CLI tool for homelab k8s bootstrap",
		Version:               version,
		EnableShellCompletion: true,
		ConfigureShellCompletionCommand: func(cmd *cli.Command) {
			cmd.Hidden = false
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-file",
				Usage: "Path to log file",
				Value: "sikifanso.log",
			},
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
		Commands: []*cli.Command{clusterCmd(), argocdCmd(), appCmd(), catalogCmd(), statusCmd()},
	}
}
