package doctor

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const checkNameCilium = "Cilium"

// CiliumCheck verifies that the Cilium DaemonSet is fully available.
type CiliumCheck struct {
	Client    *kubernetes.Clientset
	Namespace string
}

func (c CiliumCheck) Run(ctx context.Context) []Result {
	ns := c.Namespace
	if ns == "" {
		ns = "kube-system"
	}

	fix := fmt.Sprintf("kubectl -n %s logs -l app.kubernetes.io/name=cilium-agent", ns)

	ds, err := c.Client.AppsV1().DaemonSets(ns).Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return []Result{{
			Name:  checkNameCilium,
			OK:    false,
			Cause: fmt.Sprintf("getting cilium DaemonSet: %v", err),
			Fix:   fix,
		}}
	}

	desired := ds.Status.DesiredNumberScheduled
	ready := ds.Status.NumberReady

	if ready < desired {
		return []Result{{
			Name:  checkNameCilium,
			OK:    false,
			Cause: fmt.Sprintf("DaemonSet %d/%d ready", ready, desired),
			Fix:   fix,
		}}
	}

	return []Result{{
		Name:    checkNameCilium,
		OK:      true,
		Message: fmt.Sprintf("DaemonSet %d/%d ready", ready, desired),
	}}
}
