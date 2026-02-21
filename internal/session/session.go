package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/yaml"
)

// Session holds persisted metadata for a sikifanso cluster.
type Session struct {
	ClusterName  string        `json:"clusterName"`
	CreatedAt    time.Time     `json:"createdAt"`
	BootstrapURL string        `json:"bootstrapURL"`
	GitOpsPath   string        `json:"gitOpsPath"`
	Services     ServiceInfo   `json:"services"`
	K3dConfig    K3dConfigInfo `json:"k3dConfig"`
}

// ServiceInfo groups all service access details.
type ServiceInfo struct {
	ArgoCD ArgoCDInfo `json:"argocd"`
	Hubble HubbleInfo `json:"hubble"`
}

// ArgoCDInfo holds ArgoCD access details.
type ArgoCDInfo struct {
	URL          string `json:"url"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ChartVersion string `json:"chartVersion"`
}

// HubbleInfo holds Hubble UI access details.
type HubbleInfo struct {
	URL string `json:"url"`
}

// K3dConfigInfo records the k3d cluster configuration used.
type K3dConfigInfo struct {
	Image   string `json:"image"`
	Servers int    `json:"servers"`
	Agents  int    `json:"agents"`
}

const (
	baseDir     = ".sikifanso"
	clusterDir  = "clusters"
	sessionFile = "session.yaml"
	gitopsDir   = "gitops"
)

// Dir returns the session directory for the given cluster name:
// ~/.sikifanso/clusters/<name>/
func Dir(clusterName string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, baseDir, clusterDir, clusterName), nil
}

// GitOpsDir returns the gitops directory for the given cluster name:
// ~/.sikifanso/clusters/<name>/gitops/
func GitOpsDir(clusterName string) (string, error) {
	dir, err := Dir(clusterName)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, gitopsDir), nil
}

// Save marshals the session to YAML and writes it to the session directory.
func Save(s *Session) error {
	dir, err := Dir(s.ClusterName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	path := filepath.Join(dir, sessionFile)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}
	return nil
}

// Load reads and unmarshals the session for the given cluster name.
func Load(clusterName string) (*Session, error) {
	dir, err := Dir(clusterName)
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, sessionFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var s Session
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshaling session: %w", err)
	}
	return &s, nil
}

// Remove deletes the entire session directory for the given cluster name.
func Remove(clusterName string) error {
	dir, err := Dir(clusterName)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
