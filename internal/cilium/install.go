package cilium

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	namespace    = "kube-system"
	repoURL      = "https://helm.cilium.io/"
	releaseName  = "cilium"
	chartName    = "cilium"
	helmTimeout  = 5 * time.Minute
	nodeTimeout  = 2 * time.Minute
	pollInterval = 5 * time.Second
)

// Install deploys Cilium into the k3d cluster using the Helm Go SDK.
// On failure the cluster is intentionally left running so the user can
// debug with kubectl.
func Install(ctx context.Context, log *zap.Logger, clusterName string) error {
	containerName := fmt.Sprintf("k3d-%s-server-0", clusterName)
	log.Info("detecting API server IP", zap.String("container", containerName))
	apiServerIP, err := detectAPIServerIP(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("detecting API server IP: %w", err)
	}
	log.Info("detected API server IP", zap.String("ip", apiServerIP))

	vals := Values(apiServerIP)

	cfg, settings, err := helmSetup(log)
	if err != nil {
		return fmt.Errorf("helm setup: %w", err)
	}

	log.Info("downloading cilium chart", zap.String("repo", repoURL))
	ch, err := helmLocateChart(cfg, settings)
	if err != nil {
		return fmt.Errorf("locating cilium chart: %w", err)
	}
	log.Info("chart downloaded", zap.String("version", ch.Metadata.Version))

	if err := helmDeploy(ctx, cfg, ch, vals); err != nil {
		return fmt.Errorf("helm install cilium: %w", err)
	}
	log.Info("cilium deployed successfully")

	if err := waitForNodes(ctx, log); err != nil {
		return fmt.Errorf("waiting for nodes: %w", err)
	}
	log.Info("all nodes are ready")
	return nil
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

// helmSetup initialises Helm's action configuration and CLI settings.
func helmSetup(log *zap.Logger) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	cfg := &action.Configuration{}
	if err := cfg.Init(settings.RESTClientGetter(), namespace, "secret", func(format string, v ...interface{}) {
		log.Debug(fmt.Sprintf(format, v...))
	}); err != nil {
		return nil, nil, fmt.Errorf("initializing helm config: %w", err)
	}
	return cfg, settings, nil
}

// helmLocateChart downloads and loads the Cilium chart from the repo.
func helmLocateChart(cfg *action.Configuration, settings *cli.EnvSettings) (*chart.Chart, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.RepoURL = repoURL

	chartPath, err := install.ChartPathOptions.LocateChart(chartName, settings)
	if err != nil {
		return nil, fmt.Errorf("locating cilium chart: %w", err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading cilium chart: %w", err)
	}
	return ch, nil
}

// helmDeploy runs the Helm install with a spinner for visual feedback.
func helmDeploy(ctx context.Context, cfg *action.Configuration, ch *chart.Chart, vals map[string]interface{}) error {
	// Suppress the debug log callback during deploy — it fires concurrently
	// and would garble the spinner output.
	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.Wait = true
	install.Timeout = helmTimeout

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Deploying Cilium to kube-system (this may take a few minutes)..."
	s.Start()
	_, err := install.RunWithContext(ctx, ch, vals)
	s.Stop()
	if err != nil {
		return fmt.Errorf("running helm install: %w", err)
	}
	return nil
}

// waitForNodes polls all Kubernetes nodes until every node reports
// condition Ready=True, or the timeout expires. A spinner shows progress.
func waitForNodes(ctx context.Context, _ *zap.Logger) error {
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

	deadline := time.Now().Add(nodeTimeout)
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

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for nodes to become ready (%d/%d ready)", ready, total)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
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
