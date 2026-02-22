package session

import (
	"path/filepath"
	"testing"
	"time"
)

func setupTestHome(t *testing.T) {
	t.Helper()
	t.Setenv("SIKIFANSO_HOME", t.TempDir())
}

func newTestSession(name string) *Session {
	return &Session{
		ClusterName:  name,
		State:        "running",
		CreatedAt:    time.Now().Truncate(time.Second),
		BootstrapURL: "https://example.com/bootstrap.git",
		GitOpsPath:   "/local-gitops",
		Services: ServiceInfo{
			ArgoCD: ArgoCDInfo{
				URL:      "https://localhost:30080",
				Username: "admin",
				Password: "secret",
			},
		},
		K3dConfig: K3dConfigInfo{
			Image:   "rancher/k3s:latest",
			Servers: 1,
			Agents:  0,
		},
	}
}

func TestSaveAndLoad(t *testing.T) {
	setupTestHome(t)

	want := newTestSession("test-cluster")
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load("test-cluster")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.ClusterName != want.ClusterName {
		t.Errorf("ClusterName = %q, want %q", got.ClusterName, want.ClusterName)
	}
	if got.State != want.State {
		t.Errorf("State = %q, want %q", got.State, want.State)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if got.Services.ArgoCD.Password != want.Services.ArgoCD.Password {
		t.Errorf("ArgoCD password = %q, want %q", got.Services.ArgoCD.Password, want.Services.ArgoCD.Password)
	}
	if got.K3dConfig.Servers != want.K3dConfig.Servers {
		t.Errorf("Servers = %d, want %d", got.K3dConfig.Servers, want.K3dConfig.Servers)
	}
}

func TestLoadNotFound(t *testing.T) {
	setupTestHome(t)

	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("expected error loading nonexistent cluster, got nil")
	}
}

func TestListAll(t *testing.T) {
	setupTestHome(t)

	for _, name := range []string{"alpha", "bravo"} {
		if err := Save(newTestSession(name)); err != nil {
			t.Fatalf("Save(%s): %v", name, err)
		}
	}

	sessions, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("ListAll returned %d sessions, want 2", len(sessions))
	}

	names := map[string]bool{}
	for _, s := range sessions {
		names[s.ClusterName] = true
	}
	for _, want := range []string{"alpha", "bravo"} {
		if !names[want] {
			t.Errorf("ListAll missing cluster %q", want)
		}
	}
}

func TestListAllEmpty(t *testing.T) {
	setupTestHome(t)

	sessions, err := ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if sessions != nil {
		t.Errorf("ListAll = %v, want nil", sessions)
	}
}

func TestRemove(t *testing.T) {
	setupTestHome(t)

	s := newTestSession("doomed")
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Remove("doomed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := Load("doomed"); err == nil {
		t.Fatal("expected error after Remove, got nil")
	}
}

func TestDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmp)

	dir, err := Dir("my-cluster")
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}

	want := filepath.Join(tmp, "clusters", "my-cluster")
	if dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}
}
