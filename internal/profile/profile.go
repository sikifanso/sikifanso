package profile

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
)

// Profile defines a named set of catalog apps to enable together.
type Profile struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Apps        []string `json:"apps"`
}

// registry holds all known profiles keyed by name.
var registry = map[string]Profile{
	"agent-minimal": {
		Name:        "agent-minimal",
		Description: "Bare minimum to route and observe LLM calls",
		// valkey is required: langfuse uses it as a Redis-compatible session cache
		Apps: []string{"litellm-proxy", "langfuse", "cnpg-operator", "postgresql", "valkey"},
	},
	"agent-full": {
		Name:        "agent-full",
		Description: "All AI agent infrastructure tools enabled",
		Apps: []string{
			"litellm-proxy", "langfuse", "prometheus-stack", "loki", "tempo", "alloy",
			"guardrails-ai", "nemo-guardrails", "presidio",
			"qdrant", "text-embeddings-inference", "unstructured",
			"temporal", "external-secrets", "opa",
			"ollama", "cnpg-operator", "postgresql", "valkey",
		},
	},
	"agent-dev": {
		Name:        "agent-dev",
		Description: "Local development loop with LLM, RAG, and observability",
		Apps:        []string{"litellm-proxy", "ollama", "langfuse", "qdrant", "cnpg-operator", "postgresql", "valkey", "alloy"},
	},
	"agent-safe": {
		Name:        "agent-safe",
		Description: "Development stack with all guardrails and policy enforcement",
		Apps: []string{
			"litellm-proxy", "ollama", "langfuse", "qdrant", "cnpg-operator", "postgresql", "valkey",
			"guardrails-ai", "nemo-guardrails", "presidio", "opa",
		},
	},
	"rag": {
		Name:        "rag",
		Description: "RAG-focused stack with vector DB, embeddings, and document parsing",
		Apps:        []string{"qdrant", "text-embeddings-inference", "unstructured", "cnpg-operator", "postgresql"},
	},
}

// List returns all available profiles sorted by name.
func List() []Profile {
	profiles := make([]Profile, 0, len(registry))
	for _, p := range registry {
		profiles = append(profiles, p)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Name < profiles[j].Name
	})
	return profiles
}

// Get returns the profile with the given name or an error listing available profiles.
func Get(name string) (Profile, error) {
	p, ok := registry[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile %q not found; available: %s", name, strings.Join(Names(), ", "))
	}
	return p, nil
}

// Resolve takes a comma-separated profile string (e.g. "agent-dev,rag") and
// returns the deduplicated union of all apps across the named profiles.
func Resolve(profileStr string) ([]string, error) {
	parts := strings.Split(profileStr, ",")
	seen := make(map[string]bool)
	var apps []string

	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		p, err := Get(name)
		if err != nil {
			return nil, err
		}
		for _, app := range p.Apps {
			if !seen[app] {
				seen[app] = true
				apps = append(apps, app)
			}
		}
	}
	return apps, nil
}

// Apply enables the given apps in the catalog at gitOpsPath and commits the
// changes in a single commit. Apps that don't exist in the catalog are skipped
// with a warning via the provided warn function. The profileName is used in the
// commit message for traceability.
//
// Each SetEnabled call writes to disk independently. On partial failure,
// successfully written files are committed; skipped apps are warned.
// If Commit itself fails, the gitops worktree may be left dirty.
func Apply(gitOpsPath string, profileName string, apps []string, warn func(string)) error {
	var committed []string
	for _, app := range apps {
		if err := catalog.SetEnabled(gitOpsPath, app, true); err != nil {
			if warn != nil {
				warn(fmt.Sprintf("skipping %s: %s", app, err))
			}
			continue
		}
		committed = append(committed, fmt.Sprintf("catalog/%s.yaml", app))
	}

	if len(committed) == 0 {
		return nil
	}

	msg := fmt.Sprintf("profile: enable %s apps", profileName)
	return gitops.Commit(gitOpsPath, msg, committed...)
}

// Names returns the sorted list of available profile names (for shell completion).
func Names() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
