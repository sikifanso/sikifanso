package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// Deps holds shared dependencies for all MCP tool handlers.
type Deps struct {
	Logger *zap.Logger
}

// NewServer creates an MCP server with all sikifanso tools registered.
func NewServer(deps *Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "sikifanso",
		Version: "0.2.0",
	}, &mcp.ServerOptions{
		Instructions: "sikifanso manages Kubernetes clusters for AI agent infrastructure. " +
			"Tools that operate on a specific cluster require a 'cluster' parameter. " +
			"Use cluster_list to discover available clusters.",
	})

	registerClusterTools(s, deps)
	registerCatalogTools(s, deps)
	registerAgentTools(s, deps)
	registerDoctorTools(s, deps)
	registerKubeTools(s, deps)

	return s
}

// Run starts the MCP server on stdio transport and blocks until the context is cancelled.
func Run(ctx context.Context, deps *Deps) error {
	s := NewServer(deps)
	return s.Run(ctx, &mcp.StdioTransport{})
}

// textResult creates a simple text CallToolResult.
func textResult(text string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil, nil
}

// errResult creates an error CallToolResult (isError=true).
func errResult(err error) (*mcp.CallToolResult, any, error) {
	r := &mcp.CallToolResult{}
	r.SetError(err)
	return r, nil, nil
}
