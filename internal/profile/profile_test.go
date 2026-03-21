package profile

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
)

func TestList_ReturnsAllProfiles(t *testing.T) {
	profiles := List()
	if len(profiles) != len(registry) {
		t.Fatalf("List() returned %d profiles, want %d", len(profiles), len(registry))
	}
}

func TestList_SortedByName(t *testing.T) {
	profiles := List()
	for i := 1; i < len(profiles); i++ {
		if profiles[i].Name < profiles[i-1].Name {
			t.Errorf("profiles not sorted: %q before %q", profiles[i-1].Name, profiles[i].Name)
		}
	}
}

func TestGet_ExistingProfile(t *testing.T) {
	p, err := Get("agent-dev")
	if err != nil {
		t.Fatalf("Get(agent-dev) error: %v", err)
	}
	if p.Name != "agent-dev" {
		t.Errorf("Name = %q, want %q", p.Name, "agent-dev")
	}
	if len(p.Apps) == 0 {
		t.Error("Apps is empty")
	}
}

func TestGet_NotFound(t *testing.T) {
	_, err := Get("nonexistent")
	if err == nil {
		t.Fatal("Get(nonexistent) should return error")
	}
}

func TestResolve_SingleProfile(t *testing.T) {
	apps, err := Resolve("agent-minimal")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	want := registry["agent-minimal"].Apps
	if len(apps) != len(want) {
		t.Fatalf("got %d apps, want %d", len(apps), len(want))
	}
}

func TestResolve_CompositeProfile(t *testing.T) {
	apps, err := Resolve("agent-minimal,rag")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}

	// Should be the union of agent-minimal and rag, deduplicated.
	// postgresql appears in both — should only appear once.
	seen := make(map[string]int)
	for _, a := range apps {
		seen[a]++
		if seen[a] > 1 {
			t.Errorf("duplicate app: %s", a)
		}
	}

	// Verify all agent-minimal apps are present.
	for _, a := range registry["agent-minimal"].Apps {
		if seen[a] == 0 {
			t.Errorf("missing agent-minimal app: %s", a)
		}
	}
	// Verify all rag apps are present.
	for _, a := range registry["rag"].Apps {
		if seen[a] == 0 {
			t.Errorf("missing rag app: %s", a)
		}
	}
}

func TestResolve_InvalidProfile(t *testing.T) {
	_, err := Resolve("agent-minimal,nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid profile in composite")
	}
}

func TestResolve_EmptyString(t *testing.T) {
	apps, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected empty apps, got %d", len(apps))
	}
}

func TestNames_Sorted(t *testing.T) {
	names := Names()
	if len(names) != len(registry) {
		t.Fatalf("Names() returned %d, want %d", len(names), len(registry))
	}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("names not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestApply_EnablesAppsOnDisk(t *testing.T) {
	dir := t.TempDir()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"litellm-proxy", "langfuse", "postgresql"} {
		entry := catalog.Entry{
			Name:           name,
			Category:       "test",
			Description:    "test entry",
			RepoURL:        "https://example.com",
			Chart:          name,
			TargetRevision: "1.0.0",
			Namespace:      "test",
			Enabled:        false,
		}
		data, _ := yaml.Marshal(entry)
		if err := os.WriteFile(filepath.Join(catalogDir, name+".yaml"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	initGitRepo(t, dir)

	apps := []string{"litellm-proxy", "langfuse", "postgresql"}
	var warnings []string
	err := Apply(dir, "agent-minimal", apps, func(msg string) {
		warnings = append(warnings, msg)
	})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}

	for _, name := range apps {
		entry, err := catalog.Find(dir, name)
		if err != nil {
			t.Fatalf("Find(%s) error: %v", name, err)
		}
		if !entry.Enabled {
			t.Errorf("%s should be enabled", name)
		}
	}
}

func TestApply_SkipsMissingApps(t *testing.T) {
	dir := t.TempDir()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatal(err)
	}

	entry := catalog.Entry{
		Name:           "postgresql",
		Category:       "storage",
		Description:    "test",
		RepoURL:        "https://example.com",
		Chart:          "postgresql",
		TargetRevision: "1.0.0",
		Namespace:      "storage",
		Enabled:        false,
	}
	data, _ := yaml.Marshal(entry)
	if err := os.WriteFile(filepath.Join(catalogDir, "postgresql.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	initGitRepo(t, dir)

	var warnings []string
	err := Apply(dir, "test", []string{"nonexistent", "postgresql"}, func(msg string) {
		warnings = append(warnings, msg)
	})
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for nonexistent app, got %d", len(warnings))
	}

	e, _ := catalog.Find(dir, "postgresql")
	if !e.Enabled {
		t.Error("postgresql should be enabled")
	}
}

// initGitRepo initializes a git repo with an initial commit so gitops.Commit works.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "init")
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}
