package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/yaml"
)

const (
	// DefaultChartRepoURL is the Helm repo for the agent template chart.
	// During development this can be overridden via CreateOpts.ChartRepoURL.
	DefaultChartRepoURL = "https://sikifanso.github.io/sikifanso-agent-template"
	// DefaultChartVersion is the default chart version to deploy.
	DefaultChartVersion = "0.1.0"

	DefaultCPURequest    = "250m"
	DefaultCPULimit      = "1000m"
	DefaultMemoryRequest = "256Mi"
	DefaultMemoryLimit   = "1Gi"
	DefaultPods          = "10"
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
	Name          string `json:"name"`
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	Pods          string `json:"pods"`
}

// CreateOpts configures agent creation.
type CreateOpts struct {
	Name          string
	CPURequest    string
	CPULimit      string
	MemoryRequest string
	MemoryLimit   string
	Pods          string
	ChartRepoURL  string
	ChartVersion  string
}

// Info holds agent metadata for display.
type Info struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	CPURequest    string `json:"cpuRequest"`
	CPULimit      string `json:"cpuLimit"`
	MemoryRequest string `json:"memoryRequest"`
	MemoryLimit   string `json:"memoryLimit"`
	Pods          string `json:"pods"`
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// checkPair validates that a resource request does not exceed its limit.
func checkPair(req, lim, label string) error {
	r, err := resource.ParseQuantity(req)
	if err != nil {
		return fmt.Errorf("invalid %sRequest %q: %w", label, req, err)
	}
	l, err := resource.ParseQuantity(lim)
	if err != nil {
		return fmt.Errorf("invalid %sLimit %q: %w", label, lim, err)
	}
	if r.Cmp(l) > 0 {
		return fmt.Errorf("%sRequest (%s) exceeds %sLimit (%s)", label, req, label, lim)
	}
	return nil
}

// validateQuotas checks that resource requests do not exceed limits.
func validateQuotas(cpuReq, cpuLim, memReq, memLim string) error {
	if err := checkPair(cpuReq, cpuLim, "cpu"); err != nil {
		return err
	}
	return checkPair(memReq, memLim, "memory")
}

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
	cpuReq := opts.CPURequest
	if cpuReq == "" {
		cpuReq = DefaultCPURequest
	}
	cpuLim := opts.CPULimit
	if cpuLim == "" {
		cpuLim = DefaultCPULimit
	}
	memReq := opts.MemoryRequest
	if memReq == "" {
		memReq = DefaultMemoryRequest
	}
	memLim := opts.MemoryLimit
	if memLim == "" {
		memLim = DefaultMemoryLimit
	}
	pods := opts.Pods
	if pods == "" {
		pods = DefaultPods
	}

	if err := validateQuotas(cpuReq, cpuLim, memReq, memLim); err != nil {
		return err
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
			Name:          opts.Name,
			CPURequest:    cpuReq,
			CPULimit:      cpuLim,
			MemoryRequest: memReq,
			MemoryLimit:   memLim,
			Pods:          pods,
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

// populateQuota reads an agent values file and fills quota fields on info.
func populateQuota(info *Info, valuesFile string) {
	vData, err := os.ReadFile(valuesFile)
	if err != nil {
		return
	}
	var v values
	if err := yaml.Unmarshal(vData, &v); err != nil {
		return
	}
	info.CPURequest = v.Agent.CPURequest
	info.CPULimit = v.Agent.CPULimit
	info.MemoryRequest = v.Agent.MemoryRequest
	info.MemoryLimit = v.Agent.MemoryLimit
	info.Pods = v.Agent.Pods
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

		info := Info{
			Name:      ent.Name,
			Namespace: ent.Namespace,
		}
		valuesFile := filepath.Join(dir, "values", ent.Name+".yaml")
		populateQuota(&info, valuesFile)

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
	populateQuota(info, valuesFile)
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
