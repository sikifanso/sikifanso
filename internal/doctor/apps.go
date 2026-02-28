package doctor

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const checkNameApps = "Apps"

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// AppsCheck verifies the health and sync status of each enabled catalog app
// by querying the ArgoCD Application CRD via the dynamic client.
type AppsCheck struct {
	DynClient  dynamic.Interface
	GitOpsPath string
}

func (c AppsCheck) Run(ctx context.Context) []Result {
	entries, err := catalog.List(c.GitOpsPath)
	if err != nil {
		return []Result{{
			Name:  checkNameApps,
			OK:    false,
			Cause: fmt.Sprintf("listing catalog: %v", err),
		}}
	}

	var results []Result
	for _, entry := range entries {
		if !entry.Enabled {
			continue
		}
		results = append(results, c.checkApp(ctx, entry))
	}
	return results
}

func (c AppsCheck) checkApp(ctx context.Context, entry catalog.Entry) Result {
	name := fmt.Sprintf("App: %s", entry.Name)

	app, err := c.DynClient.Resource(applicationGVR).Namespace("argocd").Get(ctx, entry.Name, metav1.GetOptions{})
	if err != nil {
		return Result{
			Name:  name,
			OK:    false,
			Cause: fmt.Sprintf("getting Application: %v", err),
			Fix:   fmt.Sprintf("sikifanso catalog disable %s", entry.Name),
		}
	}

	status, ok := app.Object["status"].(map[string]interface{})
	if !ok {
		return Result{
			Name:    name,
			OK:      false,
			Cause:   "no status found on Application",
			Fix:     "sikifanso argocd sync",
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
		Fix:     fmt.Sprintf("sikifanso catalog disable %s", entry.Name),
	}
}

// extractDegradedCause scans status.resources[] for the first entry with a
// non-healthy status and extracts its health.message.
func extractDegradedCause(status map[string]interface{}) string {
	resources, ok := status["resources"].([]interface{})
	if !ok {
		return ""
	}

	for _, r := range resources {
		res, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		health, ok := res["health"].(map[string]interface{})
		if !ok {
			continue
		}
		hs, _ := health["status"].(string)
		if hs != "Healthy" && hs != "" {
			msg, _ := health["message"].(string)
			kind, _ := res["kind"].(string)
			resName, _ := res["name"].(string)
			ns, _ := res["namespace"].(string)

			cause := fmt.Sprintf("%s %s in namespace %s", kind, resName, ns)
			if msg != "" {
				cause += ": " + msg
			}
			return cause
		}
	}
	return ""
}
