package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func clusterInfoCmd() *cli.Command {
	return &cli.Command{
		Name:   "info",
		Usage:  "Show cluster details, credentials, and runtime health",
		Action: clusterInfoAction,
	}
}

func clusterInfoAction(ctx context.Context, cmd *cli.Command) error {
	if err := rejectPositionalArgs(cmd); err != nil {
		return err
	}

	// If --cluster is explicitly set, show only that cluster.
	if cmd.IsSet("cluster") {
		return showClusterInfo(ctx, cmd, cmd.String("cluster"))
	}

	// No name — show all clusters.
	sessions, err := session.ListAll()
	if err != nil {
		return fmt.Errorf("listing sessions: %w", err)
	}
	if sessions == nil {
		sessions = []*session.Session{}
	}
	if outputJSON(cmd, sessions) {
		return nil
	}
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "no clusters found — create one with: sikifanso cluster create")
		return nil
	}

	for i, sess := range sessions {
		if i > 0 {
			fmt.Fprintln(os.Stderr)
		}
		live, err := cluster.Exists(ctx, sess.ClusterName)
		if err != nil {
			zapLogger.Warn("could not check cluster status", zap.String("cluster", sess.ClusterName), zap.Error(err))
		} else if !live {
			fmt.Fprintf(os.Stderr, "warning: cluster %q is not currently running\n", sess.ClusterName)
		}
		printClusterInfo(sess)
		if live {
			printClusterHealth(ctx, sess.ClusterName)
		}
	}
	return nil
}

func showClusterInfo(ctx context.Context, cmd *cli.Command, name string) error {
	sess, err := session.Load(name)
	if err != nil {
		zapLogger.Error("failed to load session", zap.String("cluster", name), zap.Error(err))
		return fmt.Errorf("no session found for cluster %q — was it created with sikifanso?", name)
	}
	if outputJSON(cmd, sess) {
		return nil
	}

	live, err := cluster.Exists(ctx, name)
	if err != nil {
		zapLogger.Warn("could not check cluster status", zap.Error(err))
	} else if !live {
		fmt.Fprintf(os.Stderr, "warning: cluster %q is not currently running\n", name)
	}

	printClusterInfo(sess)

	if live {
		printClusterHealth(ctx, name)
	}
	return nil
}

// printClusterHealth queries the Kubernetes API and displays node readiness
// and pod counts per namespace. Gracefully degrades on errors.
func printClusterHealth(ctx context.Context, clusterName string) {
	cs, err := kube.ClientForCluster(clusterName)
	if err != nil {
		zapLogger.Warn("could not connect to Kubernetes API", zap.Error(err))
		fmt.Fprintf(os.Stderr, "  %s\n", color.YellowString("Could not connect to Kubernetes API."))
		return
	}

	// Nodes
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		zapLogger.Warn("could not list nodes", zap.Error(err))
		fmt.Fprintf(os.Stderr, "  %s\n", color.YellowString("Could not query nodes."))
		return
	}

	fmt.Fprintf(os.Stderr, "  Nodes:\n")
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
		return
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
		case corev1.PodRunning, corev1.PodSucceeded:
			c.running++
		case corev1.PodPending:
			c.pending++
		case corev1.PodFailed:
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
}
