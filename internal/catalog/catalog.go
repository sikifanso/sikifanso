package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	yamlv3 "gopkg.in/yaml.v3"
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
// It reads only the target file in the happy path; on miss, it lists all
// available names in the error message.
func Find(gitOpsPath, name string) (*Entry, error) {
	filePath := filepath.Join(CatalogDir(gitOpsPath), name+".yaml")
	data, err := os.ReadFile(filePath)
	if err == nil {
		var entry Entry
		if err := yaml.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("parsing catalog file %s.yaml: %w", name, err)
		}
		return &entry, nil
	}

	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading catalog file %s.yaml: %w", name, err)
	}

	// File not found — list available names for a helpful error.
	entries, listErr := List(gitOpsPath)
	if listErr != nil {
		return nil, listErr
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return nil, fmt.Errorf("app %q not found in catalog; available: %s", name, strings.Join(names, ", "))
}

// SetEnabled flips the enabled field of the named catalog entry and writes the
// updated file back to disk, preserving comments and field order.
// It does not commit; the caller is responsible for committing the change.
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

	var doc yamlv3.Node
	if err := yamlv3.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing catalog file %s: %w", fileName, err)
	}

	if err := setEnabledInNode(&doc, enabled); err != nil {
		return fmt.Errorf("updating enabled field in %s: %w", fileName, err)
	}

	var buf bytes.Buffer
	enc := yamlv3.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return fmt.Errorf("encoding catalog entry %s: %w", name, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("closing encoder for %s: %w", name, err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing catalog file %s: %w", fileName, err)
	}

	return nil
}

// setEnabledInNode walks the yaml.Node AST to find the "enabled" key and
// updates its value in place, preserving comments, field order, and whitespace.
func setEnabledInNode(doc *yamlv3.Node, enabled bool) error {
	if doc.Kind != yamlv3.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("unexpected YAML structure")
	}

	mapping := doc.Content[0]
	if mapping.Kind != yamlv3.MappingNode {
		return fmt.Errorf("expected mapping node, got %d", mapping.Kind)
	}

	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == "enabled" {
			if enabled {
				mapping.Content[i+1].Value = "true"
			} else {
				mapping.Content[i+1].Value = "false"
			}
			mapping.Content[i+1].Tag = "!!bool"
			return nil
		}
	}

	return fmt.Errorf("enabled field not found")
}
