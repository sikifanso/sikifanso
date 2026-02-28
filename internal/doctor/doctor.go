package doctor

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Result holds the outcome of a single health check.
type Result struct {
	Name    string
	OK      bool
	Message string // human-readable status (e.g., "3/3 nodes ready", "v27.0.3")
	Cause   string // only populated on failure
	Fix     string // suggested fix command, only on failure
}

// Check is a single health check that can be executed.
// A check may return one or more results (e.g., per-app checks).
type Check interface {
	Run(ctx context.Context) []Result
}

// Run executes all provided checks sequentially and returns their results.
func Run(ctx context.Context, checks []Check) []Result {
	var results []Result
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
func ClusterChecks(cs *kubernetes.Clientset) []Check {
	return []Check{
		NodesCheck{Client: cs},
		CiliumCheck{Client: cs},
		HubbleCheck{Client: cs},
		ArgoCDCheck{Client: cs},
	}
}

// AppChecks returns checks for enabled catalog apps.
func AppChecks(dynClient dynamic.Interface, gitOpsPath string) []Check {
	return []Check{AppsCheck{DynClient: dynClient, GitOpsPath: gitOpsPath}}
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
