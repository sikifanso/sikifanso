package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// initGitRepo creates a temp directory with the required directory structure
// and initializes a git repo with an initial commit so gitops.Commit works.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{
		filepath.Join("apps", "coordinates"),
		filepath.Join("apps", "values"),
	} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")
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

func writeCoordinate(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, "apps", "coordinates", name+".yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// List tests
// ---------------------------------------------------------------------------

func TestList_ValidCoordinates(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	writeCoordinate(t, dir, "myapp", `
name: myapp
repoURL: https://example.com/charts
chart: myapp
targetRevision: "1.2.3"
namespace: default
`)

	writeCoordinate(t, dir, "other", `
name: other
repoURL: https://other.example.com
chart: other-chart
targetRevision: "0.1.0"
namespace: other-ns
`)

	apps, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}

	// Build a map for easy lookup (order is filesystem-dependent).
	m := make(map[string]AppInfo, len(apps))
	for _, a := range apps {
		m[a.Name] = a
	}

	a, ok := m["myapp"]
	if !ok {
		t.Fatal("myapp not found in results")
	}
	if a.RepoURL != "https://example.com/charts" {
		t.Errorf("RepoURL = %q, want https://example.com/charts", a.RepoURL)
	}
	if a.Chart != "myapp" {
		t.Errorf("Chart = %q, want myapp", a.Chart)
	}
	if a.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", a.Version)
	}
	if a.Namespace != "default" {
		t.Errorf("Namespace = %q, want default", a.Namespace)
	}
}

func TestList_EmptyDirectory(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	apps, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if apps != nil {
		t.Errorf("expected nil, got %v", apps)
	}
}

func TestList_MissingCoordinatesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // no apps/coordinates at all

	apps, err := List(dir)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if apps != nil {
		t.Errorf("expected nil, got %v", apps)
	}
}

func TestList_IgnoresNonYAMLFiles(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	writeCoordinate(t, dir, "real", `
name: real
repoURL: https://example.com
chart: real
targetRevision: "1.0.0"
namespace: default
`)

	// Write a non-YAML file in the coordinates directory.
	nonYAML := filepath.Join(dir, "apps", "coordinates", "README.txt")
	if err := os.WriteFile(nonYAML, []byte("not a yaml file"), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps, want 1", len(apps))
	}
	if apps[0].Name != "real" {
		t.Errorf("Name = %q, want real", apps[0].Name)
	}
}

func TestList_SkipsInvalidYAML(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	// Write valid coordinate.
	writeCoordinate(t, dir, "good", `
name: good
repoURL: https://example.com
chart: good
targetRevision: "1.0.0"
namespace: default
`)

	// Write invalid YAML.
	bad := filepath.Join(dir, "apps", "coordinates", "bad.yaml")
	if err := os.WriteFile(bad, []byte(":::not valid yaml\n\t{["), 0o644); err != nil {
		t.Fatal(err)
	}

	apps, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps, want 1 (bad yaml should be skipped)", len(apps))
	}
	if apps[0].Name != "good" {
		t.Errorf("Name = %q, want good", apps[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Add tests
// ---------------------------------------------------------------------------

func TestAdd_Success(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	err := Add(AddOpts{
		GitOpsPath: dir,
		Name:       "my-app",
		RepoURL:    "https://charts.example.com",
		Chart:      "my-app",
		Version:    "2.0.0",
		Namespace:  "apps",
	})
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}

	// Verify coordinates file.
	coordFile := filepath.Join(dir, "apps", "coordinates", "my-app.yaml")
	data, err := os.ReadFile(coordFile)
	if err != nil {
		t.Fatalf("coordinates file not found: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "name: my-app") {
		t.Error("coordinates missing name")
	}
	if !strings.Contains(content, "repoURL: https://charts.example.com") {
		t.Error("coordinates missing repoURL")
	}
	if !strings.Contains(content, "chart: my-app") {
		t.Error("coordinates missing chart")
	}
	if !strings.Contains(content, "targetRevision: 2.0.0") {
		t.Error("coordinates missing targetRevision")
	}
	if !strings.Contains(content, "namespace: apps") {
		t.Error("coordinates missing namespace")
	}

	// Verify values file.
	valuesFile := filepath.Join(dir, "apps", "values", "my-app.yaml")
	vData, err := os.ReadFile(valuesFile)
	if err != nil {
		t.Fatalf("values file not found: %v", err)
	}
	if !strings.Contains(string(vData), "# Helm values for my-app") {
		t.Error("values file missing expected comment")
	}
}

func TestAdd_InvalidName_Empty(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	err := Add(AddOpts{GitOpsPath: dir, Name: ""})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestAdd_InvalidName_Uppercase(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	err := Add(AddOpts{GitOpsPath: dir, Name: "MyApp"})
	if err == nil {
		t.Fatal("expected error for uppercase name")
	}
}

func TestAdd_InvalidName_SpecialChars(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	for _, name := range []string{"has space", "under_score", "dot.name", "-starts-dash"} {
		err := Add(AddOpts{GitOpsPath: dir, Name: name})
		if err == nil {
			t.Errorf("expected error for invalid name %q", name)
		}
	}
}

func TestAdd_Duplicate(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	opts := AddOpts{
		GitOpsPath: dir,
		Name:       "dup-app",
		RepoURL:    "https://example.com",
		Chart:      "dup",
		Version:    "1.0.0",
		Namespace:  "default",
	}

	if err := Add(opts); err != nil {
		t.Fatalf("first Add error: %v", err)
	}

	err := Add(opts)
	if err == nil {
		t.Fatal("expected error for duplicate app")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to mention 'already exists'", err.Error())
	}
}

func TestAdd_YAMLContentParseable(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	err := Add(AddOpts{
		GitOpsPath: dir,
		Name:       "parseable",
		RepoURL:    "https://example.com",
		Chart:      "parseable-chart",
		Version:    "3.1.4",
		Namespace:  "test-ns",
	})
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}

	// The file written by Add should be parseable by List.
	apps, err := List(dir)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("got %d apps, want 1", len(apps))
	}
	if apps[0].Name != "parseable" {
		t.Errorf("Name = %q, want parseable", apps[0].Name)
	}
	if apps[0].Version != "3.1.4" {
		t.Errorf("Version = %q, want 3.1.4", apps[0].Version)
	}
}

// ---------------------------------------------------------------------------
// Remove tests
// ---------------------------------------------------------------------------

func TestRemove_Success(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	// Add first, then remove.
	if err := Add(AddOpts{
		GitOpsPath: dir,
		Name:       "doomed",
		RepoURL:    "https://example.com",
		Chart:      "doomed",
		Version:    "1.0.0",
		Namespace:  "default",
	}); err != nil {
		t.Fatalf("Add error: %v", err)
	}

	if err := Remove(dir, "doomed"); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	// Coordinates file should be gone.
	coordFile := filepath.Join(dir, "apps", "coordinates", "doomed.yaml")
	if _, err := os.Stat(coordFile); !os.IsNotExist(err) {
		t.Error("coordinates file should be deleted")
	}

	// Values file should also be gone.
	valuesFile := filepath.Join(dir, "apps", "values", "doomed.yaml")
	if _, err := os.Stat(valuesFile); !os.IsNotExist(err) {
		t.Error("values file should be deleted")
	}
}

func TestRemove_NonexistentApp(t *testing.T) {
	t.Parallel()
	dir := initGitRepo(t)

	err := Remove(dir, "ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent app")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention 'not found'", err.Error())
	}
}
