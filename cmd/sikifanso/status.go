package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:          "status",
		Usage:         "Show cluster status dashboard",
		ArgsUsage:     "[NAME]",
		Action:        statusAction,
		ShellComplete: clusterNameShellComplete,
	}
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Present() {
		return showStatus(ctx, cmd.Args().First())
	}

	sessions, err := session.ListAll()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No clusters found — create one with: sikifanso cluster create")
		return nil
	}

	for i, sess := range sessions {
		if i > 0 {
			fmt.Fprintln(os.Stderr)
		}
		if err := showStatus(ctx, sess.ClusterName); err != nil {
			zapLogger.Warn("failed to show status", zap.String("cluster", sess.ClusterName), zap.Error(err))
		}
	}
	return nil
}

func showStatus(ctx context.Context, name string) error {
	sess, err := session.Load(name)
	if err != nil {
		zapLogger.Error("failed to load session", zap.String("cluster", name), zap.Error(err))
		return fmt.Errorf("no session found for cluster %q — was it created with sikifanso?", name)
	}

	live, err := cluster.Exists(ctx, name)
	if err != nil {
		zapLogger.Warn("could not check cluster status", zap.String("cluster", name), zap.Error(err))
	}

	// Header
	title := fmt.Sprintf("  Cluster: %s  ", name)
	hBar := strings.Repeat("─", len(title))
	fmt.Fprintf(os.Stderr, "┌%s┐\n", hBar)
	fmt.Fprintf(os.Stderr, "│%s│\n", title)
	fmt.Fprintf(os.Stderr, "└%s┘\n", hBar)

	// Session info
	fmt.Fprintf(os.Stderr, "  State:    %s\n", stateString(sess.State))
	fmt.Fprintf(os.Stderr, "  Created:  %s\n", sess.CreatedAt.Format("2006-01-02 15:04"))

	nodeDesc := fmt.Sprintf("%d server", sess.K3dConfig.Servers)
	if sess.K3dConfig.Agents > 0 {
		nodeDesc += fmt.Sprintf(" + %d agent", sess.K3dConfig.Agents)
	}
	fmt.Fprintf(os.Stderr, "  Nodes:    %s\n", nodeDesc)

	if !live {
		fmt.Fprintf(os.Stderr, "\n  %s\n", color.YellowString("Cluster is not running — skipping Kubernetes queries."))
		return nil
	}

	// Kubernetes queries
	cs, err := kube.ClientForCluster(name)
	if err != nil {
		zapLogger.Warn("could not connect to Kubernetes API", zap.Error(err))
		fmt.Fprintf(os.Stderr, "\n  %s\n", color.YellowString("Could not connect to Kubernetes API."))
		return nil
	}

	// Nodes
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		zapLogger.Warn("could not list nodes", zap.Error(err))
		fmt.Fprintf(os.Stderr, "\n  %s\n", color.YellowString("Could not query nodes."))
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n  Nodes:\n")
	for _, n := range nodes.Items {
		status := color.RedString("NotReady")
		if kube.NodeReady(n) {
			status = color.GreenString("Ready")
		}
		fmt.Fprintf(os.Stderr, "    %-40s %s\n", n.Name, status)
	}

	// Pods
	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		zapLogger.Warn("could not list pods", zap.Error(err))
		fmt.Fprintf(os.Stderr, "\n  %s\n", color.YellowString("Could not query pods."))
		return nil
	}

	type podCounts struct {
		running int
		pending int
		failed  int
	}
	nsByCount := make(map[string]*podCounts)
	var nsOrder []string
	for _, p := range pods.Items {
		ns := p.Namespace
		c, ok := nsByCount[ns]
		if !ok {
			c = &podCounts{}
			nsByCount[ns] = c
			nsOrder = append(nsOrder, ns)
		}
		switch p.Status.Phase {
		case "Running", "Succeeded":
			c.running++
		case "Pending":
			c.pending++
		case "Failed":
			c.failed++
		default:
			c.pending++
		}
	}

	fmt.Fprintf(os.Stderr, "\n  Pods:\n")
	fmt.Fprintf(os.Stderr, "    %-30s %8s %8s %8s\n", "NAMESPACE", "RUNNING", "PENDING", "FAILED")
	for _, ns := range nsOrder {
		c := nsByCount[ns]
		fmt.Fprintf(os.Stderr, "    %-30s %8d %8d %8d\n", ns, c.running, c.pending, c.failed)
	}

	return nil
}
