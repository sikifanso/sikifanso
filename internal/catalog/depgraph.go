package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveDeps performs DFS transitive dependency resolution. Given a set of
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

	// DFS with cycle detection via "visiting" state.
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
			if err := visit(dep, append(append([]string{}, path...), name)); err != nil {
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
