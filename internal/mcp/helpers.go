package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/appsetreconcile"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/preflight"
	"github.com/alicanalbayrak/sikifanso/internal/profile"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/kubernetes"
)

// loadSession loads a cluster session, returning an MCP error result on failure.
func loadSession(clusterName string) (*session.Session, *mcp.CallToolResult, any, error) {
	sess, err := session.Load(clusterName)
	if err != nil {
		r, s, e := errResult(fmt.Errorf("loading cluster %q: %w", clusterName, err))
		return nil, r, s, e
	}
	return sess, nil, nil, nil
}

// kubeClient creates a Kubernetes clientset for the given cluster.
func kubeClient(clusterName string) (*kubernetes.Clientset, *mcp.CallToolResult, any, error) {
	cs, err := kube.ClientForCluster(clusterName)
	if err != nil {
		r, s, e := errResult(fmt.Errorf("connecting to cluster %q: %w", clusterName, err))
		return nil, r, s, e
	}
	return cs, nil, nil, nil
}

// requireDocker checks that Docker is running, returning an MCP error result on failure.
func requireDocker(ctx context.Context) (*mcp.CallToolResult, any, error) {
	if err := preflight.CheckDocker(ctx); err != nil {
		return errResult(fmt.Errorf("docker is not running: %w", err))
	}
	return nil, nil, nil
}

// triggerSync triggers ArgoCD ApplicationSet reconciliation via the refresh
// annotation. This replaces the old approach of listing and syncing individual
// apps via gRPC.
func triggerSync(ctx context.Context, _ *Deps, sess *session.Session, appSetNames ...string) error {
	restCfg, err := kube.RESTConfigForCluster(sess.ClusterName)
	if err != nil {
		return fmt.Errorf("k8s config: %w", err)
	}
	reconciler, err := appsetreconcile.NewReconciler(restCfg, "argocd")
	if err != nil {
		return fmt.Errorf("reconciler: %w", err)
	}
	return reconciler.Trigger(ctx, appSetNames...)
}

// appendSyncStatus triggers an ArgoCD ApplicationSet reconciliation and appends
// the outcome to the result string.
func appendSyncStatus(ctx context.Context, deps *Deps, sess *session.Session, result string, appSetNames ...string) string {
	if syncErr := triggerSync(ctx, deps, sess, appSetNames...); syncErr != nil {
		return result + fmt.Sprintf("\nSync trigger warning: %v", syncErr)
	}
	return result + "\nArgoCD reconciliation triggered."
}

// applyProfileToCluster resolves and applies a profile, then triggers sync.
func applyProfileToCluster(ctx context.Context, deps *Deps, sess *session.Session, profileName string) (string, error) {
	apps, err := profile.Resolve(profileName)
	if err != nil {
		return "", fmt.Errorf("resolving profile %q: %w", profileName, err)
	}

	var warnings []string
	autoAdded, err := profile.Apply(sess.GitOpsPath, profileName, apps, func(msg string) {
		warnings = append(warnings, msg)
	})
	if err != nil {
		return "", fmt.Errorf("applying profile %q: %w", profileName, err)
	}

	result := fmt.Sprintf("Profile %q applied (%d apps enabled).", profileName, len(apps)+len(autoAdded))
	if len(autoAdded) > 0 {
		result += fmt.Sprintf("\n  Auto-enabled dependencies: %s", strings.Join(autoAdded, ", "))
	}
	for _, w := range warnings {
		result += fmt.Sprintf("\n  Warning: %s", w)
	}

	return appendSyncStatus(ctx, deps, sess, result, "catalog"), nil
}
