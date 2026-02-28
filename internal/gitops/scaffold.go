package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"go.uber.org/zap"
)

// DefaultBootstrapURL is the default template repository for GitOps scaffolding.
const DefaultBootstrapURL = "https://github.com/sikifanso/sikifanso-homelab-bootstrap.git"

// ScaffoldOptions configures bootstrap repo cloning.
type ScaffoldOptions struct {
	RepoURL string
	Version string // tag to clone; "" means HEAD
}

// Scaffold clones the bootstrap template repo into targetDir, strips
// upstream history, and creates a fresh initial commit.
func Scaffold(ctx context.Context, log *zap.Logger, targetDir string, opts ScaffoldOptions) error {
	log.Info("cloning bootstrap repo",
		zap.String("url", opts.RepoURL),
		zap.String("version", opts.Version),
		zap.String("target", targetDir),
	)

	cloneOpts := &git.CloneOptions{
		URL:   opts.RepoURL,
		Depth: 1,
	}
	if opts.Version != "" {
		cloneOpts.ReferenceName = plumbing.NewTagReferenceName(opts.Version)
		cloneOpts.SingleBranch = true
	}

	_, err := git.PlainCloneContext(ctx, targetDir, false, cloneOpts)
	if err != nil {
		if opts.Version != "" {
			return fmt.Errorf("cloning bootstrap repo at tag %s: %w", opts.Version, err)
		}
		return fmt.Errorf("cloning bootstrap repo: %w", err)
	}

	// Ensure app directories exist (custom bootstrap repos may omit them).
	for _, sub := range []string{
		filepath.Join("apps", "coordinates"),
		filepath.Join("apps", "values"),
	} {
		if err := os.MkdirAll(filepath.Join(targetDir, sub), 0755); err != nil {
			return fmt.Errorf("creating %s directory: %w", sub, err)
		}
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

	commitMsg := fmt.Sprintf("Initial scaffold from %s", opts.RepoURL)
	if opts.Version != "" {
		commitMsg = fmt.Sprintf("Initial scaffold from %s @ %s", opts.RepoURL, opts.Version)
	}

	_, err = w.Commit(commitMsg, &git.CommitOptions{
		Author: botSignature(),
	})
	if err != nil {
		return fmt.Errorf("creating initial commit: %w", err)
	}

	log.Info("gitops repo scaffolded", zap.String("path", targetDir))
	return nil
}
