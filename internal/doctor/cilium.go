package doctor

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	checkNameCilium = "Cilium"
	fixCilium       = "kubectl -n kube-system logs -l app.kubernetes.io/name=cilium-agent"
)

// CiliumCheck verifies that the Cilium DaemonSet in kube-system is fully available.
type CiliumCheck struct {
	Client *kubernetes.Clientset
}

func (c CiliumCheck) Run(ctx context.Context) []Result {
	ds, err := c.Client.AppsV1().DaemonSets("kube-system").Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return []Result{{
			Name:  checkNameCilium,
			OK:    false,
			Cause: fmt.Sprintf("getting cilium DaemonSet: %v", err),
			Fix:   fixCilium,
		}}
	}

	desired := ds.Status.DesiredNumberScheduled
	ready := ds.Status.NumberReady

	if ready < desired {
		return []Result{{
			Name:  checkNameCilium,
			OK:    false,
			Cause: fmt.Sprintf("DaemonSet %d/%d ready", ready, desired),
			Fix:   fixCilium,
		}}
	}

	return []Result{{
		Name:    checkNameCilium,
		OK:      true,
		Message: fmt.Sprintf("DaemonSet %d/%d ready", ready, desired),
	}}
}
