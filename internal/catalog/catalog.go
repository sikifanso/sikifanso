package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sigs.k8s.io/yaml"
)

// Entry represents a single application in the catalog.
type Entry struct {
	Name           string `json:"name"`
	Category       string `json:"category"`
	Description    string `json:"description"`
	RepoURL        string `json:"repoURL"`
	Chart          string `json:"chart"`
	TargetRevision string `json:"targetRevision"`
	Namespace      string `json:"namespace"`
	Enabled        bool   `json:"enabled"`
}

// CatalogDir returns the path to the catalog directory within gitOpsPath.
func CatalogDir(gitOpsPath string) string {
	return filepath.Join(gitOpsPath, "catalog")
}

// List reads all *.yaml files in the catalog directory and returns their entries.
// Subdirectories (e.g. catalog/values/) and non-YAML files are skipped.
// Entries are returned regardless of their enabled state.
func List(gitOpsPath string) ([]Entry, error) {
	dir := CatalogDir(gitOpsPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading catalog directory: %w", err)
	}

	var apps []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading catalog file %s: %w", e.Name(), err)
		}

		var entry Entry
		if err := yaml.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("parsing catalog file %s: %w", e.Name(), err)
		}
		apps = append(apps, entry)
	}

	sort.Slice(apps, func(i, j int) bool { return apps[i].Name < apps[j].Name })

	return apps, nil
}

// Find returns the catalog entry with the given name.
// If no entry is found, it returns an error listing all available names.
func Find(gitOpsPath, name string) (*Entry, error) {
	entries, err := List(gitOpsPath)
	if err != nil {
		return nil, err
	}

	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return nil, fmt.Errorf("app %q not found in catalog; available: %s", name, strings.Join(names, ", "))
}

// SetEnabled flips the enabled field of the named catalog entry and writes the
// updated file back to disk. It does not commit; the caller is responsible for
// committing the change.
func SetEnabled(gitOpsPath, name string, enabled bool) error {
	fileName := name + ".yaml"
	filePath := filepath.Join(CatalogDir(gitOpsPath), fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("app %q not found in catalog", name)
		}
		return fmt.Errorf("reading catalog file %s: %w", fileName, err)
	}

	var entry Entry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return fmt.Errorf("parsing catalog file %s: %w", fileName, err)
	}

	entry.Enabled = enabled

	out, err := yaml.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling catalog entry %s: %w", name, err)
	}

	if err := os.WriteFile(filePath, out, 0644); err != nil {
		return fmt.Errorf("writing catalog file %s: %w", fileName, err)
	}

	return nil
}
