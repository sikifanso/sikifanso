package cilium

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/helm"
	"github.com/briandowns/spinner"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	namespace    = "kube-system"
	nodeTimeout  = 2 * time.Minute
	pollInterval = 5 * time.Second
)

var helmParams = helm.InstallParams{
	Namespace:     namespace,
	RepoURL:       "https://helm.cilium.io/",
	ChartName:     "cilium",
	ReleaseName:   "cilium",
	Timeout:       5 * time.Minute,
	SpinnerSuffix: " Deploying Cilium to kube-system (this may take a few minutes)...",
}

// InstallResult holds metadata from a successful Cilium install.
type InstallResult struct {
	ChartVersion string
	APIServerIP  string
}

// Install deploys Cilium into the k3d cluster using the Helm Go SDK.
// On failure the cluster is intentionally left running so the user can
// debug with kubectl.
func Install(ctx context.Context, log *zap.Logger, clusterName string) (*InstallResult, error) {
	containerName := fmt.Sprintf("k3d-%s-server-0", clusterName)
	log.Info("detecting API server IP", zap.String("container", containerName))
	apiServerIP, err := detectAPIServerIP(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("detecting API server IP: %w", err)
	}
	log.Info("detected API server IP", zap.String("ip", apiServerIP))

	vals := Values(apiServerIP)

	cfg, settings, err := helm.Setup(log, namespace)
	if err != nil {
		return nil, fmt.Errorf("helm setup: %w", err)
	}

	log.Info("downloading cilium chart", zap.String("repo", helmParams.RepoURL))
	ch, err := helm.LocateChart(cfg, settings, helmParams)
	if err != nil {
		return nil, fmt.Errorf("locating cilium chart: %w", err)
	}
	log.Info("chart downloaded", zap.String("version", ch.Metadata.Version))

	if err := helm.Deploy(ctx, cfg, ch, vals, helmParams); err != nil {
		return nil, fmt.Errorf("helm install cilium: %w", err)
	}
	log.Info("cilium deployed successfully")

	if err := waitForNodes(ctx); err != nil {
		return nil, fmt.Errorf("waiting for nodes: %w", err)
	}
	log.Info("all nodes are ready")
	return &InstallResult{ChartVersion: ch.Metadata.Version, APIServerIP: apiServerIP}, nil
}

// detectAPIServerIP inspects the k3d server-0 Docker container and returns
// its network IP so Cilium agents can reach the API server after kube-proxy
// replacement.
func detectAPIServerIP(ctx context.Context, clusterName string) (string, error) {
	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return "", fmt.Errorf("creating docker client: %w", err)
	}
	defer docker.Close()

	containerName := fmt.Sprintf("k3d-%s-server-0", clusterName)
	info, err := docker.ContainerInspect(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("inspecting container %s: %w", containerName, err)
	}

	for _, net := range info.NetworkSettings.Networks {
		if net.IPAddress != "" {
			return net.IPAddress, nil
		}
	}
	return "", fmt.Errorf("no IP address found for container %s", containerName)
}

// waitForNodes polls all Kubernetes nodes until every node reports
// condition Ready=True, or the timeout expires. A spinner shows progress.
func waitForNodes(ctx context.Context) error {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Waiting for nodes to be ready..."
	s.Start()
	defer s.Stop()

	ctx, cancel := context.WithTimeout(ctx, nodeTimeout)
	defer cancel()

	for {
		nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("listing nodes: %w", err)
		}

		total := len(nodes.Items)
		ready := 0
		for _, n := range nodes.Items {
			if nodeReady(n) {
				ready++
			}
		}

		s.Suffix = fmt.Sprintf(" Waiting for nodes to be ready (%d/%d ready)...", ready, total)

		if ready == total && total > 0 {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for nodes to become ready (%d/%d ready)", ready, total)
		case <-time.After(pollInterval):
		}
	}
}

func nodeReady(node corev1.Node) bool {
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
