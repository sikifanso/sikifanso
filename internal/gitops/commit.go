package gitops

import (
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Commit stages the given paths and creates a commit in the gitops repo.
func Commit(repoDir, message string, paths ...string) error {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return fmt.Errorf("opening git repo: %w", err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("getting worktree: %w", err)
	}

	for _, p := range paths {
		if _, err := w.Add(p); err != nil {
			return fmt.Errorf("staging %s: %w", p, err)
		}
	}

	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "sikifanso",
			Email: "sikifanso@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("creating commit: %w", err)
	}

	return nil
}
