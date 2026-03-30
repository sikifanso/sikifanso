package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.uber.org/zap"
)

// newTestClient creates a connected MCP server+client pair for testing.
// Returns the client session; both sessions are cleaned up when the test ends.
func newTestClient(t *testing.T) *mcp.ClientSession {
	t.Helper()
	deps := &Deps{Logger: zap.NewNop()}
	s := NewServer(deps)

	t1, t2 := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), t1, nil)
	if err != nil {
		t.Fatalf("Server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	c := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0"}, nil)
	cs, err := c.Connect(context.Background(), t2, nil)
	if err != nil {
		t.Fatalf("Client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

func TestNewServer_RegistersAllTools(t *testing.T) {
	t.Parallel()
	cs := newTestClient(t)

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expected := []string{
		"agent_create", "agent_delete", "agent_info", "agent_list",
		"argocd_app_detail", "argocd_app_diff", "argocd_apps", "argocd_rollback",
		"catalog_disable", "catalog_enable", "catalog_list",
		"cluster_create", "cluster_delete", "cluster_info", "cluster_list", "cluster_start_stop",
		"doctor",
		"kube_events", "kube_logs", "kube_pods", "kube_services",
		"profile_apply", "profile_list",
	}

	if len(result.Tools) != len(expected) {
		t.Fatalf("got %d tools, want %d", len(result.Tools), len(expected))
	}

	toolNames := make(map[string]bool)
	for _, tool := range result.Tools {
		toolNames[tool.Name] = true
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing tool: %s", name)
		}
	}
}

func TestNewServer_ToolsHaveDescriptions(t *testing.T) {
	t.Parallel()
	cs := newTestClient(t)

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range result.Tools {
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
	}
}

func TestNewServer_GlobalToolsHaveNoRequiredParams(t *testing.T) {
	t.Parallel()
	cs := newTestClient(t)

	result, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	globalTools := map[string]bool{"cluster_list": true, "profile_list": true}

	for _, tool := range result.Tools {
		if !globalTools[tool.Name] {
			continue
		}
		schema := tool.InputSchema
		if schema == nil {
			continue
		}
		b, err := json.Marshal(schema)
		if err != nil {
			t.Fatalf("marshal schema for %q: %v", tool.Name, err)
		}
		var schemaMap map[string]any
		if err := json.Unmarshal(b, &schemaMap); err != nil {
			t.Fatalf("unmarshal schema for %q: %v", tool.Name, err)
		}
		if req, ok := schemaMap["required"]; ok {
			if reqArr, ok := req.([]any); ok && len(reqArr) > 0 {
				t.Errorf("tool %q should have no required params, got %v", tool.Name, reqArr)
			}
		}
	}
}

func TestTextResult(t *testing.T) {
	t.Parallel()
	result, out, err := textResult("hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got %v", out)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "hello world" {
		t.Errorf("text = %q, want %q", tc.Text, "hello world")
	}
}

func TestErrResult(t *testing.T) {
	t.Parallel()
	result, out, err := errResult(fmt.Errorf("something broke"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil output, got %v", out)
	}
	if !result.IsError {
		t.Error("expected IsError=true")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if tc.Text != "something broke" {
		t.Errorf("text = %q, want %q", tc.Text, "something broke")
	}
}
