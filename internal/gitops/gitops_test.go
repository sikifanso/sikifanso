package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// Commit tests
// ---------------------------------------------------------------------------

func TestCommit_SuccessfulCommitWithStagedFiles(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)

	// Create a file to commit.
	filePath := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(filePath, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Commit(dir, "add hello.txt", "hello.txt"); err != nil {
		t.Fatalf("Commit error: %v", err)
	}

	// Verify the commit exists in the log.
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	if commit.Message != "add hello.txt" {
		t.Errorf("commit message = %q, want %q", commit.Message, "add hello.txt")
	}
}

func TestCommit_VerifyAuthorSignature(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "sig.txt"), []byte("sig"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Commit(dir, "test signature", "sig.txt"); err != nil {
		t.Fatal(err)
	}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}

	if commit.Author.Name != "sikifanso" {
		t.Errorf("author name = %q, want %q", commit.Author.Name, "sikifanso")
	}
	if commit.Author.Email != "sikifanso@local" {
		t.Errorf("author email = %q, want %q", commit.Author.Email, "sikifanso@local")
	}
}

func TestCommit_MultipleFiles(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)

	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := Commit(dir, "add two files", "a.txt", "b.txt"); err != nil {
		t.Fatalf("Commit error: %v", err)
	}

	// Verify both files are in the commit tree.
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt"} {
		if _, err := tree.File(name); err != nil {
			t.Errorf("file %s not found in commit tree: %v", name, err)
		}
	}
}

func TestCommit_ErrorWhenNotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // Not a git repo.

	err := Commit(dir, "should fail")
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
	if !strings.Contains(err.Error(), "opening git repo") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCommit_ErrorWhenPathDoesNotExist(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)

	err := Commit(dir, "should fail", "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Scaffold tests
// ---------------------------------------------------------------------------

func TestScaffold_ClonesAndReinitializesRepo(t *testing.T) {
	t.Parallel()
	seedDir := createSeedRepo(t)
	targetDir := filepath.Join(t.TempDir(), "scaffolded")

	log := zap.NewNop()
	err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: seedDir,
	})
	if err != nil {
		t.Fatalf("Scaffold error: %v", err)
	}

	// .git directory should exist (fresh repo).
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); err != nil {
		t.Fatalf(".git directory missing: %v", err)
	}

	// Verify history has exactly one commit (fresh init).
	repo, err := git.PlainOpen(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	iter, err := repo.Log(&git.LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	_ = iter.ForEach(func(_ *object.Commit) error {
		count++
		return nil
	})
	if count != 1 {
		t.Errorf("expected 1 commit (fresh history), got %d", count)
	}
}

func TestScaffold_CreatesRequiredSubdirectories(t *testing.T) {
	t.Parallel()
	seedDir := createSeedRepo(t)
	targetDir := filepath.Join(t.TempDir(), "scaffolded")

	log := zap.NewNop()
	if err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: seedDir,
	}); err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{
		filepath.Join("apps", "coordinates"),
		filepath.Join("apps", "values"),
		"agents",
		filepath.Join("agents", "values"),
	} {
		info, err := os.Stat(filepath.Join(targetDir, sub))
		if err != nil {
			t.Errorf("expected directory %s to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s should be a directory", sub)
		}
	}
}

func TestScaffold_InitialCommitExists(t *testing.T) {
	t.Parallel()
	seedDir := createSeedRepo(t)
	targetDir := filepath.Join(t.TempDir(), "scaffolded")

	log := zap.NewNop()
	if err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: seedDir,
	}); err != nil {
		t.Fatal(err)
	}

	repo, err := git.PlainOpen(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("no HEAD reference: %v", err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(commit.Message, "Initial scaffold from") {
		t.Errorf("commit message = %q, want it to contain 'Initial scaffold from'", commit.Message)
	}
	if !strings.Contains(commit.Message, seedDir) {
		t.Errorf("commit message should reference the repo URL, got: %q", commit.Message)
	}
}

func TestScaffold_WithVersionTag(t *testing.T) {
	t.Parallel()
	seedDir := createSeedRepo(t)

	// Tag the seed repo.
	gitRun(t, seedDir, "tag", "v0.1.0")

	targetDir := filepath.Join(t.TempDir(), "scaffolded")
	log := zap.NewNop()
	if err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: seedDir,
		Version: "v0.1.0",
	}); err != nil {
		t.Fatal(err)
	}

	repo, err := git.PlainOpen(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}

	wantSubstr := "@ v0.1.0"
	if !strings.Contains(commit.Message, wantSubstr) {
		t.Errorf("commit message = %q, want it to contain %q", commit.Message, wantSubstr)
	}
}

func TestScaffold_PreservesFileContent(t *testing.T) {
	t.Parallel()
	seedDir := createSeedRepo(t)
	targetDir := filepath.Join(t.TempDir(), "scaffolded")

	log := zap.NewNop()
	if err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: seedDir,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(targetDir, "README.md"))
	if err != nil {
		t.Fatalf("README.md not found: %v", err)
	}
	if string(data) != "seed repo" {
		t.Errorf("file content = %q, want %q", string(data), "seed repo")
	}
}

func TestScaffold_ErrorWhenInvalidURL(t *testing.T) {
	t.Parallel()
	targetDir := filepath.Join(t.TempDir(), "scaffolded")
	log := zap.NewNop()

	err := Scaffold(context.Background(), log, targetDir, ScaffoldOptions{
		RepoURL: "/nonexistent/path/to/repo",
	})
	if err == nil {
		t.Fatal("expected error for invalid repo URL")
	}
	if !strings.Contains(err.Error(), "cloning bootstrap repo") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// initTestRepo creates a temp directory with an initialized git repo and an
// initial empty commit so that go-git can stage and commit subsequent changes.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")
	gitRun(t, dir, "commit", "--allow-empty", "-m", "init")
	return dir
}

// createSeedRepo creates a small git repo that can be used as a clone source
// (file:// URL) for Scaffold tests.
func createSeedRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@test.com")
	gitRun(t, dir, "config", "user.name", "test")

	// Write a file so the repo is non-empty.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("seed repo"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "seed commit")
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
