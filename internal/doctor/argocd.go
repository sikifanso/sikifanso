package doctor

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	checkNameArgoCD = "ArgoCD"
	fixArgoCD       = "kubectl -n argocd get deployments"
)

var argoCDDeployments = []string{
	"argocd-server",
	"argocd-repo-server",
	"argocd-applicationset-controller",
}

// ArgoCDCheck verifies that the core ArgoCD deployments are Available.
type ArgoCDCheck struct {
	Client *kubernetes.Clientset
}

func (c ArgoCDCheck) Run(ctx context.Context) []Result {
	total := len(argoCDDeployments)
	available := 0

	var firstFailure string
	for _, name := range argoCDDeployments {
		deploy, err := c.Client.AppsV1().Deployments("argocd").Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if firstFailure == "" {
				firstFailure = fmt.Sprintf("getting %s: %v", name, err)
			}
			continue
		}

		if deploymentAvailable(deploy) {
			available++
		} else if firstFailure == "" {
			firstFailure = fmt.Sprintf("%s not Available", name)
		}
	}

	if available < total {
		return []Result{{
			Name:  checkNameArgoCD,
			OK:    false,
			Cause: firstFailure,
			Fix:   fixArgoCD,
		}}
	}

	return []Result{{
		Name:    checkNameArgoCD,
		OK:      true,
		Message: fmt.Sprintf("%d/%d deployments ready", available, total),
	}}
}
