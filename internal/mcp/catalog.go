package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/profile"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type catalogListInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
}

type catalogToggleInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Name of the catalog app"`
}

type profileListInput struct{}

type profileApplyInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Profile name, e.g. agent-dev or agent-safe"`
}

func registerCatalogTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_list",
		Description: "List all catalog entries with their enabled/disabled status and category",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input catalogListInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}
		entries, err := catalog.List(sess.GitOpsPath)
		if err != nil {
			return errResult(fmt.Errorf("listing catalog: %w", err))
		}
		if len(entries) == 0 {
			return textResult("No catalog entries found.")
		}
		var sb strings.Builder
		sb.WriteString("Catalog entries:\n")
		for _, e := range entries {
			status := "disabled"
			if e.Enabled {
				status = "enabled"
			}
			fmt.Fprintf(&sb, "  - %-20s [%-14s] %s — %s\n",
				e.Name, e.Category, status, e.Description)
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_enable",
		Description: "Enable a catalog app and trigger ArgoCD sync",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input catalogToggleInput) (*mcp.CallToolResult, any, error) {
		return catalogToggle(ctx, deps, input.Cluster, input.Name, true)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "catalog_disable",
		Description: "Disable a catalog app and trigger ArgoCD sync",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input catalogToggleInput) (*mcp.CallToolResult, any, error) {
		return catalogToggle(ctx, deps, input.Cluster, input.Name, false)
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "profile_list",
		Description: "List available cluster profiles with their descriptions and included apps",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ profileListInput) (*mcp.CallToolResult, any, error) {
		profiles := profile.List()
		var sb strings.Builder
		sb.WriteString("Available profiles:\n")
		for _, p := range profiles {
			fmt.Fprintf(&sb, "  %s — %s\n    Apps: %s\n",
				p.Name, p.Description, strings.Join(p.Apps, ", "))
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "profile_apply",
		Description: "Apply a profile to a cluster (enables its catalog apps) and trigger ArgoCD sync",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input profileApplyInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}
		result, err := applyProfileToCluster(ctx, deps, sess, input.Name)
		if err != nil {
			return errResult(err)
		}
		return textResult(result)
	})
}

func catalogToggle(ctx context.Context, deps *Deps, clusterName, appName string, enable bool) (*mcp.CallToolResult, any, error) {
	past := "enabled"
	if !enable {
		past = "disabled"
	}

	sess, r, sv, e := loadSession(clusterName)
	if sess == nil {
		return r, sv, e
	}

	// MCP has no --force equivalent — agents must disable dependents explicitly.
	result, err := catalog.ToggleWithDeps(sess.GitOpsPath, appName, enable, false)
	if err != nil {
		return errResult(err)
	}
	if result.NoChange {
		return textResult(fmt.Sprintf("%s is already %s.", appName, past))
	}

	msg := fmt.Sprintf("%s %s and committed to gitops repo.", appName, past)
	if len(result.AutoDeps) > 0 {
		msg += fmt.Sprintf(" Auto-enabled dependencies: %s.", strings.Join(result.AutoDeps, ", "))
	}
	return textResult(appendSyncStatus(ctx, deps, sess, msg, "catalog"))
}
