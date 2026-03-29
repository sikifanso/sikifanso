package doctor

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
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
// When GRPCClient is non-nil and an app is unhealthy, the resource tree is
// fetched to enrich the result with per-resource failure details.
type AppsCheck struct {
	DynClient       dynamic.Interface
	GitOpsPath      string
	ArgoCDNamespace string
	GRPCClient      *grpcclient.Client
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

	ns := c.ArgoCDNamespace
	if ns == "" {
		ns = "argocd"
	}
	app, err := c.DynClient.Resource(applicationGVR).Namespace(ns).Get(ctx, entry.Name, metav1.GetOptions{})
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

	// Enrich with resource tree details when a gRPC client is available.
	if c.GRPCClient != nil {
		if treeCause := c.resourceTreeCause(ctx, entry.Name); treeCause != "" {
			cause = treeCause
		}
	}

	return Result{
		Name:    name,
		OK:      false,
		Message: fmt.Sprintf("%s -- %s", healthStatus, syncStatus),
		Cause:   cause,
		Fix:     fmt.Sprintf("sikifanso catalog disable %s", entry.Name),
	}
}

// resourceTreeCause fetches the resource tree via gRPC and returns a summary
// of the first unhealthy resource, or an empty string on error.
func (c AppsCheck) resourceTreeCause(ctx context.Context, appName string) string {
	nodes, err := c.GRPCClient.ResourceTree(ctx, appName)
	if err != nil {
		return ""
	}
	for _, node := range nodes {
		if node.Health != "" && node.Health != "Healthy" {
			cause := fmt.Sprintf("%s/%s", node.Kind, node.Name)
			if node.Namespace != "" {
				cause += " in " + node.Namespace
			}
			cause += fmt.Sprintf(" (%s)", node.Health)
			if node.Message != "" {
				cause += ": " + node.Message
			}
			return cause
		}
	}
	return ""
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
