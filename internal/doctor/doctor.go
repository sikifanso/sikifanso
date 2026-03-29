package doctor

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Result holds the outcome of a single health check.
type Result struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
	Cause   string `json:"cause,omitempty"`
	Fix     string `json:"fix,omitempty"`
}

// Check is a single health check that can be executed.
// A check may return one or more results (e.g., per-app checks).
type Check interface {
	Run(ctx context.Context) []Result
}

// Run executes all provided checks sequentially and returns their results.
func Run(ctx context.Context, checks []Check) []Result {
	results := make([]Result, 0, len(checks))
	for _, c := range checks {
		results = append(results, c.Run(ctx)...)
	}
	return results
}

// InfraChecks returns checks that don't need a Kubernetes client (just Docker).
func InfraChecks() []Check {
	return []Check{DockerCheck{}}
}

// ClusterChecks returns checks that need a typed Kubernetes clientset.
// Namespaces are read from the provided InfraConfig.
func ClusterChecks(cs *kubernetes.Clientset, cfg *infraconfig.InfraConfig) []Check {
	return []Check{
		NodesCheck{Client: cs},
		CiliumCheck{Client: cs, Namespace: cfg.Cilium.Namespace},
		HubbleCheck{Client: cs, Namespace: cfg.Cilium.Namespace},
		ArgoCDCheck{Client: cs, Namespace: cfg.ArgoCD.Namespace},
	}
}

// AppChecks returns checks for enabled catalog apps and agent namespaces.
// grpcClient is optional; when non-nil, unhealthy apps are enriched with
// per-resource details fetched from the ArgoCD gRPC API.
func AppChecks(dynClient dynamic.Interface, gitOpsPath string, cfg *infraconfig.InfraConfig, grpcClient *grpcclient.Client) []Check {
	return []Check{
		AppsCheck{DynClient: dynClient, GitOpsPath: gitOpsPath, ArgoCDNamespace: cfg.ArgoCD.Namespace, GRPCClient: grpcClient},
		AgentsCheck{DynClient: dynClient, GitOpsPath: gitOpsPath, ArgoCDNamespace: cfg.ArgoCD.Namespace},
	}
}

// deploymentAvailable returns true if the Deployment has the Available
// condition set to True.
func deploymentAvailable(deploy *appsv1.Deployment) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
