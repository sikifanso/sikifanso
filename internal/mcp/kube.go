package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultLogTailLines = 50
	maxLogTailLines     = 1000
	defaultEventLimit   = 20
)

type kubePodsInput struct {
	Cluster   string `json:"cluster" jsonschema:"Name of the cluster"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace"`
}

type kubeServicesInput struct {
	Cluster   string `json:"cluster" jsonschema:"Name of the cluster"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace"`
}

type kubeLogsInput struct {
	Cluster   string `json:"cluster" jsonschema:"Name of the cluster"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace"`
	Pod       string `json:"pod" jsonschema:"Pod name"`
	Lines     int64  `json:"lines,omitempty" jsonschema:"Number of recent log lines to return, default 50"`
}

type kubeEventsInput struct {
	Cluster   string `json:"cluster" jsonschema:"Name of the cluster"`
	Namespace string `json:"namespace" jsonschema:"Kubernetes namespace"`
	Lines     int    `json:"lines,omitempty" jsonschema:"Number of recent events to return, default 20"`
}

func registerKubeTools(s *mcp.Server, _ *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_pods",
		Description: "List pods in a Kubernetes namespace with their status",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input kubePodsInput) (*mcp.CallToolResult, any, error) {
		cs, r, sv, e := kubeClient(input.Cluster)
		if cs == nil {
			return r, sv, e
		}

		pods, err := cs.CoreV1().Pods(input.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult(fmt.Errorf("listing pods: %w", err))
		}
		if len(pods.Items) == 0 {
			return textResult(fmt.Sprintf("No pods found in namespace %q.", input.Namespace))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Pods in %s:\n", input.Namespace)
		for _, pod := range pods.Items {
			ready := podReadyCount(&pod)
			fmt.Fprintf(&sb, "  - %-50s %s (%s)\n",
				pod.Name, string(pod.Status.Phase), ready)
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_services",
		Description: "List services in a Kubernetes namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input kubeServicesInput) (*mcp.CallToolResult, any, error) {
		cs, r, sv, e := kubeClient(input.Cluster)
		if cs == nil {
			return r, sv, e
		}

		svcs, err := cs.CoreV1().Services(input.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return errResult(fmt.Errorf("listing services: %w", err))
		}
		if len(svcs.Items) == 0 {
			return textResult(fmt.Sprintf("No services found in namespace %q.", input.Namespace))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Services in %s:\n", input.Namespace)
		for _, svc := range svcs.Items {
			ports := formatPorts(svc.Spec.Ports)
			fmt.Fprintf(&sb, "  - %-40s %s  %s\n",
				svc.Name, string(svc.Spec.Type), ports)
		}
		return textResult(sb.String())
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_logs",
		Description: "Get recent log lines from a pod",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input kubeLogsInput) (*mcp.CallToolResult, any, error) {
		cs, r, sv, e := kubeClient(input.Cluster)
		if cs == nil {
			return r, sv, e
		}

		lines := input.Lines
		if lines <= 0 {
			lines = defaultLogTailLines
		}
		if lines > maxLogTailLines {
			lines = maxLogTailLines
		}

		req := cs.CoreV1().Pods(input.Namespace).GetLogs(input.Pod, &corev1.PodLogOptions{
			TailLines: &lines,
		})
		body, err := req.DoRaw(ctx)
		if err != nil {
			return errResult(fmt.Errorf("getting logs for pod %q: %w", input.Pod, err))
		}
		if len(body) == 0 {
			return textResult(fmt.Sprintf("No logs found for pod %q.", input.Pod))
		}
		return textResult(string(body))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kube_events",
		Description: "Get recent events in a Kubernetes namespace",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input kubeEventsInput) (*mcp.CallToolResult, any, error) {
		cs, r, sv, e := kubeClient(input.Cluster)
		if cs == nil {
			return r, sv, e
		}

		limit := input.Lines
		if limit <= 0 {
			limit = defaultEventLimit
		}

		events, err := cs.CoreV1().Events(input.Namespace).List(ctx, metav1.ListOptions{
			Limit: int64(limit),
		})
		if err != nil {
			return errResult(fmt.Errorf("listing events: %w", err))
		}
		if len(events.Items) == 0 {
			return textResult(fmt.Sprintf("No events found in namespace %q.", input.Namespace))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Events in %s (last %d):\n", input.Namespace, len(events.Items))
		for _, ev := range events.Items {
			fmt.Fprintf(&sb, "  %s  %-8s  %-20s  %s\n",
				ev.LastTimestamp.Format("15:04:05"),
				ev.Type,
				ev.InvolvedObject.Name,
				ev.Message)
		}
		return textResult(sb.String())
	})
}

func podReadyCount(pod *corev1.Pod) string {
	total := len(pod.Spec.Containers)
	ready := 0
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d ready", ready, total)
}

func formatPorts(ports []corev1.ServicePort) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
	}
	return strings.Join(parts, ", ")
}
