package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/agent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type agentListInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
}

type agentInfoInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Name of the agent"`
}

type agentCreateInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Name for the new agent"`
	CPU     string `json:"cpu,omitempty" jsonschema:"CPU quota, e.g. 500m"`
	Memory  string `json:"memory,omitempty" jsonschema:"Memory quota, e.g. 512Mi"`
	Pods    string `json:"pods,omitempty" jsonschema:"Max pods, e.g. 10"`
}

type agentDeleteInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
	Name    string `json:"name" jsonschema:"Name of the agent to delete"`
}

func registerAgentTools(s *mcp.Server, deps *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "agent_list",
		Description: "List all agents with their resource quotas (CPU, memory, pods)",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input agentListInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}
		agents, err := agent.List(sess.GitOpsPath)
		if err != nil {
			return errResult(fmt.Errorf("listing agents: %w", err))
		}
		if len(agents) == 0 {
			return textResult("No agents found. Use agent_create to create one.")
		}
		var sb strings.Builder
		sb.WriteString("Agents:\n")
		for _, a := range agents {
			fmt.Fprintf(&sb, "  - %s (namespace: %s, cpu: %s, memory: %s, pods: %s)\n",
				a.Name, a.Namespace, a.CPU, a.Memory, a.Pods)
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "agent_info",
		Description: "Get detailed information about a specific agent",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input agentInfoInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}
		info, err := agent.Find(sess.GitOpsPath, input.Name)
		if err != nil {
			return errResult(err)
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Agent: %s\n", info.Name)
		fmt.Fprintf(&sb, "Namespace: %s\n", info.Namespace)
		fmt.Fprintf(&sb, "CPU Quota: %s\n", info.CPU)
		fmt.Fprintf(&sb, "Memory Quota: %s\n", info.Memory)
		fmt.Fprintf(&sb, "Max Pods: %s\n", info.Pods)
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "agent_create",
		Description: "Create an isolated agent namespace with resource quotas and network policies",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input agentCreateInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		opts := agent.CreateOpts{
			Name:   input.Name,
			CPU:    input.CPU,
			Memory: input.Memory,
			Pods:   input.Pods,
		}
		if err := agent.Create(sess.GitOpsPath, opts); err != nil {
			return errResult(err)
		}

		result := fmt.Sprintf("Agent %q created (namespace: agent-%s).\nCommitted to gitops repo.", input.Name, input.Name)
		return textResult(appendSyncStatus(ctx, deps, sess, result))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "agent_delete",
		Description: "Delete an agent and its namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input agentDeleteInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		if err := agent.Delete(sess.GitOpsPath, input.Name); err != nil {
			return errResult(err)
		}

		result := fmt.Sprintf("Agent %q deleted.\nCommitted to gitops repo.", input.Name)
		return textResult(appendSyncStatus(ctx, deps, sess, result))
	})
}
