# Rolling Sync + Dependency Graph Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix catalog app sync ordering so CRD-dependent apps (e.g. postgresql) wait for their operators (e.g. cnpg-operator) before syncing, both server-side (ArgoCD RollingSync) and CLI-side (dependency resolution on enable/disable).

**Architecture:** Two complementary mechanisms: (1) ArgoCD ApplicationSet RollingSync groups apps into ordered tiers via labels — tier 0 operators sync first, then tier 1 data, then tier 2 services, then unmatched apps last. (2) CLI-side dependency graph auto-enables transitive deps on `app enable` and blocks unsafe `app disable` unless `--force` is passed. Profile `Apply` resolves transitive deps before enabling.

**Tech Stack:** Go 1.25+, urfave/cli/v3, ArgoCD ApplicationSet v2.6+ (RollingSync/progressive syncs), sigs.k8s.io/yaml, gopkg.in/yaml.v3

**Spec:** `docs/superpowers/specs/2026-04-04-rolling-sync-design.md`

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `internal/catalog/catalog.go` | Modify | Add `Tier` and `DependsOn` fields to `Entry` struct |
| `internal/catalog/depgraph.go` | Create | `ResolveDeps`, `Dependents`, cycle detection |
| `internal/catalog/depgraph_test.go` | Create | Tests for dependency graph functions |
| `internal/catalog/service.go` | Modify | Add `ToggleWithDeps` alongside existing `Toggle` |
| `cmd/sikifanso/app_cmd.go` | Modify | Wire `ToggleWithDeps`, add `--force` flag to disable |
| `internal/profile/profile.go` | Modify | `Apply` returns `([]string, error)` with auto-added deps |
| `cmd/sikifanso/cluster_create.go` | Modify | Handle updated `Apply` return signature |
| `internal/mcp/helpers.go` | Modify | Handle updated `Apply` return signature |
| `internal/mcp/catalog.go` | Modify | Use `ToggleWithDeps` instead of `Toggle` |
| `internal/infraconfig/defaults/argocd-values.yaml` | Modify | Add `--enable-progressive-syncs` arg |
| `../sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml` | Modify | RollingSync strategy + tier label |
| `../sikifanso-homelab-bootstrap/catalog/*.yaml` (19 files) | Modify | Add `tier` and `dependsOn` fields |

---

### Task 1: Add Tier and DependsOn Fields to Entry Struct

**Files:**
- Modify: `internal/catalog/catalog.go:16-25`

- [ ] **Step 1: Add the two new fields to Entry**

In `internal/catalog/catalog.go`, add `Tier` and `DependsOn` after the `Enabled` field (line 24):

```go
type Entry struct {
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	RepoURL        string   `json:"repoURL"`
	Chart          string   `json:"chart"`
	TargetRevision string   `json:"targetRevision"`
	Namespace      string   `json:"namespace"`
	Enabled        bool     `json:"enabled"`
	Tier           string   `json:"tier,omitempty"`
	DependsOn      []string `json:"dependsOn,omitempty"`
}
```

- [ ] **Step 2: Run existing tests to verify backward compatibility**

Run: `cd sikifanso && go test ./internal/catalog/ -race -v`
Expected: All 10 existing tests PASS (omitempty means existing YAML without these fields still parses correctly)

- [ ] **Step 3: Commit**

```bash
git add internal/catalog/catalog.go
git commit -m "catalog: add Tier and DependsOn fields to Entry"
```

---

### Task 2: Implement Dependency Graph

**Files:**
- Create: `internal/catalog/depgraph.go`
- Create: `internal/catalog/depgraph_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/catalog/depgraph_test.go`:

```go
package catalog

import (
	"testing"
)

func TestResolveDeps_NoDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "ollama"},
		{Name: "qdrant"},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"ollama"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 1 || resolved[0] != "ollama" {
		t.Errorf("resolved = %v, want [ollama]", resolved)
	}
	if len(autoAdded) != 0 {
		t.Errorf("autoAdded = %v, want empty", autoAdded)
	}
}

func TestResolveDeps_DirectDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "prometheus-stack"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"postgresql"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	// Deps must come before the requested app.
	if len(resolved) != 3 {
		t.Fatalf("resolved = %v, want 3 entries", resolved)
	}
	// postgresql must be last.
	if resolved[len(resolved)-1] != "postgresql" {
		t.Errorf("last resolved = %q, want postgresql", resolved[len(resolved)-1])
	}
	if len(autoAdded) != 2 {
		t.Errorf("autoAdded = %v, want 2 entries", autoAdded)
	}
}

func TestResolveDeps_TransitiveDeps(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "prometheus-stack"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
		{Name: "langfuse", DependsOn: []string{"cnpg-operator", "postgresql"}},
	}
	resolved, autoAdded, err := ResolveDeps([]string{"langfuse"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	// Must include: cnpg-operator, prometheus-stack (transitive via postgresql), postgresql, langfuse.
	if len(resolved) != 4 {
		t.Fatalf("resolved = %v, want 4 entries", resolved)
	}
	if resolved[len(resolved)-1] != "langfuse" {
		t.Errorf("last resolved = %q, want langfuse", resolved[len(resolved)-1])
	}
	if len(autoAdded) != 3 {
		t.Errorf("autoAdded = %v, want 3 entries", autoAdded)
	}
}

func TestResolveDeps_AlreadyRequested(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator"},
		{Name: "postgresql", DependsOn: []string{"cnpg-operator"}},
	}
	// If cnpg-operator is already in the requested list, it should not appear in autoAdded.
	resolved, autoAdded, err := ResolveDeps([]string{"cnpg-operator", "postgresql"}, all)
	if err != nil {
		t.Fatalf("ResolveDeps: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %v, want 2 entries", resolved)
	}
	if len(autoAdded) != 0 {
		t.Errorf("autoAdded = %v, want empty (both were requested)", autoAdded)
	}
}

func TestResolveDeps_CycleDetection(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"a"}},
	}
	_, _, err := ResolveDeps([]string{"a"}, all)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
}

func TestResolveDeps_UnknownDep(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "postgresql", DependsOn: []string{"nonexistent"}},
	}
	_, _, err := ResolveDeps([]string{"postgresql"}, all)
	if err == nil {
		t.Fatal("expected error for unknown dep, got nil")
	}
}

func TestDependents_Direct(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator", Enabled: true},
		{Name: "postgresql", Enabled: true, DependsOn: []string{"cnpg-operator", "prometheus-stack"}},
		{Name: "langfuse", Enabled: true, DependsOn: []string{"cnpg-operator", "postgresql"}},
		{Name: "ollama", Enabled: true},
	}
	deps := Dependents("cnpg-operator", all)
	if len(deps) != 2 {
		t.Fatalf("Dependents = %v, want 2 entries (postgresql, langfuse)", deps)
	}
}

func TestDependents_OnlyEnabled(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "cnpg-operator", Enabled: true},
		{Name: "postgresql", Enabled: false, DependsOn: []string{"cnpg-operator"}},
	}
	deps := Dependents("cnpg-operator", all)
	if len(deps) != 0 {
		t.Errorf("Dependents = %v, want empty (postgresql is disabled)", deps)
	}
}

func TestDependents_NoDependents(t *testing.T) {
	t.Parallel()
	all := []Entry{
		{Name: "ollama", Enabled: true},
		{Name: "qdrant", Enabled: true},
	}
	deps := Dependents("ollama", all)
	if len(deps) != 0 {
		t.Errorf("Dependents = %v, want empty", deps)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd sikifanso && go test ./internal/catalog/ -run TestResolveDeps -race -v`
Expected: FAIL — `ResolveDeps` undefined

- [ ] **Step 3: Implement depgraph.go**

Create `internal/catalog/depgraph.go`:

```go
package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveDeps performs BFS transitive dependency resolution. Given a set of
// requested app names and all catalog entries, it returns:
//   - resolved: the full ordered set (deps before dependents)
//   - autoAdded: names that were not in the original requested set
//
// Returns an error if a cycle is detected or a dependency does not exist.
func ResolveDeps(requested []string, all []Entry) (resolved, autoAdded []string, err error) {
	byName := make(map[string]Entry, len(all))
	for _, e := range all {
		byName[e.Name] = e
	}

	requestedSet := make(map[string]bool, len(requested))
	for _, name := range requested {
		requestedSet[name] = true
	}

	// BFS with cycle detection via "visiting" state.
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int)
	var order []string

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("dependency cycle detected: %s -> %s", strings.Join(path, " -> "), name)
		}

		entry, ok := byName[name]
		if !ok {
			return fmt.Errorf("dependency %q not found in catalog", name)
		}

		state[name] = visiting
		for _, dep := range entry.DependsOn {
			if err := visit(dep, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = visited
		order = append(order, name)
		return nil
	}

	for _, name := range requested {
		if err := visit(name, nil); err != nil {
			return nil, nil, err
		}
	}

	// Determine which were auto-added (in order but not originally requested).
	var added []string
	for _, name := range order {
		if !requestedSet[name] {
			added = append(added, name)
		}
	}

	return order, added, nil
}

// Dependents returns the names of enabled entries that directly depend on the
// given app name. The result is sorted for deterministic output.
func Dependents(name string, all []Entry) []string {
	var deps []string
	for _, e := range all {
		if !e.Enabled {
			continue
		}
		for _, d := range e.DependsOn {
			if d == name {
				deps = append(deps, e.Name)
				break
			}
		}
	}
	sort.Strings(deps)
	return deps
}
```

- [ ] **Step 4: Run all depgraph tests**

Run: `cd sikifanso && go test ./internal/catalog/ -run "TestResolveDeps|TestDependents" -race -v`
Expected: All 9 tests PASS

- [ ] **Step 5: Run full catalog test suite**

Run: `cd sikifanso && go test ./internal/catalog/ -race -v`
Expected: All tests PASS (existing + new)

- [ ] **Step 6: Commit**

```bash
git add internal/catalog/depgraph.go internal/catalog/depgraph_test.go
git commit -m "catalog: add dependency graph with BFS resolution and cycle detection"
```

---

### Task 3: Add ToggleWithDeps to Service Layer

**Files:**
- Modify: `internal/catalog/service.go`

- [ ] **Step 1: Add ToggleWithDepsResult type and ToggleWithDeps function**

Add below the existing `Flip` function (after line 59) in `internal/catalog/service.go`:

```go
// ToggleWithDepsResult describes the outcome of a ToggleWithDeps operation.
type ToggleWithDepsResult struct {
	Name      string
	Enabled   bool
	NoChange  bool
	AutoDeps  []string // dep names that were auto-enabled (enable path only)
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

	resolved, autoAdded, err := ResolveDeps([]string{name}, all)
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
```

Also add `"strings"` to the import block at the top of `service.go`:

```go
import (
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
)
```

- [ ] **Step 2: Verify compilation**

Run: `cd sikifanso && go build ./internal/catalog/`
Expected: Success

- [ ] **Step 3: Run full test suite**

Run: `cd sikifanso && go test ./internal/catalog/ -race -v`
Expected: All tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/catalog/service.go
git commit -m "catalog: add ToggleWithDeps with dependency resolution and disable guard"
```

---

### Task 4: Wire CLI to Use ToggleWithDeps

**Files:**
- Modify: `cmd/sikifanso/app_cmd.go:318-369`

- [ ] **Step 1: Add --force flag to appDisableCmd**

In `cmd/sikifanso/app_cmd.go`, replace the `appDisableCmd` function (lines 318-329):

```go
func appDisableCmd() *cli.Command {
	return &cli.Command{
		Name:      "disable",
		Usage:     "Disable a catalog application",
		ArgsUsage: "NAME",
		Flags: append(waitSyncFlags(), &cli.BoolFlag{
			Name:  "force",
			Usage: "Bypass dependent-app safety check",
		}),
		Action: withSession(func(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
			return appToggleAction(ctx, cmd, sess, false)
		}),
		ShellComplete: catalogEnabledNamesComplete,
	}
}
```

- [ ] **Step 2: Replace catalog.Toggle with catalog.ToggleWithDeps in appToggleAction**

Replace the `appToggleAction` function body (lines 331-369):

```go
func appToggleAction(ctx context.Context, cmd *cli.Command, sess *session.Session, enable bool) error {
	verb := "enable"
	past := "enabled"
	if !enable {
		verb = "disable"
		past = "disabled"
	}

	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("app name is required: sikifanso app %s NAME", verb)
	}

	force := cmd.Bool("force")
	result, err := catalog.ToggleWithDeps(sess.GitOpsPath, name, enable, force)
	if err != nil {
		return err
	}
	if result.NoChange {
		fmt.Fprintf(os.Stderr, "%s is already %s\n", name, past)
		return nil
	}

	if len(result.AutoDeps) > 0 {
		fmt.Fprintf(os.Stderr, "auto-enabled: %s\n", strings.Join(result.AutoDeps, ", "))
	}

	fmt.Fprintf(os.Stderr, "%s committed to gitops repo\n", name)

	op := grpcsync.OpEnable
	if !enable {
		op = grpcsync.OpDisable
	}

	syncApps := []string{name}
	if len(result.AutoDeps) > 0 {
		syncApps = append(result.AutoDeps, name)
	}

	if err := syncAfterMutation(ctx, cmd, sess, MutationOpts{
		Operation:  op,
		Apps:       syncApps,
		AppSetName: "catalog",
	}); err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s %s ✓\n", color.GreenString(name), past)
	return nil
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd sikifanso && go build ./cmd/sikifanso/`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add cmd/sikifanso/app_cmd.go
git commit -m "cli: wire ToggleWithDeps for app enable/disable with --force flag"
```

---

### Task 5: Update Profile Apply to Return Auto-Added Deps

**Files:**
- Modify: `internal/profile/profile.go:112-130`

- [ ] **Step 1: Change Apply signature and add dependency resolution**

Replace the `Apply` function (lines 112-130) in `internal/profile/profile.go`:

```go
// Apply enables the given apps in the catalog at gitOpsPath and commits the
// changes in a single commit. Transitive dependencies are resolved and
// auto-enabled. Returns the names of auto-added dependencies.
//
// Apps that don't exist in the catalog are skipped with a warning via the
// provided warn function. The profileName is used in the commit message.
func Apply(gitOpsPath string, profileName string, apps []string, warn func(string)) ([]string, error) {
	all, err := catalog.List(gitOpsPath)
	if err != nil {
		return nil, fmt.Errorf("listing catalog: %w", err)
	}

	resolved, autoAdded, err := catalog.ResolveDeps(apps, all)
	if err != nil {
		return nil, fmt.Errorf("resolving dependencies: %w", err)
	}

	var committed []string
	for _, app := range resolved {
		if err := catalog.SetEnabled(gitOpsPath, app, true); err != nil {
			if warn != nil {
				warn(fmt.Sprintf("skipping %s: %s", app, err))
			}
			continue
		}
		committed = append(committed, fmt.Sprintf("catalog/%s.yaml", app))
	}

	if len(committed) == 0 {
		return nil, nil
	}

	msg := fmt.Sprintf("profile: enable %s apps", profileName)
	if err := gitops.Commit(gitOpsPath, msg, committed...); err != nil {
		return nil, err
	}
	return autoAdded, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd sikifanso && go build ./internal/profile/`
Expected: FAIL — callers of `Apply` don't handle the new return value yet. That's expected; we fix them in the next steps.

- [ ] **Step 3: Commit (partial — callers updated in next tasks)**

```bash
git add internal/profile/profile.go
git commit -m "profile: resolve transitive deps in Apply, return auto-added list"
```

---

### Task 6: Update Apply Call Sites

**Files:**
- Modify: `cmd/sikifanso/cluster_create.go:95`
- Modify: `internal/mcp/helpers.go:76`

- [ ] **Step 1: Update cluster_create.go**

In `cmd/sikifanso/cluster_create.go`, replace lines 95-99:

Old:
```go
		if err := profile.Apply(sess.GitOpsPath, profileStr, profileApps, func(msg string) {
			zapLogger.Warn(msg)
		}); err != nil {
			return fmt.Errorf("applying profile: %w", err)
		}
```

New:
```go
		autoAdded, err := profile.Apply(sess.GitOpsPath, profileStr, profileApps, func(msg string) {
			zapLogger.Warn(msg)
		})
		if err != nil {
			return fmt.Errorf("applying profile: %w", err)
		}
		if len(autoAdded) > 0 {
			zapLogger.Info("auto-enabled dependencies", zap.Strings("deps", autoAdded))
		}
```

- [ ] **Step 2: Update mcp/helpers.go**

In `internal/mcp/helpers.go`, replace lines 75-85:

Old:
```go
	var warnings []string
	if err := profile.Apply(sess.GitOpsPath, profileName, apps, func(msg string) {
		warnings = append(warnings, msg)
	}); err != nil {
		return "", fmt.Errorf("applying profile %q: %w", profileName, err)
	}

	result := fmt.Sprintf("Profile %q applied (%d apps enabled).", profileName, len(apps))
	for _, w := range warnings {
		result += fmt.Sprintf("\n  Warning: %s", w)
	}
```

New:
```go
	var warnings []string
	autoAdded, err := profile.Apply(sess.GitOpsPath, profileName, apps, func(msg string) {
		warnings = append(warnings, msg)
	})
	if err != nil {
		return "", fmt.Errorf("applying profile %q: %w", profileName, err)
	}

	result := fmt.Sprintf("Profile %q applied (%d apps enabled).", profileName, len(apps))
	if len(autoAdded) > 0 {
		result += fmt.Sprintf("\n  Auto-enabled dependencies: %s", strings.Join(autoAdded, ", "))
	}
	for _, w := range warnings {
		result += fmt.Sprintf("\n  Warning: %s", w)
	}
```

Also add `"strings"` to the import block of `internal/mcp/helpers.go` if not already present.

- [ ] **Step 3: Verify full build**

Run: `cd sikifanso && go build ./...`
Expected: Success — all callers now handle the updated return type

- [ ] **Step 4: Commit**

```bash
git add cmd/sikifanso/cluster_create.go internal/mcp/helpers.go
git commit -m "cli/mcp: handle updated profile.Apply return with auto-added deps"
```

---

### Task 7: Update MCP Catalog Toggle

**Files:**
- Modify: `internal/mcp/catalog.go:102-123`

- [ ] **Step 1: Replace catalog.Toggle with catalog.ToggleWithDeps**

In `internal/mcp/catalog.go`, replace the `catalogToggle` function (lines 102-123):

```go
func catalogToggle(ctx context.Context, deps *Deps, clusterName, appName string, enable bool) (*mcp.CallToolResult, any, error) {
	past := "enabled"
	if !enable {
		past = "disabled"
	}

	sess, r, sv, e := loadSession(clusterName)
	if sess == nil {
		return r, sv, e
	}

	// MCP has no --force equivalent — agents must disable dependents explicitly.
	result, err := catalog.ToggleWithDeps(sess.GitOpsPath, appName, enable, false)
	if err != nil {
		return errResult(err)
	}
	if result.NoChange {
		return textResult(fmt.Sprintf("%s is already %s.", appName, past))
	}

	msg := fmt.Sprintf("%s %s and committed to gitops repo.", appName, past)
	if len(result.AutoDeps) > 0 {
		msg += fmt.Sprintf(" Auto-enabled dependencies: %s.", strings.Join(result.AutoDeps, ", "))
	}
	return textResult(appendSyncStatus(ctx, deps, sess, msg, "catalog"))
}
```

Also add `"strings"` to the import block of `internal/mcp/catalog.go` if not already present.

- [ ] **Step 2: Verify build**

Run: `cd sikifanso && go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/mcp/catalog.go
git commit -m "mcp: use ToggleWithDeps for catalog enable/disable"
```

---

### Task 8: Enable Progressive Syncs in ArgoCD Config

**Files:**
- Modify: `internal/infraconfig/defaults/argocd-values.yaml:74-84`

- [ ] **Step 1: Add --enable-progressive-syncs flag**

In `internal/infraconfig/defaults/argocd-values.yaml`, replace lines 74-84:

Old:
```yaml
applicationSet:
  enabled: true
  extraArgs:
    - "--repo-server-plaintext"
  resources:
    requests:
      cpu: 25m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
```

New:
```yaml
applicationSet:
  enabled: true
  extraArgs:
    - "--repo-server-plaintext"
    - "--enable-progressive-syncs"
  resources:
    requests:
      cpu: 25m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 256Mi
```

- [ ] **Step 2: Verify build (embeds this file)**

Run: `cd sikifanso && go build ./...`
Expected: Success

- [ ] **Step 3: Commit**

```bash
git add internal/infraconfig/defaults/argocd-values.yaml
git commit -m "argocd: enable progressive syncs for ApplicationSet RollingSync"
```

---

### Task 9: Update root-catalog.yaml to RollingSync Strategy

**Files:**
- Modify: `../sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml`

- [ ] **Step 1: Rewrite root-catalog.yaml**

Replace the entire content of `sikifanso-homelab-bootstrap/bootstrap/root-catalog.yaml`:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: catalog
  namespace: argocd
spec:
  strategy:
    type: RollingSync
    rollingSync:
      steps:
        - matchExpressions:
            - key: tier
              operator: In
              values:
                - 0-operators
        - matchExpressions:
            - key: tier
              operator: In
              values:
                - 1-data
        - matchExpressions:
            - key: tier
              operator: In
              values:
                - 2-services
  generators:
    - git:
        repoURL: /local-gitops
        revision: HEAD
        files:
          - path: "catalog/*.yaml"
      selector:
        matchExpressions:
          - key: enabled
            operator: In
            values:
              - "true"
  template:
    metadata:
      name: "{{name}}"
      namespace: argocd
      labels:
        tier: "{{tier}}"
      finalizers:
        - resources-finalizer.argocd.argoproj.io
    spec:
      project: default
      sources:
        - repoURL: "{{repoURL}}"
          chart: "{{chart}}"
          targetRevision: "{{targetRevision}}"
          helm:
            releaseName: "{{name}}"
            valueFiles:
              - $values/catalog/values/{{name}}.yaml
        - repoURL: /local-gitops
          targetRevision: HEAD
          ref: values
      destination:
        server: https://kubernetes.default.svc
        namespace: "{{namespace}}"
      ignoreDifferences:
        - group: apps
          kind: StatefulSet
          jqPathExpressions:
            - .spec.volumeClaimTemplates[]?.apiVersion
            - .spec.volumeClaimTemplates[]?.kind
        - group: postgresql.cnpg.io
          kind: Cluster
          jqPathExpressions:
            - .spec.imageName
      syncPolicy:
        syncOptions:
          - CreateNamespace=true
          - ServerSideApply=true
          - RespectIgnoreDifferences=true
          - SkipDryRunOnMissingResource=true
        retry:
          limit: 3
          backoff:
            duration: 10s
            factor: 2
            maxDuration: 3m
```

Key changes from original:
- `syncPolicy.applicationsSync: sync` → `strategy.type: RollingSync` with 3 ordered steps
- Added `tier: "{{tier}}"` label to template metadata
- Removed `syncPolicy.automated` (RollingSync takes over sync triggering)
- Added `SkipDryRunOnMissingResource=true` to syncOptions
- Added `retry` policy for resilience

- [ ] **Step 2: Commit (in sikifanso-homelab-bootstrap repo)**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap
git add bootstrap/root-catalog.yaml
git commit -m "argocd: replace parallel sync with RollingSync tier ordering"
```

---

### Task 10: Add tier and dependsOn Fields to All 19 Catalog YAMLs

**Files:**
- Modify: `../sikifanso-homelab-bootstrap/catalog/*.yaml` (19 files)

The catalog YAML files are read by ArgoCD's git generator using `sigs.k8s.io/yaml` unmarshal into `Entry`. Fields are placed after `enabled` to maintain the existing field order.

**Tier assignments per spec:**
- `0-operators`: cnpg-operator, prometheus-stack, external-secrets
- `1-data`: postgresql
- `2-services`: langfuse, temporal
- No tier: all others (13 apps)

**Dependency declarations per spec:**
- postgresql: `[cnpg-operator, prometheus-stack]`
- langfuse: `[cnpg-operator, postgresql]`
- temporal: `[cnpg-operator, postgresql]`
- All others: none

- [ ] **Step 1: Update tier-0 operator entries (3 files)**

**cnpg-operator.yaml** — append after `enabled: false`:
```yaml
tier: 0-operators
```

**prometheus-stack.yaml** — append after `enabled: false`:
```yaml
tier: 0-operators
```

**external-secrets.yaml** — append after `enabled: false`:
```yaml
tier: 0-operators
```

- [ ] **Step 2: Update tier-1 data entry (1 file)**

**postgresql.yaml** — append after `enabled: false`:
```yaml
tier: 1-data
dependsOn:
  - cnpg-operator
  - prometheus-stack
```

- [ ] **Step 3: Update tier-2 service entries (2 files)**

**langfuse.yaml** — append after `enabled: false`:
```yaml
tier: 2-services
dependsOn:
  - cnpg-operator
  - postgresql
```

**temporal.yaml** — append after `enabled: false`:
```yaml
tier: 2-services
dependsOn:
  - cnpg-operator
  - postgresql
```

- [ ] **Step 4: Verify remaining 13 files need no changes**

The following 13 entries have no `tier` and no `dependsOn` — they are unchanged: alloy, guardrails-ai, litellm-proxy, loki, nemo-guardrails, ollama, opa, presidio, qdrant, tempo, text-embeddings-inference, unstructured, valkey.

ArgoCD's RollingSync syncs unmatched apps (no `tier` label) after all defined steps complete — which is the desired behavior.

- [ ] **Step 5: Verify YAML is valid**

Run from the bootstrap repo:
```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap
for f in catalog/*.yaml; do python3 -c "import yaml; yaml.safe_load(open('$f'))" && echo "OK: $f"; done
```
Expected: All 19 files print OK

- [ ] **Step 6: Commit (in sikifanso-homelab-bootstrap repo)**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso-homelab-bootstrap
git add catalog/cnpg-operator.yaml catalog/prometheus-stack.yaml catalog/external-secrets.yaml \
        catalog/postgresql.yaml catalog/langfuse.yaml catalog/temporal.yaml
git commit -m "catalog: add tier and dependsOn fields for RollingSync ordering"
```

---

### Task 11: Final Verification

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso
make test
```
Expected: All tests pass

- [ ] **Step 2: Run linter**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso
make lint
```
Expected: No lint issues

- [ ] **Step 3: Verify depgraph tests specifically**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso
go test ./internal/catalog/ -run "TestResolveDeps|TestDependents" -race -v
```
Expected: All 9 dependency graph tests pass

- [ ] **Step 4: Build the binary**

```bash
cd /Users/alicanalbayrak/dev/sikifanso/sikifanso
make build
```
Expected: `sikifanso` binary builds successfully
