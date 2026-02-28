package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/doctor"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"k8s.io/client-go/dynamic"
)

func doctorCmd() *cli.Command {
	return &cli.Command{
		Name:   "doctor",
		Usage:  "Run health checks on the cluster and its components",
		Action: doctorAction,
	}
}

func doctorAction(ctx context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")

	checks := doctor.InfraChecks()

	sess, err := session.Load(clusterName)
	if err != nil {
		zapLogger.Info("no session found, running Docker check only", zap.String("cluster", clusterName))
		results := doctor.Run(ctx, checks)
		results = append(results, doctor.Result{
			Name:  "k3d cluster",
			OK:    false,
			Cause: fmt.Sprintf("no session found for cluster %q", clusterName),
			Fix:   "sikifanso cluster create",
		})
		return printResults(results)
	}

	cs, err := kube.ClientForCluster(clusterName)
	if err != nil {
		zapLogger.Warn("could not create Kubernetes client", zap.Error(err))
		results := doctor.Run(ctx, checks)
		results = append(results, doctor.Result{
			Name:  "k3d cluster",
			OK:    false,
			Cause: fmt.Sprintf("cannot connect to cluster: %v", err),
			Fix:   "sikifanso cluster start",
		})
		return printResults(results)
	}

	checks = append(checks, doctor.ClusterChecks(cs)...)

	restCfg, err := kube.RESTConfigForCluster(clusterName)
	if err == nil {
		dynClient, dynErr := dynamic.NewForConfig(restCfg)
		if dynErr == nil {
			checks = append(checks, doctor.AppChecks(dynClient, sess.GitOpsPath)...)
		} else {
			zapLogger.Warn("could not create dynamic client", zap.Error(dynErr))
		}
	}

	results := doctor.Run(ctx, checks)
	return printResults(results)
}

func printResults(results []doctor.Result) error {
	// Compute max name width for alignment. Pad plain text first,
	// then wrap in color so ANSI escape codes don't break widths.
	nameWidth := 0
	for _, r := range results {
		if len(r.Name) > nameWidth {
			nameWidth = len(r.Name)
		}
	}

	anyFailed := false
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	for _, r := range results {
		paddedName := fmt.Sprintf("%-*s", nameWidth, r.Name)
		if r.OK {
			fmt.Fprintf(os.Stderr, "%s  %s  %s\n", green("ok"), paddedName, r.Message)
		} else {
			anyFailed = true
			msg := r.Cause
			if r.Message != "" {
				msg = r.Message
			}
			fmt.Fprintf(os.Stderr, "%s  %s  %s\n", red("!!"), paddedName, msg)
			// Indent detail lines under the message column.
			indent := fmt.Sprintf("%-*s", nameWidth+6, "")
			if r.Cause != "" && r.Message != "" {
				fmt.Fprintf(os.Stderr, "%s-> %s\n", indent, r.Cause)
			}
			if r.Fix != "" {
				fmt.Fprintf(os.Stderr, "%s-> Try: %s\n", indent, r.Fix)
			}
		}
	}

	if anyFailed {
		return cli.Exit("", 1)
	}
	return nil
}
