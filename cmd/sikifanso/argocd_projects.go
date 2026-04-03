package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdProjectsCmd() *cli.Command {
	return &cli.Command{
		Name:  "projects",
		Usage: "Manage ArgoCD projects",
		Commands: []*cli.Command{
			argocdProjectsListCmd(),
			argocdProjectsCreateCmd(),
			argocdProjectsDeleteCmd(),
		},
	}
}

func argocdProjectsListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List ArgoCD projects",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output format: table or json",
				Value:   outputFormatTable,
			},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			projects, err := client.ListProjects(ctx)
			if err != nil {
				return fmt.Errorf("listing projects: %w", err)
			}
			if projects == nil {
				projects = []grpcclient.ProjectSummary{}
			}

			if outputJSON(cmd, projects) {
				return nil
			}

			if len(projects) == 0 {
				fmt.Fprintln(os.Stderr, "No ArgoCD projects found.")
				return nil
			}

			headers := []string{"NAME", "DESCRIPTION"}
			rows := make([][]string, 0, len(projects))
			for _, p := range projects {
				desc := p.Description
				if desc == "" {
					desc = "-"
				}
				rows = append(rows, []string{p.Name, desc})
			}
			printTable(os.Stderr, headers, rows)
			return nil
		}),
	}
}

func argocdProjectsCreateCmd() *cli.Command {
	return &cli.Command{
		Name:      "create",
		Usage:     "Create an ArgoCD project",
		ArgsUsage: "NAME",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "description",
				Usage: "Project description",
			},
			&cli.StringSliceFlag{
				Name:  "destination",
				Usage: "Allowed destination (server/namespace), repeatable",
			},
			&cli.StringSliceFlag{
				Name:  "source",
				Usage: "Allowed source repository URL, repeatable",
			},
		},
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("project name is required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			rawDests := cmd.StringSlice("destination")
			dests := make([]grpcclient.ProjectDestination, 0, len(rawDests))
			for _, d := range rawDests {
				// Accept "server/namespace" format.
				server, namespace := splitDestination(d)
				dests = append(dests, grpcclient.ProjectDestination{
					Server:    server,
					Namespace: namespace,
				})
			}

			spec := grpcclient.ProjectSpec{
				Name:         name,
				Description:  cmd.String("description"),
				Destinations: dests,
				Sources:      cmd.StringSlice("source"),
			}

			if err := client.CreateProject(ctx, spec); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Project %q created.\n", name)
			return nil
		}),
	}
}

func argocdProjectsDeleteCmd() *cli.Command {
	return &cli.Command{
		Name:      "delete",
		Usage:     "Delete an ArgoCD project",
		ArgsUsage: "NAME",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			name := cmd.Args().First()
			if name == "" {
				return fmt.Errorf("project name is required")
			}

			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			if err := client.DeleteProject(ctx, name); err != nil {
				return err
			}

			fmt.Fprintf(os.Stderr, "Project %q deleted.\n", name)
			return nil
		}),
	}
}

// splitDestination splits a "server/namespace" string into its parts.
// If no "/" is present the whole string is treated as the server.
func splitDestination(dest string) (server, namespace string) {
	for i := 0; i < len(dest); i++ {
		if dest[i] == '/' {
			return dest[:i], dest[i+1:]
		}
	}
	return dest, ""
}
