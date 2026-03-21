package mcp

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
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
		return errResult(fmt.Errorf("Docker is not running: %w", err))
	}
	return nil, nil, nil
}

// triggerSync fires ArgoCD webhooks to trigger reconciliation.
// Does not wait for sync to complete — use doctor or argocd_apps to check status.
func triggerSync(ctx context.Context, deps *Deps, sess *session.Session) error {
	return argocd.Sync(ctx, deps.Logger, sess.ClusterName, sess.Services.ArgoCD.URL)
}

// appendSyncStatus triggers an ArgoCD sync and appends the outcome to the result string.
func appendSyncStatus(ctx context.Context, deps *Deps, sess *session.Session, result string) string {
	if syncErr := triggerSync(ctx, deps, sess); syncErr != nil {
		return result + fmt.Sprintf("\nSync trigger warning: %v", syncErr)
	}
	return result + "\nArgoCD sync triggered."
}

// applyProfileToCluster resolves and applies a profile, then triggers sync.
func applyProfileToCluster(ctx context.Context, deps *Deps, sess *session.Session, profileName string) (string, error) {
	apps, err := profile.Resolve(profileName)
	if err != nil {
		return "", fmt.Errorf("resolving profile %q: %w", profileName, err)
	}

	var warnings []string
	if err := profile.Apply(sess.GitOpsPath, profileName, apps, func(msg string) {
		warnings = append(warnings, msg)
	}); err != nil {
		return "", fmt.Errorf("applying profile %q: %w", profileName, err)
	}

	result := fmt.Sprintf("Profile %q applied (%d apps enabled).", profileName, len(apps))
	for _, w := range warnings {
		result += fmt.Sprintf("\n  Warning: %s", w)
	}

	return appendSyncStatus(ctx, deps, sess, result), nil
}
