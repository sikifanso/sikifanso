package catalog

import (
	"fmt"
	"strings"

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

// ToggleWithDepsResult describes the outcome of a ToggleWithDeps operation.
type ToggleWithDepsResult struct {
	Name     string
	Enabled  bool
	NoChange bool
	AutoDeps []string // dep names that were auto-enabled (enable path only)
}

// ToggleWithDeps is like Toggle but resolves transitive dependencies.
//
// Enable path: auto-enables missing dependencies, commits all changes together.
// Disable path: returns error listing dependents unless force is true.
// Force bypasses the dependent check but does NOT cascade-disable dependents.
func ToggleWithDeps(gitOpsPath, name string, enable, force bool) (*ToggleWithDepsResult, error) {
	entry, err := Find(gitOpsPath, name)
	if err != nil {
		return nil, err
	}

	if entry.Enabled == enable {
		return &ToggleWithDepsResult{Name: name, Enabled: enable, NoChange: true}, nil
	}

	if enable {
		return toggleWithDepsEnable(gitOpsPath, name)
	}
	return toggleWithDepsDisable(gitOpsPath, name, force)
}

func toggleWithDepsEnable(gitOpsPath, name string) (*ToggleWithDepsResult, error) {
	all, err := List(gitOpsPath)
	if err != nil {
		return nil, fmt.Errorf("listing catalog: %w", err)
	}

	resolved, _, err := ResolveDeps([]string{name}, all)
	if err != nil {
		return nil, fmt.Errorf("resolving dependencies: %w", err)
	}

	// Build a map of currently enabled entries to skip no-ops.
	enabledSet := make(map[string]bool, len(all))
	for _, e := range all {
		if e.Enabled {
			enabledSet[e.Name] = true
		}
	}

	var commitPaths []string
	var actualAutoAdded []string
	for _, app := range resolved {
		if enabledSet[app] {
			continue // already enabled
		}
		if err := SetEnabled(gitOpsPath, app, true); err != nil {
			return nil, fmt.Errorf("enabling %s: %w", app, err)
		}
		commitPaths = append(commitPaths, fmt.Sprintf("catalog/%s.yaml", app))
		if app != name {
			actualAutoAdded = append(actualAutoAdded, app)
		}
	}

	if len(commitPaths) > 0 {
		commitMsg := fmt.Sprintf("catalog: enable %s", name)
		if len(actualAutoAdded) > 0 {
			commitMsg += fmt.Sprintf(" (auto-deps: %s)", strings.Join(actualAutoAdded, ", "))
		}
		if err := gitops.Commit(gitOpsPath, commitMsg, commitPaths...); err != nil {
			return nil, fmt.Errorf("committing changes: %w", err)
		}
	}

	return &ToggleWithDepsResult{
		Name:     name,
		Enabled:  true,
		AutoDeps: actualAutoAdded,
	}, nil
}

func toggleWithDepsDisable(gitOpsPath, name string, force bool) (*ToggleWithDepsResult, error) {
	if !force {
		all, err := List(gitOpsPath)
		if err != nil {
			return nil, fmt.Errorf("listing catalog: %w", err)
		}
		deps := Dependents(name, all)
		if len(deps) > 0 {
			return nil, fmt.Errorf("cannot disable %s: required by %s (use --force to override)", name, strings.Join(deps, ", "))
		}
	}

	if err := SetEnabled(gitOpsPath, name, false); err != nil {
		return nil, fmt.Errorf("disabling %s: %w", name, err)
	}

	commitMsg := fmt.Sprintf("catalog: disable %s", name)
	commitPath := fmt.Sprintf("catalog/%s.yaml", name)
	if err := gitops.Commit(gitOpsPath, commitMsg, commitPath); err != nil {
		return nil, fmt.Errorf("committing change: %w", err)
	}

	return &ToggleWithDepsResult{Name: name, Enabled: false}, nil
}
