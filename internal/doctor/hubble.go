package doctor

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	checkNameHubble = "Hubble"
	fixHubble       = "sikifanso argocd sync"
)

// HubbleCheck verifies that the hubble-relay Deployment in kube-system is Available.
type HubbleCheck struct {
	Client *kubernetes.Clientset
}

func (c HubbleCheck) Run(ctx context.Context) []Result {
	deploy, err := c.Client.AppsV1().Deployments("kube-system").Get(ctx, "hubble-relay", metav1.GetOptions{})
	if err != nil {
		return []Result{{
			Name:  checkNameHubble,
			OK:    false,
			Cause: fmt.Sprintf("getting hubble-relay Deployment: %v", err),
			Fix:   fixHubble,
		}}
	}

	if deploymentAvailable(deploy) {
		return []Result{{
			Name:    checkNameHubble,
			OK:      true,
			Message: "relay deployment ready",
		}}
	}

	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}

	return []Result{{
		Name:  checkNameHubble,
		OK:    false,
		Cause: fmt.Sprintf("hubble-relay not Available (ready %d/%d)", deploy.Status.ReadyReplicas, desired),
		Fix:   fixHubble,
	}}
}
