package dashboard

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/catalog"
	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// AppStatus combines a catalog entry with its runtime health and sync status.
type AppStatus struct {
	Name       string `json:"name"`
	Category   string `json:"category"`
	Namespace  string `json:"namespace"`
	Enabled    bool   `json:"enabled"`
	Health     string `json:"health"`     // Healthy, Degraded, Missing, Unknown, etc.
	SyncStatus string `json:"syncStatus"` // Synced, OutOfSync, Unknown, etc.
}

// ClusterData aggregates cluster information for dashboard rendering.
type ClusterData struct {
	ClusterName string           `json:"clusterName"`
	Session     *session.Session `json:"-"`
	CatalogApps []AppStatus      `json:"catalogApps"`
	NodeCount   int              `json:"nodeCount"`
	NodesReady  int              `json:"nodesReady"`
	ArgoCDURL   string           `json:"argocdURL"`
	HubbleURL   string           `json:"hubbleURL"`
}

// Gather collects cluster data for the dashboard.
func Gather(ctx context.Context, clusterName string) (*ClusterData, error) {
	sess, err := session.Load(clusterName)
	if err != nil {
		return nil, fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	data := &ClusterData{
		ClusterName: clusterName,
		Session:     sess,
		ArgoCDURL:   sess.Services.ArgoCD.URL,
		HubbleURL:   sess.Services.Hubble.URL,
	}

	// Gather node status.
	cs, err := kube.ClientForCluster(clusterName)
	if err == nil {
		nodes, listErr := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if listErr == nil {
			data.NodeCount = len(nodes.Items)
			for _, n := range nodes.Items {
				if nodeReady(n) {
					data.NodesReady++
				}
			}
		}
	}

	// Gather catalog app statuses.
	entries, err := catalog.List(sess.GitOpsPath)
	if err != nil {
		return data, nil // return partial data
	}

	restCfg, err := kube.RESTConfigForCluster(clusterName)
	if err != nil {
		// Return catalog entries without health info.
		for _, e := range entries {
			data.CatalogApps = append(data.CatalogApps, AppStatus{
				Name:       e.Name,
				Category:   e.Category,
				Namespace:  e.Namespace,
				Enabled:    e.Enabled,
				Health:     "Unknown",
				SyncStatus: "Unknown",
			})
		}
		return data, nil
	}

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		for _, e := range entries {
			data.CatalogApps = append(data.CatalogApps, AppStatus{
				Name:       e.Name,
				Category:   e.Category,
				Namespace:  e.Namespace,
				Enabled:    e.Enabled,
				Health:     "Unknown",
				SyncStatus: "Unknown",
			})
		}
		return data, nil
	}

	cfg, cfgErr := infraconfig.Load(sess.GitOpsPath)
	argoNS := "argocd"
	if cfgErr == nil {
		argoNS = cfg.ArgoCD.Namespace
	}

	for _, e := range entries {
		as := AppStatus{
			Name:      e.Name,
			Category:  e.Category,
			Namespace: e.Namespace,
			Enabled:   e.Enabled,
		}

		if e.Enabled {
			health, sync := queryAppStatus(ctx, dynClient, e.Name, argoNS)
			as.Health = health
			as.SyncStatus = sync
		}

		data.CatalogApps = append(data.CatalogApps, as)
	}

	return data, nil
}

func queryAppStatus(ctx context.Context, dynClient dynamic.Interface, appName, namespace string) (health, syncStatus string) {
	app, err := dynClient.Resource(applicationGVR).Namespace(namespace).Get(ctx, appName, metav1.GetOptions{})
	if err != nil {
		return "Missing", "Unknown"
	}

	status, ok := app.Object["status"].(map[string]interface{})
	if !ok {
		return "Unknown", "Unknown"
	}

	health, _, _ = unstructured.NestedString(status, "health", "status")
	syncStatus, _, _ = unstructured.NestedString(status, "sync", "status")

	if health == "" {
		health = "Unknown"
	}
	if syncStatus == "" {
		syncStatus = "Unknown"
	}
	return health, syncStatus
}

func nodeReady(node corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
