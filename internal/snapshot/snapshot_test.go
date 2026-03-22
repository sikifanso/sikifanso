package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/alicanalbayrak/sikifanso/internal/session"
)

// writeTestSession creates a minimal session.yaml inside home/clusters/<clusterName>/.
func writeTestSession(t *testing.T, home, clusterName, gitOpsPath string) {
	t.Helper()
	sess := &session.Session{
		ClusterName: clusterName,
		State:       "running",
		GitOpsPath:  gitOpsPath,
	}
	data, err := yaml.Marshal(sess)
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	dir := filepath.Join(home, "clusters", clusterName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "session.yaml"), data, 0o600); err != nil {
		t.Fatalf("write session file: %v", err)
	}
}

// createTestGitOpsDir creates a gitops directory with a few files for testing.
func createTestGitOpsDir(t *testing.T, base string) string {
	t.Helper()
	gitops := filepath.Join(base, "gitops")
	sub := filepath.Join(gitops, "catalog")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("create gitops dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitops, "root-app.yaml"), []byte("kind: Application\n"), 0o644); err != nil {
		t.Fatalf("write root-app.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "grafana.yaml"), []byte("enabled: true\n"), 0o644); err != nil {
		t.Fatalf("write grafana.yaml: %v", err)
	}
	return gitops
}

// buildTestArchive creates a minimal .tar.gz with a snapshot-meta.yaml inside.
func buildTestArchive(t *testing.T, archivePath string, meta Meta) {
	t.Helper()

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	data, err := yaml.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	hdr := &tar.Header{
		Name:    metaFile,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write data: %v", err)
	}
}

// tarEntryNames opens a .tar.gz and returns all entry names.
func tarEntryNames(t *testing.T, archivePath string) []string {
	t.Helper()
	f, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gr.Close() }()

	var names []string
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		names = append(names, hdr.Name)
	}
	return names
}

// --- Tests ---

func TestSnapshotsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	dir, err := SnapshotsDir()
	if err != nil {
		t.Fatalf("SnapshotsDir: %v", err)
	}

	expected := filepath.Join(tmpDir, "snapshots")
	if dir != expected {
		t.Fatalf("expected %s, got %s", expected, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat snapshots dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("snapshots path is not a directory")
	}

	// Idempotency: call again, should succeed with same result.
	dir2, err := SnapshotsDir()
	if err != nil {
		t.Fatalf("SnapshotsDir (second call): %v", err)
	}
	if dir2 != expected {
		t.Fatalf("expected %s on second call, got %s", expected, dir2)
	}
}

func TestDelete(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	snapshotsPath := filepath.Join(tmpDir, "snapshots")
	if err := os.MkdirAll(snapshotsPath, 0o755); err != nil {
		t.Fatalf("create snapshots dir: %v", err)
	}

	// Create a fake archive.
	fakeName := "test-snap"
	fakePath := filepath.Join(snapshotsPath, fakeName+".tar.gz")
	if err := os.WriteFile(fakePath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("write fake archive: %v", err)
	}

	// Delete should succeed.
	if err := Delete(fakeName); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// File should be gone.
	if _, err := os.Stat(fakePath); !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, stat error: %v", err)
	}

	// Deleting again should return an error.
	if err := Delete(fakeName); err == nil {
		t.Fatal("expected error deleting nonexistent snapshot")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	err := Delete("does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot")
	}
}

func TestCapture(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	clusterName := "test-cluster"
	gitopsDir := createTestGitOpsDir(t, tmpDir)
	writeTestSession(t, tmpDir, clusterName, gitopsDir)

	archivePath, err := Capture(clusterName, "my-snapshot", "v0.1.0")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// Verify archive exists.
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("archive not found: %v", err)
	}

	// Open archive and verify it contains the expected entries.
	names := tarEntryNames(t, archivePath)
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	required := []string{metaFile, sessionFile}
	for _, r := range required {
		if !nameSet[r] {
			t.Errorf("archive missing entry %q; entries: %v", r, names)
		}
	}

	// Verify gitops files are present (under gitops/ prefix).
	foundRootApp := false
	foundGrafana := false
	for _, n := range names {
		if n == "gitops/root-app.yaml" {
			foundRootApp = true
		}
		if n == "gitops/catalog/grafana.yaml" {
			foundGrafana = true
		}
	}
	if !foundRootApp {
		t.Errorf("archive missing gitops/root-app.yaml; entries: %v", names)
	}
	if !foundGrafana {
		t.Errorf("archive missing gitops/catalog/grafana.yaml; entries: %v", names)
	}

	// Verify snapshot-meta.yaml content.
	m, err := readMeta(archivePath)
	if err != nil {
		t.Fatalf("readMeta: %v", err)
	}
	if m.Name != "my-snapshot" {
		t.Errorf("meta name: got %q, want %q", m.Name, "my-snapshot")
	}
	if m.ClusterName != clusterName {
		t.Errorf("meta clusterName: got %q, want %q", m.ClusterName, clusterName)
	}
	if m.CLIVersion != "v0.1.0" {
		t.Errorf("meta cliVersion: got %q, want %q", m.CLIVersion, "v0.1.0")
	}
}

func TestList(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	snapshotsPath := filepath.Join(tmpDir, "snapshots")
	if err := os.MkdirAll(snapshotsPath, 0o755); err != nil {
		t.Fatalf("create snapshots dir: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	earlier := now.Add(-1 * time.Hour)

	// Create two valid snapshot archives.
	buildTestArchive(t, filepath.Join(snapshotsPath, "snap-a.tar.gz"), Meta{
		Name:        "snap-a",
		ClusterName: "c1",
		CreatedAt:   earlier,
		CLIVersion:  "v0.1.0",
	})
	buildTestArchive(t, filepath.Join(snapshotsPath, "snap-b.tar.gz"), Meta{
		Name:        "snap-b",
		ClusterName: "c2",
		CreatedAt:   now,
		CLIVersion:  "v0.2.0",
	})

	// Also create a non-tar.gz file that should be ignored.
	if err := os.WriteFile(filepath.Join(snapshotsPath, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatalf("write junk file: %v", err)
	}

	metas, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(metas) != 2 {
		t.Fatalf("expected 2 metas, got %d", len(metas))
	}

	// Sorted by CreatedAt descending: snap-b (now) before snap-a (earlier).
	if metas[0].Name != "snap-b" {
		t.Errorf("first meta name: got %q, want %q", metas[0].Name, "snap-b")
	}
	if metas[1].Name != "snap-a" {
		t.Errorf("second meta name: got %q, want %q", metas[1].Name, "snap-a")
	}
}

func TestListNoDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	// Do NOT create the snapshots directory.
	metas, err := List()
	if err != nil {
		t.Fatalf("List with no directory: %v", err)
	}
	if metas != nil {
		t.Fatalf("expected nil, got %v", metas)
	}
}

func TestRestoreRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	clusterName := "roundtrip-cluster"
	gitopsDir := createTestGitOpsDir(t, tmpDir)
	writeTestSession(t, tmpDir, clusterName, gitopsDir)

	snapshotName := "roundtrip-snap"
	_, err := Capture(clusterName, snapshotName, "v1.0.0")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}

	// Now remove the original session and gitops to simulate a clean restore.
	clusterDir := filepath.Join(tmpDir, "clusters", clusterName)
	if err := os.RemoveAll(clusterDir); err != nil {
		t.Fatalf("remove cluster dir: %v", err)
	}

	sess, gitopsPath, err := Restore(snapshotName)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// Verify session fields.
	if sess.ClusterName != clusterName {
		t.Errorf("restored clusterName: got %q, want %q", sess.ClusterName, clusterName)
	}
	if sess.State != "running" {
		t.Errorf("restored state: got %q, want %q", sess.State, "running")
	}

	// Verify gitops files were restored.
	rootApp := filepath.Join(gitopsPath, "root-app.yaml")
	data, err := os.ReadFile(rootApp)
	if err != nil {
		t.Fatalf("read restored root-app.yaml: %v", err)
	}
	if string(data) != "kind: Application\n" {
		t.Errorf("root-app.yaml content: got %q", string(data))
	}

	grafana := filepath.Join(gitopsPath, "catalog", "grafana.yaml")
	data, err = os.ReadFile(grafana)
	if err != nil {
		t.Fatalf("read restored grafana.yaml: %v", err)
	}
	if string(data) != "enabled: true\n" {
		t.Errorf("grafana.yaml content: got %q", string(data))
	}

	// Verify session was persisted on disk.
	loaded, err := session.Load(clusterName)
	if err != nil {
		t.Fatalf("session.Load after restore: %v", err)
	}
	if loaded.GitOpsPath != gitopsPath {
		t.Errorf("loaded GitOpsPath: got %q, want %q", loaded.GitOpsPath, gitopsPath)
	}
}

func TestRestoreNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("SIKIFANSO_HOME", tmpDir)

	_, _, err := Restore("no-such-snapshot")
	if err == nil {
		t.Fatal("expected error restoring nonexistent snapshot")
	}
}
