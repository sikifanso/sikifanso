package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCreate_WritesEntryAndValues(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	err := Create(dir, CreateOpts{
		Name:          "my-agent",
		CPURequest:    "125m",
		CPULimit:      "500m",
		MemoryRequest: "128Mi",
		MemoryLimit:   "512Mi",
		Pods:          "5",
	})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	// Entry file exists.
	entryFile := filepath.Join(dir, "agents", "my-agent.yaml")
	data, err := os.ReadFile(entryFile)
	if err != nil {
		t.Fatalf("entry file not found: %v", err)
	}
	content := string(data)
	if !contains(content, "name: my-agent") {
		t.Error("entry missing name")
	}
	if !contains(content, "namespace: agent-my-agent") {
		t.Error("entry missing namespace")
	}
	if !contains(content, "chart: sikifanso-agent-template") {
		t.Error("entry missing chart")
	}

	// Values file exists with separate request/limit fields.
	valuesFile := filepath.Join(dir, "agents", "values", "my-agent.yaml")
	vData, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("values file not found: %v", err)
	}
	vContent := string(vData)
	if !contains(vContent, "cpuRequest: 125m") {
		t.Error("values missing cpuRequest")
	}
	if !contains(vContent, "cpuLimit: 500m") {
		t.Error("values missing cpuLimit")
	}
	if !contains(vContent, "memoryRequest: 128Mi") {
		t.Error("values missing memoryRequest")
	}
	if !contains(vContent, "memoryLimit: 512Mi") {
		t.Error("values missing memoryLimit")
	}
}

func TestCreate_DefaultValues(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	err := Create(dir, CreateOpts{Name: "default-agent"})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}

	valuesFile := filepath.Join(dir, "agents", "values", "default-agent.yaml")
	data, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("values file not found: %v", err)
	}
	content := string(data)
	if !contains(content, "cpuRequest: 250m") {
		t.Error("expected default cpuRequest 250m")
	}
	if !contains(content, "cpuLimit: 1000m") {
		t.Error("expected default cpuLimit 1000m")
	}
	if !contains(content, "memoryRequest: 256Mi") {
		t.Error("expected default memoryRequest 256Mi")
	}
	if !contains(content, "memoryLimit: 1Gi") {
		t.Error("expected default memoryLimit 1Gi")
	}
	if !contains(content, "pods: \"10\"") {
		t.Error("expected default pods 10")
	}
}

func TestCreate_DuplicateReturnsError(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	if err := Create(dir, CreateOpts{Name: "dup-agent"}); err != nil {
		t.Fatal(err)
	}
	err := Create(dir, CreateOpts{Name: "dup-agent"})
	if err == nil {
		t.Fatal("expected error for duplicate agent")
	}
}

func TestCreate_RequestExceedsLimitReturnsError(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	// CPU request > limit
	err := Create(dir, CreateOpts{Name: "bad-cpu", CPURequest: "2000m", CPULimit: "500m"})
	if err == nil {
		t.Fatal("expected error when cpuRequest > cpuLimit")
	}
	if !contains(err.Error(), "cpuRequest") || !contains(err.Error(), "exceeds") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Memory request > limit
	err = Create(dir, CreateOpts{Name: "bad-mem", MemoryRequest: "2Gi", MemoryLimit: "512Mi"})
	if err == nil {
		t.Fatal("expected error when memoryRequest > memoryLimit")
	}
	if !contains(err.Error(), "memoryRequest") || !contains(err.Error(), "exceeds") {
		t.Errorf("unexpected error message: %v", err)
	}

	// Invalid quantity format
	err = Create(dir, CreateOpts{Name: "bad-fmt", CPURequest: "notaunit"})
	if err == nil {
		t.Fatal("expected error for invalid quantity")
	}
}

func TestCreate_InvalidNameReturnsError(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	for _, name := range []string{"", "UPPER", "has space", "-starts-dash"} {
		err := Create(dir, CreateOpts{Name: name})
		if err == nil {
			t.Errorf("expected error for invalid name %q", name)
		}
	}
}

func TestList_ReturnsAgentsSorted(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)

	for _, name := range []string{"zulu", "alpha", "middle"} {
		if err := Create(dir, CreateOpts{Name: name}); err != nil {
			t.Fatal(err)
		}
	}

	agents, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 3 {
		t.Fatalf("got %d agents, want 3", len(agents))
	}
	if agents[0].Name != "alpha" || agents[1].Name != "middle" || agents[2].Name != "zulu" {
		t.Errorf("agents not sorted: %v", agents)
	}
}

func TestList_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)
	agents, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected empty, got %d", len(agents))
	}
}

func TestFind_Existing(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)
	if err := Create(dir, CreateOpts{Name: "found-me", CPULimit: "2000m"}); err != nil {
		t.Fatal(err)
	}

	info, err := Find(dir, "found-me")
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if info.Name != "found-me" {
		t.Errorf("Name = %q, want found-me", info.Name)
	}
	if info.CPULimit != "2000m" {
		t.Errorf("CPULimit = %q, want 2000m", info.CPULimit)
	}
}

func TestFind_NotFound(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)
	_, err := Find(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestDelete_RemovesFiles(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)
	if err := Create(dir, CreateOpts{Name: "doomed"}); err != nil {
		t.Fatal(err)
	}

	if err := Delete(dir, "doomed"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "agents", "doomed.yaml")); !os.IsNotExist(err) {
		t.Error("entry file should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "agents", "values", "doomed.yaml")); !os.IsNotExist(err) {
		t.Error("values file should be deleted")
	}
}

func TestDelete_NotFoundReturnsError(t *testing.T) {
	t.Parallel()
	dir := setupGitOps(t)
	err := Delete(dir, "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

// setupGitOps creates a temp directory with agents/ structure and initializes git.
func setupGitOps(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"agents", filepath.Join("agents", "values")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")
	// Create an initial commit (git needs at least one commit for go-git).
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
