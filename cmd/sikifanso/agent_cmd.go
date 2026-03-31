package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/agent"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcsync"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

func agentCmd() *cli.Command {
	return &cli.Command{
		Name:  "agent",
		Usage: "Manage isolated agent namespaces",
		Commands: []*cli.Command{
			agentCreateCmd(),
			agentListCmd(),
			agentDeleteCmd(),
		},
	}
}

func agentCreateCmd() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create an isolated agent namespace",
		ArgsUsage: "NAME",
		Flags: append([]cli.Flag{
			&cli.StringFlag{Name: "cpu", Usage: "CPU quota", Value: "500m"},
			&cli.StringFlag{Name: "memory", Usage: "Memory quota", Value: "512Mi"},
			&cli.StringFlag{Name: "pods", Usage: "Max pods", Value: "10"},
		}, waitSyncFlags()...),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("agent name is required: sikifanso agent create NAME")
			}

			if err := agent.Create(sess.GitOpsPath, agent.CreateOpts{
				Name:   name,
				CPU:    cmd.String("cpu"),
				Memory: cmd.String("memory"),
				Pods:   cmd.String("pods"),
			}); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "%s created (namespace: agent-%s)\n", color.GreenString(name), name)
			fmt.Fprintln(os.Stderr, "committed to gitops repo")

			if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
				Operation:  grpcsync.OpEnable,
				Apps:       []string{name},
				AppSetName: "agents",
			}); err != nil {
				return err
			}
			return nil
		}),
	}
}

func agentListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List agent namespaces",
		Action: withSession(func(_ context.Context, cmd *cli.Command, sess *session.Session) error {
			agents, err := agent.List(sess.GitOpsPath)
			if err != nil {
				return fmt.Errorf("listing agents: %w", err)
			}
			if agents == nil {
				agents = []agent.Info{}
			}
			if outputJSON(cmd, agents) {
				return nil
			}
			if len(agents) == 0 {
				fmt.Fprintln(os.Stderr, "No agents found")
				return nil
			}

			bold := color.New(color.Bold).SprintFunc()
			fmt.Fprintf(os.Stderr, "%s  %s  %s  %s  %s\n",
				bold(fmt.Sprintf("%-20s", "NAME")),
				bold(fmt.Sprintf("%-28s", "NAMESPACE")),
				bold(fmt.Sprintf("%-8s", "CPU")),
				bold(fmt.Sprintf("%-10s", "MEMORY")),
				bold("PODS"),
			)
			for _, a := range agents {
				fmt.Fprintf(os.Stderr, "%-20s  %-28s  %-8s  %-10s  %s\n",
					a.Name, a.Namespace, a.CPU, a.Memory, a.Pods)
			}
			return nil
		}),
	}
}

func agentDeleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete an agent namespace",
		ArgsUsage: "NAME",
		Flags:     waitSyncFlags(),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("agent name is required: sikifanso agent delete NAME")
			}

			if err := agent.Delete(sess.GitOpsPath, name); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "%s deleted\n", color.GreenString(name))
			fmt.Fprintln(os.Stderr, "committed to gitops repo")

			if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
				Operation:  grpcsync.OpDisable,
				Apps:       []string{name},
				AppSetName: "agents",
			}); err != nil {
				return err
			}
			return nil
		}),
	}
}
