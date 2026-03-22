package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultChartRepoURL is the Helm repo for the agent template chart.
	// During development this can be overridden via CreateOpts.ChartRepoURL.
	DefaultChartRepoURL = "https://sikifanso.github.io/sikifanso-agent-template"
	// DefaultChartVersion is the default chart version to deploy.
	DefaultChartVersion = "0.1.0"
)

// entry is the YAML structure written to agents/<name>.yaml.
type entry struct {
	Name           string `json:"name"`
	RepoURL        string `json:"repoURL"`
	Chart          string `json:"chart"`
	TargetRevision string `json:"targetRevision"`
	Namespace      string `json:"namespace"`
}

// values is the YAML structure written to agents/values/<name>.yaml.
type values struct {
	Agent agentValues `json:"agent"`
}

type agentValues struct {
	Name   string `json:"name"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Pods   string `json:"pods"`
}

// CreateOpts configures agent creation.
type CreateOpts struct {
	Name         string
	CPU          string
	Memory       string
	Pods         string
	ChartRepoURL string
	ChartVersion string
}

// Info holds agent metadata for display.
type Info struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	CPU       string `json:"cpu"`
	Memory    string `json:"memory"`
	Pods      string `json:"pods"`
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// AgentsDir returns the path to the agents directory within gitOpsPath.
func AgentsDir(gitOpsPath string) string {
	return filepath.Join(gitOpsPath, "agents")
}

// Create writes agent entry and values files, then commits to the gitops repo.
func Create(gitOpsPath string, opts CreateOpts) error {
	if opts.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if !validName.MatchString(opts.Name) {
		return fmt.Errorf("invalid agent name %q: must match [a-z0-9][a-z0-9-]*", opts.Name)
	}

	entryPath := filepath.Join("agents", opts.Name+".yaml")
	absEntry := filepath.Join(gitOpsPath, entryPath)
	if _, err := os.Stat(absEntry); err == nil {
		return fmt.Errorf("agent %q already exists", opts.Name)
	}

	repoURL := opts.ChartRepoURL
	if repoURL == "" {
		repoURL = DefaultChartRepoURL
	}
	chartVersion := opts.ChartVersion
	if chartVersion == "" {
		chartVersion = DefaultChartVersion
	}
	cpu := opts.CPU
	if cpu == "" {
		cpu = "500m"
	}
	mem := opts.Memory
	if mem == "" {
		mem = "512Mi"
	}
	pods := opts.Pods
	if pods == "" {
		pods = "10"
	}

	e := entry{
		Name:           opts.Name,
		RepoURL:        repoURL,
		Chart:          "sikifanso-agent-template",
		TargetRevision: chartVersion,
		Namespace:      "agent-" + opts.Name,
	}

	v := values{
		Agent: agentValues{
			Name:   opts.Name,
			CPU:    cpu,
			Memory: mem,
			Pods:   pods,
		},
	}

	entryData, err := yaml.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling agent entry: %w", err)
	}
	if err := os.WriteFile(absEntry, entryData, 0o644); err != nil {
		return fmt.Errorf("writing agent entry: %w", err)
	}

	valuesPath := filepath.Join("agents", "values", opts.Name+".yaml")
	absValues := filepath.Join(gitOpsPath, valuesPath)
	valuesData, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshaling agent values: %w", err)
	}
	if err := os.WriteFile(absValues, valuesData, 0o644); err != nil {
		return fmt.Errorf("writing agent values: %w", err)
	}

	return gitops.Commit(gitOpsPath, fmt.Sprintf("agent: create %s", opts.Name), entryPath, valuesPath)
}

// List reads all agent entries from the agents directory.
func List(gitOpsPath string) ([]Info, error) {
	dir := AgentsDir(gitOpsPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading agents directory: %w", err)
	}

	var agents []Info
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading agent file %s: %w", e.Name(), err)
		}

		var ent entry
		if err := yaml.Unmarshal(data, &ent); err != nil {
			return nil, fmt.Errorf("parsing agent file %s: %w", e.Name(), err)
		}

		// Read values for quota info.
		info := Info{
			Name:      ent.Name,
			Namespace: ent.Namespace,
		}
		valuesFile := filepath.Join(dir, "values", ent.Name+".yaml")
		if vData, err := os.ReadFile(valuesFile); err == nil {
			var v values
			if err := yaml.Unmarshal(vData, &v); err == nil {
				info.CPU = v.Agent.CPU
				info.Memory = v.Agent.Memory
				info.Pods = v.Agent.Pods
			}
		}

		agents = append(agents, info)
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].Name < agents[j].Name
	})
	return agents, nil
}

// Find returns the agent with the given name or an error.
func Find(gitOpsPath, name string) (*Info, error) {
	entryFile := filepath.Join(AgentsDir(gitOpsPath), name+".yaml")
	data, err := os.ReadFile(entryFile)
	if err != nil {
		if os.IsNotExist(err) {
			agents, listErr := List(gitOpsPath)
			if listErr != nil {
				return nil, listErr
			}
			names := make([]string, len(agents))
			for i, a := range agents {
				names[i] = a.Name
			}
			if len(names) == 0 {
				return nil, fmt.Errorf("agent %q not found; no agents exist", name)
			}
			return nil, fmt.Errorf("agent %q not found; available: %s", name, strings.Join(names, ", "))
		}
		return nil, fmt.Errorf("reading agent file: %w", err)
	}

	var ent entry
	if err := yaml.Unmarshal(data, &ent); err != nil {
		return nil, fmt.Errorf("parsing agent file: %w", err)
	}

	info := &Info{Name: ent.Name, Namespace: ent.Namespace}
	valuesFile := filepath.Join(AgentsDir(gitOpsPath), "values", name+".yaml")
	if vData, err := os.ReadFile(valuesFile); err == nil {
		var v values
		if err := yaml.Unmarshal(vData, &v); err == nil {
			info.CPU = v.Agent.CPU
			info.Memory = v.Agent.Memory
			info.Pods = v.Agent.Pods
		}
	}
	return info, nil
}

// Delete removes agent entry and values files, then commits.
func Delete(gitOpsPath, name string) error {
	entryPath := filepath.Join("agents", name+".yaml")
	absEntry := filepath.Join(gitOpsPath, entryPath)
	if _, err := os.Stat(absEntry); os.IsNotExist(err) {
		return fmt.Errorf("agent %q not found", name)
	}

	if err := os.Remove(absEntry); err != nil {
		return fmt.Errorf("removing agent entry: %w", err)
	}

	valuesPath := filepath.Join("agents", "values", name+".yaml")
	absValues := filepath.Join(gitOpsPath, valuesPath)
	_ = os.Remove(absValues) // best-effort; values file may not exist

	return gitops.Commit(gitOpsPath, fmt.Sprintf("agent: delete %s", name), entryPath, valuesPath)
}
