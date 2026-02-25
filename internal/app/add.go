package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"sigs.k8s.io/yaml"
)

// AddOpts holds all parameters for adding an app (no terminal I/O).
type AddOpts struct {
	GitOpsPath string
	Name       string
	RepoURL    string
	Chart      string
	Version    string
	Namespace  string
}

// coordinates is the YAML structure written to apps/coordinates/<name>.yaml.
type coordinates struct {
	Name           string `json:"name"`
	RepoURL        string `json:"repoURL"`
	Chart          string `json:"chart"`
	TargetRevision string `json:"targetRevision"`
	Namespace      string `json:"namespace"`
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Add writes the coordinate and values files, then commits to the gitops repo.
func Add(opts AddOpts) error {
	if opts.Name == "" {
		return fmt.Errorf("app name is required")
	}
	if !validName.MatchString(opts.Name) {
		return fmt.Errorf("invalid app name %q: must match [a-z0-9][a-z0-9-]*", opts.Name)
	}

	coordPath := filepath.Join("apps", "coordinates", opts.Name+".yaml")
	absCoord := filepath.Join(opts.GitOpsPath, coordPath)

	if _, err := os.Stat(absCoord); err == nil {
		return fmt.Errorf("app %q already exists", opts.Name)
	}

	coord := coordinates{
		Name:           opts.Name,
		RepoURL:        opts.RepoURL,
		Chart:          opts.Chart,
		TargetRevision: opts.Version,
		Namespace:      opts.Namespace,
	}

	coordData, err := yaml.Marshal(coord)
	if err != nil {
		return fmt.Errorf("marshaling coordinates: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absCoord), 0755); err != nil {
		return fmt.Errorf("creating coordinates directory: %w", err)
	}
	if err := os.WriteFile(absCoord, coordData, 0644); err != nil {
		return fmt.Errorf("writing coordinates file: %w", err)
	}

	valuesPath := filepath.Join("apps", "values", opts.Name+".yaml")
	absValues := filepath.Join(opts.GitOpsPath, valuesPath)

	if err := os.MkdirAll(filepath.Dir(absValues), 0755); err != nil {
		return fmt.Errorf("creating values directory: %w", err)
	}
	if err := os.WriteFile(absValues, []byte("# Helm values for "+opts.Name+"\n"), 0644); err != nil {
		return fmt.Errorf("writing values file: %w", err)
	}

	if err := gitops.Commit(opts.GitOpsPath, fmt.Sprintf("add app %s", opts.Name), coordPath, valuesPath); err != nil {
		return fmt.Errorf("committing app files: %w", err)
	}

	return nil
}
