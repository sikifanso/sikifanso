package infraconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	t.Parallel()
	cfg := Defaults()

	if cfg.Platform.K3sImage != "rancher/k3s:v1.29.1-k3s2" {
		t.Errorf("K3sImage = %q, want rancher/k3s:v1.29.1-k3s2", cfg.Platform.K3sImage)
	}
	if cfg.Platform.Servers != 1 {
		t.Errorf("Servers = %d, want 1", cfg.Platform.Servers)
	}
	if cfg.Platform.Agents != 2 {
		t.Errorf("Agents = %d, want 2", cfg.Platform.Agents)
	}
	if cfg.Platform.NodePorts.HTTP != 30082 {
		t.Errorf("NodePorts.HTTP = %d, want 30082", cfg.Platform.NodePorts.HTTP)
	}
	if cfg.Platform.NodePorts.HTTPS != 30083 {
		t.Errorf("NodePorts.HTTPS = %d, want 30083", cfg.Platform.NodePorts.HTTPS)
	}
	if cfg.Platform.NodePorts.ArgoCDUI != 30080 {
		t.Errorf("NodePorts.ArgoCDUI = %d, want 30080", cfg.Platform.NodePorts.ArgoCDUI)
	}
	if cfg.Platform.NodePorts.HubbleUI != 30081 {
		t.Errorf("NodePorts.HubbleUI = %d, want 30081", cfg.Platform.NodePorts.HubbleUI)
	}
	if cfg.Cilium.RepoURL != "https://helm.cilium.io/" {
		t.Errorf("Cilium.RepoURL = %q", cfg.Cilium.RepoURL)
	}
	if cfg.Cilium.Chart != "cilium" {
		t.Errorf("Cilium.Chart = %q", cfg.Cilium.Chart)
	}
	if cfg.Cilium.Namespace != "kube-system" {
		t.Errorf("Cilium.Namespace = %q", cfg.Cilium.Namespace)
	}
	if cfg.ArgoCD.RepoURL != "https://argoproj.github.io/argo-helm" {
		t.Errorf("ArgoCD.RepoURL = %q", cfg.ArgoCD.RepoURL)
	}
	if cfg.ArgoCD.Chart != "argo-cd" {
		t.Errorf("ArgoCD.Chart = %q", cfg.ArgoCD.Chart)
	}
	if cfg.ArgoCD.Namespace != "argocd" {
		t.Errorf("ArgoCD.Namespace = %q", cfg.ArgoCD.Namespace)
	}
	if cfg.CiliumValues == nil {
		t.Fatal("CiliumValues is nil")
	}
	if cfg.ArgoCDValues == nil {
		t.Fatal("ArgoCDValues is nil")
	}
}

func TestLoadMissingDir(t *testing.T) {
	t.Parallel()
	cfg, err := Load("/nonexistent/path")
	if err != nil {
		t.Fatalf("Load returned error for missing dir: %v", err)
	}
	// Should return defaults
	if cfg.Platform.K3sImage != "rancher/k3s:v1.29.1-k3s2" {
		t.Errorf("expected defaults when dir is missing, got K3sImage=%q", cfg.Platform.K3sImage)
	}
}

func TestLoadPartialFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	if err := os.MkdirAll(infraDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Only provide platform.yaml with a custom image
	if err := os.WriteFile(filepath.Join(infraDir, "platform.yaml"), []byte(`
k3sImage: "rancher/k3s:v1.30.0-k3s1"
servers: 3
agents: 5
nodePorts:
  http: 30082
  https: 30083
  argocdUI: 30080
  hubbleUI: 30081
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	// Platform should reflect overrides
	if cfg.Platform.K3sImage != "rancher/k3s:v1.30.0-k3s1" {
		t.Errorf("K3sImage = %q, want custom", cfg.Platform.K3sImage)
	}
	if cfg.Platform.Servers != 3 {
		t.Errorf("Servers = %d, want 3", cfg.Platform.Servers)
	}
	if cfg.Platform.Agents != 5 {
		t.Errorf("Agents = %d, want 5", cfg.Platform.Agents)
	}

	// Other configs should retain defaults
	if cfg.Cilium.RepoURL != "https://helm.cilium.io/" {
		t.Errorf("Cilium.RepoURL should be default, got %q", cfg.Cilium.RepoURL)
	}
	if cfg.ArgoCD.Chart != "argo-cd" {
		t.Errorf("ArgoCD.Chart should be default, got %q", cfg.ArgoCD.Chart)
	}
}

func TestLoadFullFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	infraDir := filepath.Join(dir, "infra")
	if err := os.MkdirAll(infraDir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"platform.yaml": `
k3sImage: "rancher/k3s:v1.31.0-k3s1"
servers: 2
agents: 4
nodePorts:
  http: 31082
  https: 31083
  argocdUI: 31080
  hubbleUI: 31081
`,
		"cilium.yaml": `
repoURL: https://custom-helm.example.com/
chart: cilium-custom
targetRevision: "1.15.0"
namespace: cilium-ns
releaseName: my-cilium
`,
		"argocd.yaml": `
repoURL: https://custom-argo.example.com/
chart: argocd-custom
targetRevision: "6.0.0"
namespace: my-argocd
releaseName: my-argocd
`,
		"cilium-values.yaml": `
debug:
  enabled: true
`,
		"argocd-values.yaml": `
server:
  replicas: 2
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(infraDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.Platform.K3sImage != "rancher/k3s:v1.31.0-k3s1" {
		t.Errorf("K3sImage = %q", cfg.Platform.K3sImage)
	}
	if cfg.Platform.NodePorts.HTTP != 31082 {
		t.Errorf("NodePorts.HTTP = %d", cfg.Platform.NodePorts.HTTP)
	}
	if cfg.Cilium.RepoURL != "https://custom-helm.example.com/" {
		t.Errorf("Cilium.RepoURL = %q", cfg.Cilium.RepoURL)
	}
	if cfg.Cilium.Chart != "cilium-custom" {
		t.Errorf("Cilium.Chart = %q", cfg.Cilium.Chart)
	}
	if cfg.Cilium.TargetRevision != "1.15.0" {
		t.Errorf("Cilium.TargetRevision = %q", cfg.Cilium.TargetRevision)
	}
	if cfg.ArgoCD.Namespace != "my-argocd" {
		t.Errorf("ArgoCD.Namespace = %q", cfg.ArgoCD.Namespace)
	}

	// Values are deep-merged with defaults: user override is applied,
	// but default keys not in the override file are preserved.
	if cfg.CiliumValues["debug"] == nil {
		t.Error("CiliumValues[debug] is nil")
	}
	debugMap := cfg.CiliumValues["debug"].(map[string]interface{})
	if debugMap["enabled"] != true {
		t.Errorf("CiliumValues[debug][enabled] = %v, want true", debugMap["enabled"])
	}
	// Default keys should be preserved since we merge, not replace.
	if cfg.CiliumValues["kubeProxyReplacement"] == nil {
		t.Error("CiliumValues[kubeProxyReplacement] should be preserved from defaults")
	}
	// ArgoCD values: user added server.replicas, default keys should still be present.
	if cfg.ArgoCDValues["controller"] == nil {
		t.Error("ArgoCDValues[controller] should be preserved from defaults")
	}
}

func TestMergeValues(t *testing.T) {
	t.Parallel()
	base := map[string]interface{}{
		"a": "base-a",
		"b": map[string]interface{}{
			"b1": "base-b1",
			"b2": "base-b2",
		},
		"c": 42,
	}

	overrides := map[string]interface{}{
		"a": "override-a",
		"b": map[string]interface{}{
			"b2": "override-b2",
			"b3": "new-b3",
		},
		"d": "new-d",
	}

	result := MergeValues(base, overrides)

	if result["a"] != "override-a" {
		t.Errorf("a = %v, want override-a", result["a"])
	}
	if result["c"] != 42 {
		t.Errorf("c = %v, want 42", result["c"])
	}
	if result["d"] != "new-d" {
		t.Errorf("d = %v, want new-d", result["d"])
	}

	bMap, ok := result["b"].(map[string]interface{})
	if !ok {
		t.Fatal("b is not a map")
	}
	if bMap["b1"] != "base-b1" {
		t.Errorf("b.b1 = %v, want base-b1", bMap["b1"])
	}
	if bMap["b2"] != "override-b2" {
		t.Errorf("b.b2 = %v, want override-b2", bMap["b2"])
	}
	if bMap["b3"] != "new-b3" {
		t.Errorf("b.b3 = %v, want new-b3", bMap["b3"])
	}
}

func TestMergeValuesDoesNotMutateBase(t *testing.T) {
	t.Parallel()
	base := map[string]interface{}{
		"key": "original",
	}
	overrides := map[string]interface{}{
		"key": "changed",
	}

	MergeValues(base, overrides)

	if base["key"] != "original" {
		t.Error("MergeValues mutated the base map")
	}
}
