package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

// AppInfo describes an installed app (read from coordinate file).
type AppInfo struct {
	Name      string `json:"name"`
	RepoURL   string `json:"repoURL"`
	Chart     string `json:"chart"`
	Version   string `json:"targetRevision"`
	Namespace string `json:"namespace"`
}

// List returns all apps found in the coordinates directory.
func List(gitOpsPath string) ([]AppInfo, error) {
	coordDir := filepath.Join(gitOpsPath, "apps", "coordinates")

	entries, err := os.ReadDir(coordDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading coordinates directory: %w", err)
	}

	var apps []AppInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(coordDir, e.Name()))
		if err != nil {
			continue
		}

		var info AppInfo
		if err := yaml.Unmarshal(data, &info); err != nil {
			continue
		}
		apps = append(apps, info)
	}

	return apps, nil
}
