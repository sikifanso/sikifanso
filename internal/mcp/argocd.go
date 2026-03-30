package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type argocdAppInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Application name"`
}

type argocdRollbackInput struct {
	Cluster  string `json:"cluster" jsonschema:"Name of the cluster"`
	Name     string `json:"name" jsonschema:"Application name"`
	Revision int64  `json:"revision" jsonschema:"Revision ID to rollback to"`
}

// grpcClientFromMCPSession creates a gRPC client from session credentials.
// Returns an error if GRPCAddress is not populated.
func grpcClientFromMCPSession(ctx context.Context, sess *session.Session) (*grpcclient.Client, error) {
	if sess.Services.ArgoCD.GRPCAddress == "" {
		return nil, fmt.Errorf("no gRPC address in session — cluster may need recreation")
	}
	return grpcclient.NewClient(ctx, grpcclient.Options{
		Address:  sess.Services.ArgoCD.GRPCAddress,
		Username: sess.Services.ArgoCD.Username,
		Password: sess.Services.ArgoCD.Password,
	})
}

func registerArgoCDTools(s *mcp.Server, _ *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_app_detail",
		Description: "Get detailed status and resource tree for an ArgoCD application",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		client, err := grpcClientFromMCPSession(ctx, sess)
		if err != nil {
			return errResult(fmt.Errorf("connecting to ArgoCD gRPC: %w", err))
		}
		defer client.Close()

		detail, err := client.GetApplication(ctx, input.Name)
		if err != nil {
			return errResult(fmt.Errorf("getting application %q: %w", input.Name, err))
		}

		nodes, err := client.ResourceTree(ctx, input.Name)
		if err != nil {
			return errResult(fmt.Errorf("fetching resource tree for %q: %w", input.Name, err))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Application: %s\n", detail.Name)
		fmt.Fprintf(&sb, "  Sync:   %s\n", detail.SyncStatus)
		fmt.Fprintf(&sb, "  Health: %s\n", detail.Health)
		if detail.Message != "" {
			fmt.Fprintf(&sb, "  Message: %s\n", detail.Message)
		}

		if len(nodes) > 0 {
			sb.WriteString("\nResources:\n")
			for _, node := range nodes {
				health := node.Health
				if health == "" {
					health = "-"
				}
				ref := fmt.Sprintf("%s/%s", node.Kind, node.Name)
				if node.Namespace != "" {
					ref = fmt.Sprintf("%s/%s (%s)", node.Kind, node.Name, node.Namespace)
				}
				fmt.Fprintf(&sb, "  %-50s health=%s\n", ref, health)
				if node.Message != "" {
					fmt.Fprintf(&sb, "    -> %s\n", node.Message)
				}
			}
		}

		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_app_diff",
		Description: "Show diff between live and desired state for an ArgoCD application",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		client, err := grpcClientFromMCPSession(ctx, sess)
		if err != nil {
			return errResult(fmt.Errorf("connecting to ArgoCD gRPC: %w", err))
		}
		defer client.Close()

		resources, err := client.ManagedResources(ctx, input.Name)
		if err != nil {
			return errResult(fmt.Errorf("fetching managed resources for %q: %w", input.Name, err))
		}

		var sb strings.Builder
		hasDiff := false
		for _, res := range resources {
			if res.LiveState == res.TargetState {
				continue
			}
			hasDiff = true
			fmt.Fprintf(&sb, "--- live    %s/%s\n", res.Kind, res.Name)
			fmt.Fprintf(&sb, "+++ desired %s/%s\n", res.Kind, res.Name)
			if res.Namespace != "" {
				fmt.Fprintf(&sb, "    namespace: %s\n", res.Namespace)
			}
			if res.LiveState == "" {
				sb.WriteString("  (resource not present in live cluster)\n")
			} else {
				fmt.Fprintf(&sb, "  live:\n%s\n", res.LiveState)
			}
			if res.TargetState == "" {
				sb.WriteString("  (resource not present in desired state)\n")
			} else {
				fmt.Fprintf(&sb, "  desired:\n%s\n", res.TargetState)
			}
			sb.WriteString("\n")
		}

		if !hasDiff {
			fmt.Fprintf(&sb, "No diff found for application %q — live state matches desired state.\n", input.Name)
		}

		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_rollback",
		Description: "Roll back an ArgoCD application to a previous revision",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdRollbackInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		client, err := grpcClientFromMCPSession(ctx, sess)
		if err != nil {
			return errResult(fmt.Errorf("connecting to ArgoCD gRPC: %w", err))
		}
		defer client.Close()

		if err := client.Rollback(ctx, input.Name, input.Revision); err != nil {
			return errResult(fmt.Errorf("rolling back application %q: %w", input.Name, err))
		}

		return textResult(fmt.Sprintf("Application %q rolled back to revision %d.", input.Name, input.Revision))
	})
}
