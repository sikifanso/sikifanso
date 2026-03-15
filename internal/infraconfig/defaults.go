package infraconfig

import (
	"embed"
	"sync"

	"sigs.k8s.io/yaml"
)

//go:embed defaults/*.yaml
var defaultFS embed.FS

var (
	defaultOnce sync.Once
	defaultCfg  *InfraConfig
)

// Defaults returns compiled-in default infrastructure configuration.
// The embedded YAML is parsed once; each call returns a deep copy so
// callers (e.g. Load) can mutate freely.
func Defaults() *InfraConfig {
	defaultOnce.Do(func() {
		defaultCfg = &InfraConfig{}
		mustUnmarshal("defaults/platform.yaml", &defaultCfg.Platform)
		mustUnmarshal("defaults/cilium.yaml", &defaultCfg.Cilium)
		mustUnmarshal("defaults/argocd.yaml", &defaultCfg.ArgoCD)
		defaultCfg.CiliumValues = mustUnmarshalMap("defaults/cilium-values.yaml")
		defaultCfg.ArgoCDValues = mustUnmarshalMap("defaults/argocd-values.yaml")
	})

	cp := *defaultCfg
	cp.CiliumValues = copyMap(defaultCfg.CiliumValues)
	cp.ArgoCDValues = copyMap(defaultCfg.ArgoCDValues)
	return &cp
}

func mustUnmarshal(name string, dst interface{}) {
	data, err := defaultFS.ReadFile(name)
	if err != nil {
		panic("infraconfig: missing embedded default: " + name + ": " + err.Error())
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		panic("infraconfig: invalid embedded default: " + name + ": " + err.Error())
	}
}

func mustUnmarshalMap(name string) map[string]interface{} {
	data, err := defaultFS.ReadFile(name)
	if err != nil {
		panic("infraconfig: missing embedded default: " + name + ": " + err.Error())
	}
	var m map[string]interface{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		panic("infraconfig: invalid embedded default: " + name + ": " + err.Error())
	}
	return m
}

// copyMap returns a shallow copy of a map. Nested maps are copied recursively.
func copyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{}, len(m))
	for k, v := range m {
		if nested, ok := v.(map[string]interface{}); ok {
			cp[k] = copyMap(nested)
		} else {
			cp[k] = v
		}
	}
	return cp
}
