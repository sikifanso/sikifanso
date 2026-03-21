package doctor

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/agent"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

const checkNameAgents = "Agents"

// AgentsCheck verifies the health and sync status of each agent
// by querying the ArgoCD Application CRD via the dynamic client.
type AgentsCheck struct {
	DynClient       dynamic.Interface
	GitOpsPath      string
	ArgoCDNamespace string
}

func (c AgentsCheck) Run(ctx context.Context) []Result {
	agents, err := agent.List(c.GitOpsPath)
	if err != nil {
		return []Result{{
			Name:  checkNameAgents,
			OK:    false,
			Cause: fmt.Sprintf("listing agents: %v", err),
		}}
	}

	if len(agents) == 0 {
		return nil
	}

	var results []Result
	for _, a := range agents {
		results = append(results, c.checkAgent(ctx, a))
	}
	return results
}

func (c AgentsCheck) checkAgent(ctx context.Context, a agent.Info) Result {
	name := fmt.Sprintf("Agent: %s", a.Name)

	ns := c.ArgoCDNamespace
	if ns == "" {
		ns = "argocd"
	}

	app, err := c.DynClient.Resource(applicationGVR).Namespace(ns).Get(ctx, a.Name, metav1.GetOptions{})
	if err != nil {
		return Result{
			Name:  name,
			OK:    false,
			Cause: fmt.Sprintf("getting Application: %v", err),
			Fix:   fmt.Sprintf("sikifanso agent delete %s", a.Name),
		}
	}

	status, ok := app.Object["status"].(map[string]interface{})
	if !ok {
		return Result{
			Name:  name,
			OK:    false,
			Cause: "no status found on Application",
			Fix:   "sikifanso argocd sync",
		}
	}

	healthStatus, _, _ := unstructured.NestedString(status, "health", "status")
	syncStatus, _, _ := unstructured.NestedString(status, "sync", "status")

	if healthStatus == "Healthy" && syncStatus == "Synced" {
		return Result{
			Name:    name,
			OK:      true,
			Message: "Healthy, Synced",
		}
	}

	cause := extractDegradedCause(status)
	return Result{
		Name:    name,
		OK:      false,
		Message: fmt.Sprintf("%s -- %s", healthStatus, syncStatus),
		Cause:   cause,
		Fix:     fmt.Sprintf("sikifanso agent delete %s", a.Name),
	}
}
