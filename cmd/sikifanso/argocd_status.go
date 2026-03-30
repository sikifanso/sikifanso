package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/urfave/cli/v3"
)

func argocdStatusCmd() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show detailed application status with resource tree",
		ArgsUsage: "[APP]",
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			client, err := grpcClientFromSession(ctx, sess)
			if err != nil {
				return err
			}
			defer client.Close()

			appName := cmd.Args().First()
			if appName != "" {
				return printAppDetail(ctx, client, appName)
			}
			return printAllAppsStatus(ctx, client)
		}),
	}
}

func printAllAppsStatus(ctx context.Context, client *grpcclient.Client) error {
	apps, err := client.ListApplications(ctx)
	if err != nil {
		return fmt.Errorf("listing applications: %w", err)
	}
	if len(apps) == 0 {
		fmt.Fprintln(os.Stderr, "No ArgoCD applications found.")
		return nil
	}

	for _, app := range apps {
		indicator := "✓"
		if app.Health == "Degraded" || app.Health == "Missing" {
			indicator = "✗"
		} else if app.SyncStatus != "Synced" || app.Health != "Healthy" {
			indicator = "~"
		}
		fmt.Fprintf(os.Stderr, "  %s %-30s sync=%-10s health=%s\n",
			indicator, app.Name, app.SyncStatus, app.Health)
	}
	return nil
}

func printAppDetail(ctx context.Context, client *grpcclient.Client, appName string) error {
	detail, err := client.GetApplication(ctx, appName)
	if err != nil {
		return fmt.Errorf("getting application %q: %w", appName, err)
	}

	fmt.Fprintf(os.Stderr, "Application: %s\n", detail.Name)
	fmt.Fprintf(os.Stderr, "  Sync:   %s\n", detail.SyncStatus)
	fmt.Fprintf(os.Stderr, "  Health: %s\n", detail.Health)
	if detail.Message != "" {
		fmt.Fprintf(os.Stderr, "  Message: %s\n", detail.Message)
	}

	nodes, err := client.ResourceTree(ctx, appName)
	if err != nil {
		return fmt.Errorf("fetching resource tree for %q: %w", appName, err)
	}

	if len(nodes) == 0 {
		fmt.Fprintln(os.Stderr, "\nNo resources found.")
		return nil
	}

	fmt.Fprintln(os.Stderr, "\nResources:")
	for _, node := range nodes {
		indicator := "✓"
		if node.Health != "" && node.Health != "Healthy" {
			indicator = "✗"
		}
		ref := strings.TrimLeft(fmt.Sprintf("%s/%s", node.Kind, node.Name), "/")
		if node.Namespace != "" {
			ref = fmt.Sprintf("%s/%s (%s)", node.Kind, node.Name, node.Namespace)
		}
		health := node.Health
		if health == "" {
			health = "-"
		}
		fmt.Fprintf(os.Stderr, "  %s %-50s health=%s\n", indicator, ref, health)
		if node.Message != "" {
			fmt.Fprintf(os.Stderr, "      -> %s\n", node.Message)
		}
	}
	return nil
}
