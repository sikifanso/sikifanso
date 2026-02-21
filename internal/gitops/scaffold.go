package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"go.uber.org/zap"
)

// DefaultBootstrapURL is the default template repository for GitOps scaffolding.
const DefaultBootstrapURL = "https://github.com/sikifanso/sikifanso-homelab-bootstrap.git"

// Scaffold clones the bootstrap template repo into targetDir, strips
// upstream history, and creates a fresh initial commit.
func Scaffold(ctx context.Context, log *zap.Logger, repoURL, targetDir string) error {
	log.Info("cloning bootstrap repo", zap.String("url", repoURL), zap.String("target", targetDir))

	_, err := git.PlainCloneContext(ctx, targetDir, false, &git.CloneOptions{
		URL:   repoURL,
		Depth: 1,
	})
	if err != nil {
		return fmt.Errorf("cloning bootstrap repo: %w", err)
	}

	// Strip upstream history.
	gitDir := filepath.Join(targetDir, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return fmt.Errorf("removing .git directory: %w", err)
	}

	// Initialize a fresh repository.
	repo, err := git.PlainInit(targetDir, false)
	if err != nil {
		return fmt.Errorf("initializing git repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	if _, err := w.Add("."); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}

	_, err = w.Commit(fmt.Sprintf("Initial scaffold from %s", repoURL), &git.CommitOptions{
		Author: &object.Signature{
			Name:  "sikifanso",
			Email: "sikifanso@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("creating initial commit: %w", err)
	}

	log.Info("gitops repo scaffolded", zap.String("path", targetDir))
	return nil
}
