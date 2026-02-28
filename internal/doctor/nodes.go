package doctor

import (
	"context"
	"fmt"

	"github.com/alicanalbayrak/sikifanso/internal/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	checkNameNodes = "k3d cluster"
	fixNodes       = "sikifanso cluster start"
)

// NodesCheck verifies that all k3d nodes are in the Ready state.
type NodesCheck struct {
	Client *kubernetes.Clientset
}

func (c NodesCheck) Run(ctx context.Context) []Result {
	nodes, err := c.Client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return []Result{{
			Name:  checkNameNodes,
			OK:    false,
			Cause: fmt.Sprintf("listing nodes: %v", err),
			Fix:   fixNodes,
		}}
	}

	total := len(nodes.Items)
	if total == 0 {
		return []Result{{
			Name:  checkNameNodes,
			OK:    false,
			Cause: "no nodes found",
			Fix:   fixNodes,
		}}
	}

	ready := 0
	for _, n := range nodes.Items {
		if kube.NodeReady(n) {
			ready++
		}
	}

	if ready < total {
		return []Result{{
			Name:  checkNameNodes,
			OK:    false,
			Cause: fmt.Sprintf("%d/%d nodes ready", ready, total),
			Fix:   fixNodes,
		}}
	}

	return []Result{{
		Name:    checkNameNodes,
		OK:      true,
		Message: fmt.Sprintf("%d/%d nodes ready", ready, total),
	}}
}
