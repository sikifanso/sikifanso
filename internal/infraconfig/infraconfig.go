package infraconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"
)

// PlatformConfig holds K3s image and cluster topology settings.
type PlatformConfig struct {
	K3sImage  string         `yaml:"k3sImage"`
	Servers   int            `yaml:"servers"`
	Agents    int            `yaml:"agents"`
	NodePorts NodePortConfig `yaml:"nodePorts"`
}

// NodePortConfig holds container-side NodePort assignments.
type NodePortConfig struct {
	HTTP     int `yaml:"http"`
	HTTPS    int `yaml:"https"`
	ArgoCDUI int `yaml:"argocdUI"`
	HubbleUI int `yaml:"hubbleUI"`
}

// ChartConfig holds Helm chart coordinates for a component.
type ChartConfig struct {
	RepoURL        string `yaml:"repoURL"`
	Chart          string `yaml:"chart"`
	TargetRevision string `yaml:"targetRevision"`
	Namespace      string `yaml:"namespace"`
	ReleaseName    string `yaml:"releaseName"`
}

// InfraConfig aggregates all infrastructure configuration.
type InfraConfig struct {
	Platform     PlatformConfig
	Cilium       ChartConfig
	CiliumValues map[string]interface{}
	ArgoCD       ChartConfig
	ArgoCDValues map[string]interface{}
}

// Load reads infra configuration from the given gitops directory.
// It looks for an infra/ subdirectory and loads each file independently,
// falling back to compiled-in defaults for any missing file.
func Load(gitopsDir string) (*InfraConfig, error) {
	cfg := Defaults()
	infraDir := filepath.Join(gitopsDir, "infra")

	if err := loadYAML(filepath.Join(infraDir, "platform.yaml"), &cfg.Platform); err != nil {
		return nil, fmt.Errorf("loading platform config: %w", err)
	}

	if err := loadYAML(filepath.Join(infraDir, "cilium.yaml"), &cfg.Cilium); err != nil {
		return nil, fmt.Errorf("loading cilium chart config: %w", err)
	}

	if err := loadYAML(filepath.Join(infraDir, "argocd.yaml"), &cfg.ArgoCD); err != nil {
		return nil, fmt.Errorf("loading argocd chart config: %w", err)
	}

	if err := mergeValuesYAML(filepath.Join(infraDir, "cilium-values.yaml"), &cfg.CiliumValues); err != nil {
		return nil, fmt.Errorf("loading cilium values: %w", err)
	}

	if err := mergeValuesYAML(filepath.Join(infraDir, "argocd-values.yaml"), &cfg.ArgoCDValues); err != nil {
		return nil, fmt.Errorf("loading argocd values: %w", err)
	}

	return cfg, nil
}

// CiliumRuntimeOverrides returns runtime-computed Helm value overrides
// for Cilium that depend on the running cluster (API server IP, node ports).
func CiliumRuntimeOverrides(np NodePortConfig, apiServerIP string) map[string]interface{} {
	return map[string]interface{}{
		"k8sServiceHost": apiServerIP,
		"ingressController": map[string]interface{}{
			"service": map[string]interface{}{
				"insecureNodePort": np.HTTP,
				"secureNodePort":   np.HTTPS,
			},
		},
		"hubble": map[string]interface{}{
			"ui": map[string]interface{}{
				"service": map[string]interface{}{
					"nodePort": np.HubbleUI,
				},
			},
		},
	}
}

// ArgoCDRuntimeOverrides returns runtime-computed Helm value overrides
// for ArgoCD that depend on node port assignments.
func ArgoCDRuntimeOverrides(np NodePortConfig) map[string]interface{} {
	return map[string]interface{}{
		"server": map[string]interface{}{
			"service": map[string]interface{}{
				"nodePortHttp": np.ArgoCDUI,
			},
		},
	}
}

// MergeValues performs a recursive deep-merge of overrides into base.
// Override values take precedence. Nested maps are merged recursively;
// all other types are replaced.
func MergeValues(base, overrides map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range overrides {
		if baseMap, ok := result[k].(map[string]interface{}); ok {
			if overrideMap, ok := v.(map[string]interface{}); ok {
				result[k] = MergeValues(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}

// loadYAML reads a YAML file into dst. If the file does not exist,
// dst is left unchanged (preserving defaults).
func loadYAML(path string, dst interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, dst)
}

// mergeValuesYAML reads a YAML file and deep-merges it into dst.
// If the file does not exist, dst is left unchanged (preserving defaults).
// This ensures users only need to specify the values they want to override.
func mergeValuesYAML(path string, dst *map[string]interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return err
	}
	*dst = MergeValues(*dst, m)
	return nil
}
