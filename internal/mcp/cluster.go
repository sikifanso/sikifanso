package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type clusterListInput struct{}

type clusterInfoInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
}

type clusterCreateInput struct {
	Name         string `json:"name" jsonschema:"Name for the new cluster"`
	Profile      string `json:"profile,omitempty" jsonschema:"Profile to apply after creation, e.g. agent-dev or agent-safe"`
	BootstrapURL string `json:"bootstrap_url,omitempty" jsonschema:"Bootstrap template repo URL"`
}

type clusterDeleteInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster to delete"`
}

type clusterStartStopInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Action  string `json:"action" jsonschema:"Action to perform: start or stop"`
}

const (
	actionStart = "start"
	actionStop  = "stop"
)

func registerClusterTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "cluster_list",
		Description: "List all sikifanso clusters with their state",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ clusterListInput) (*mcp.CallToolResult, any, error) {
		sessions, err := session.ListAll()
		if err != nil {
			return errResult(fmt.Errorf("listing clusters: %w", err))
		}
		if len(sessions) == 0 {
			return textResult("No clusters found. Use cluster_create to create one.")
		}
		var sb strings.Builder
		sb.WriteString("Clusters:\n")
		for _, sess := range sessions {
			fmt.Fprintf(&sb, "  - %s (state: %s, created: %s)\n",
				sess.ClusterName, sess.State, sess.CreatedAt.Format("2006-01-02 15:04"))
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "cluster_info",
		Description: "Get detailed information about a cluster (state, services, k3d config)",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input clusterInfoInput) (*mcp.CallToolResult, any, error) {
		sess, r, s, e := loadSession(input.Cluster)
		if sess == nil {
			return r, s, e
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Cluster: %s\n", sess.ClusterName)
		fmt.Fprintf(&sb, "State: %s\n", sess.State)
		fmt.Fprintf(&sb, "Created: %s\n", sess.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Fprintf(&sb, "GitOps Path: %s\n", sess.GitOpsPath)
		fmt.Fprintf(&sb, "Bootstrap: %s @ %s\n", sess.BootstrapURL, sess.BootstrapVersion)
		fmt.Fprintf(&sb, "K3d Image: %s\n", sess.K3dConfig.Image)
		fmt.Fprintf(&sb, "K3d Servers: %d, Agents: %d\n", sess.K3dConfig.Servers, sess.K3dConfig.Agents)
		fmt.Fprintf(&sb, "ArgoCD URL: %s\n", sess.Services.ArgoCD.URL)
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "cluster_create",
		Description: "Create a new k3d cluster with ArgoCD and Cilium. This takes 2-3 minutes.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input clusterCreateInput) (*mcp.CallToolResult, any, error) {
		if r, s, e := requireDocker(ctx); r != nil {
			return r, s, e
		}

		bootstrapURL := input.BootstrapURL
		if bootstrapURL == "" {
			bootstrapURL = gitops.DefaultBootstrapURL
		}

		sess, err := cluster.Create(ctx, deps.Logger, input.Name, cluster.Options{
			BootstrapURL: bootstrapURL,
		})
		if err != nil {
			return errResult(fmt.Errorf("creating cluster: %w", err))
		}

		result := fmt.Sprintf("Cluster %q created successfully.\nArgoCD URL: %s\nGitOps Path: %s",
			sess.ClusterName, sess.Services.ArgoCD.URL, sess.GitOpsPath)

		if input.Profile != "" {
			profileResult, profileErr := applyProfileToCluster(ctx, deps, sess, input.Profile)
			if profileErr != nil {
				return errResult(profileErr)
			}
			result += "\n" + profileResult
		}

		return textResult(result)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "cluster_delete",
		Description: "Delete a cluster permanently. This removes the k3d cluster and all session data.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input clusterDeleteInput) (*mcp.CallToolResult, any, error) {
		if r, s, e := requireDocker(ctx); r != nil {
			return r, s, e
		}
		if err := cluster.Delete(ctx, deps.Logger, input.Cluster); err != nil {
			return errResult(fmt.Errorf("deleting cluster: %w", err))
		}
		return textResult(fmt.Sprintf("Cluster %q deleted.", input.Cluster))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "cluster_start_stop",
		Description: "Start or stop a cluster. Action must be 'start' or 'stop'.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input clusterStartStopInput) (*mcp.CallToolResult, any, error) {
		if r, s, e := requireDocker(ctx); r != nil {
			return r, s, e
		}
		switch input.Action {
		case actionStart:
			if err := cluster.Start(ctx, deps.Logger, input.Cluster); err != nil {
				return errResult(fmt.Errorf("starting cluster: %w", err))
			}
			return textResult(fmt.Sprintf("Cluster %q started.", input.Cluster))
		case actionStop:
			if err := cluster.Stop(ctx, deps.Logger, input.Cluster); err != nil {
				return errResult(fmt.Errorf("stopping cluster: %w", err))
			}
			return textResult(fmt.Sprintf("Cluster %q stopped.", input.Cluster))
		default:
			return errResult(fmt.Errorf("invalid action %q: must be 'start' or 'stop'", input.Action))
		}
	})
}
