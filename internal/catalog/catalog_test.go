package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeEntry writes a catalog YAML file into <dir>/catalog/<name>.yaml.
func writeEntry(t *testing.T, dir string, content string, name string) {
	t.Helper()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("creating catalog dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(catalogDir, name+".yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("writing catalog file %s: %v", name, err)
	}
}

func TestList_ReturnsAllEntries(t *testing.T) {
	dir := t.TempDir()

	writeEntry(t, dir, `
name: prometheus-stack
category: monitoring
description: Kubernetes monitoring stack
repoURL: https://prometheus-community.github.io/helm-charts
chart: kube-prometheus-stack
targetRevision: ">=84.0.0 <85.0.0"
namespace: monitoring
enabled: false
`, "prometheus-stack")

	writeEntry(t, dir, `
name: gitea
category: scm
description: Self-hosted Git service
repoURL: https://dl.gitea.io/charts
chart: gitea
targetRevision: ">=10.0.0 <11.0.0"
namespace: gitea
enabled: true
`, "gitea")

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("List returned %d entries, want 2", len(entries))
	}
}

func TestList_IgnoresSubdirectoriesAndNonYAML(t *testing.T) {
	dir := t.TempDir()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("creating catalog dir: %v", err)
	}

	// Write a valid YAML entry.
	writeEntry(t, dir, `
name: grafana
category: monitoring
description: Grafana dashboards
repoURL: https://grafana.github.io/helm-charts
chart: grafana
targetRevision: ">=8.0.0 <9.0.0"
namespace: monitoring
enabled: false
`, "grafana")

	// Create a subdirectory that should be skipped.
	if err := os.MkdirAll(filepath.Join(catalogDir, "values"), 0755); err != nil {
		t.Fatalf("creating values subdir: %v", err)
	}
	// Place a YAML file inside the subdir — should not be read.
	if err := os.WriteFile(filepath.Join(catalogDir, "values", "grafana.yaml"), []byte("somekey: val\n"), 0644); err != nil {
		t.Fatalf("writing subdir file: %v", err)
	}
	// Place a non-YAML file at the top level — should be skipped.
	if err := os.WriteFile(filepath.Join(catalogDir, "README.md"), []byte("# catalog\n"), 0644); err != nil {
		t.Fatalf("writing README: %v", err)
	}

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("List returned %d entries, want 1", len(entries))
	}
	if entries[0].Name != "grafana" {
		t.Errorf("entry name = %q, want %q", entries[0].Name, "grafana")
	}
}

func TestList_EmptyCatalogDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "catalog"), 0755); err != nil {
		t.Fatalf("creating catalog dir: %v", err)
	}

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List returned %d entries, want 0", len(entries))
	}
}

func TestList_MissingCatalogDir(t *testing.T) {
	dir := t.TempDir()

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List with missing catalog dir should not error, got: %v", err)
	}
	if entries != nil {
		t.Errorf("List = %v, want nil", entries)
	}
}

func TestFind_ReturnsCorrectEntry(t *testing.T) {
	dir := t.TempDir()

	writeEntry(t, dir, `
name: alertmanager
category: monitoring
description: Alertmanager
repoURL: https://prometheus-community.github.io/helm-charts
chart: alertmanager
targetRevision: ">=1.0.0 <2.0.0"
namespace: monitoring
enabled: false
`, "alertmanager")

	writeEntry(t, dir, `
name: gitea
category: scm
description: Self-hosted Git service
repoURL: https://dl.gitea.io/charts
chart: gitea
targetRevision: ">=10.0.0 <11.0.0"
namespace: gitea
enabled: true
`, "gitea")

	entry, err := Find(dir, "gitea")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if entry.Name != "gitea" {
		t.Errorf("entry.Name = %q, want %q", entry.Name, "gitea")
	}
	if entry.Namespace != "gitea" {
		t.Errorf("entry.Namespace = %q, want %q", entry.Namespace, "gitea")
	}
	if !entry.Enabled {
		t.Errorf("entry.Enabled = false, want true")
	}
}

func TestFind_NotFoundErrorListsAvailable(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"alertmanager", "gitea", "grafana"} {
		writeEntry(t, dir, "name: "+name+"\ncategory: test\ndescription: test\nrepoURL: https://example.com\nchart: "+name+"\ntargetRevision: \"1.0.0\"\nnamespace: default\nenabled: false\n", name)
	}

	_, err := Find(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent app, got nil")
	}

	msg := err.Error()
	if !strings.Contains(msg, "nonexistent") {
		t.Errorf("error %q does not contain app name %q", msg, "nonexistent")
	}
	for _, name := range []string{"alertmanager", "gitea", "grafana"} {
		if !strings.Contains(msg, name) {
			t.Errorf("error %q does not contain available name %q", msg, name)
		}
	}
}

func TestSetEnabled_FlipsTrueAndWritesToDisk(t *testing.T) {
	dir := t.TempDir()

	writeEntry(t, dir, `
name: prometheus-stack
category: monitoring
description: Kubernetes monitoring stack
repoURL: https://prometheus-community.github.io/helm-charts
chart: kube-prometheus-stack
targetRevision: ">=84.0.0 <85.0.0"
namespace: monitoring
enabled: false
`, "prometheus-stack")

	if err := SetEnabled(dir, "prometheus-stack", true); err != nil {
		t.Fatalf("SetEnabled(true): %v", err)
	}

	entry, err := Find(dir, "prometheus-stack")
	if err != nil {
		t.Fatalf("Find after SetEnabled: %v", err)
	}
	if !entry.Enabled {
		t.Errorf("entry.Enabled = false after SetEnabled(true), want true")
	}
	// Verify other fields are preserved.
	if entry.Chart != "kube-prometheus-stack" {
		t.Errorf("entry.Chart = %q, want %q", entry.Chart, "kube-prometheus-stack")
	}
	if entry.Namespace != "monitoring" {
		t.Errorf("entry.Namespace = %q, want %q", entry.Namespace, "monitoring")
	}
}

func TestSetEnabled_FlipsFalseAndWritesToDisk(t *testing.T) {
	dir := t.TempDir()

	writeEntry(t, dir, `
name: grafana
category: monitoring
description: Grafana dashboards
repoURL: https://grafana.github.io/helm-charts
chart: grafana
targetRevision: ">=8.0.0 <9.0.0"
namespace: monitoring
enabled: true
`, "grafana")

	if err := SetEnabled(dir, "grafana", false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}

	entry, err := Find(dir, "grafana")
	if err != nil {
		t.Fatalf("Find after SetEnabled: %v", err)
	}
	if entry.Enabled {
		t.Errorf("entry.Enabled = true after SetEnabled(false), want false")
	}
}

func TestSetEnabled_NonexistentReturnsError(t *testing.T) {
	dir := t.TempDir()

	writeEntry(t, dir, `
name: grafana
category: monitoring
description: Grafana dashboards
repoURL: https://grafana.github.io/helm-charts
chart: grafana
targetRevision: ">=8.0.0 <9.0.0"
namespace: monitoring
enabled: false
`, "grafana")

	err := SetEnabled(dir, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent app, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error %q does not mention the app name", err.Error())
	}
}

func TestCatalogDir(t *testing.T) {
	got := CatalogDir("/some/gitops/path")
	want := "/some/gitops/path/catalog"
	if got != want {
		t.Errorf("CatalogDir = %q, want %q", got, want)
	}
}
