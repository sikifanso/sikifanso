package catalog

import (
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
)

// ToggleResult describes the outcome of a Toggle operation.
type ToggleResult struct {
	Name    string
	Enabled bool
	// NoChange is true when the entry was already in the desired state.
	NoChange bool
}

// Toggle finds a catalog entry by name, sets its enabled state, and commits
// the change to the gitops repo. If the entry is already in the desired state,
// it returns a result with NoChange=true and no commit is made.
//
// Callers are responsible for triggering ArgoCD sync after a successful toggle,
// as each surface (CLI, MCP, Dashboard) has different sync UX requirements.
func Toggle(gitOpsPath, name string, enable bool) (*ToggleResult, error) {
	entry, err := Find(gitOpsPath, name)
	if err != nil {
		return nil, err
	}

	if entry.Enabled == enable {
		return &ToggleResult{Name: name, Enabled: enable, NoChange: true}, nil
	}

	if err := SetEnabled(gitOpsPath, name, enable); err != nil {
		return nil, fmt.Errorf("setting enabled=%v for %s: %w", enable, name, err)
	}

	verb := "enable"
	if !enable {
		verb = "disable"
	}
	commitMsg := fmt.Sprintf("catalog: %s %s", verb, name)
	commitPath := fmt.Sprintf("catalog/%s.yaml", name)
	if err := gitops.Commit(gitOpsPath, commitMsg, commitPath); err != nil {
		return nil, fmt.Errorf("committing change: %w", err)
	}

	return &ToggleResult{Name: name, Enabled: enable}, nil
}

// Flip reads the current enabled state of the named entry and toggles it.
// This is a convenience for callers that don't know (or care about) the
// current state — e.g., the dashboard toggle button.
func Flip(gitOpsPath, name string) (*ToggleResult, error) {
	entry, err := Find(gitOpsPath, name)
	if err != nil {
		return nil, err
	}
	return Toggle(gitOpsPath, name, !entry.Enabled)
}
