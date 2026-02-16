package argocd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	namespace    = "argocd"
	repoURL      = "https://argoproj.github.io/argo-helm"
	chartName    = "argo-cd"
	releaseName  = "argocd"
	helmTimeout  = 5 * time.Minute
	readyTimeout = 3 * time.Minute
	pollInterval = 5 * time.Second
)

// deploymentNames lists the ArgoCD deployments that must be Available
// before we consider the install complete.
var deploymentNames = []string{
	"argocd-server",
	"argocd-repo-server",
	"argocd-applicationset-controller",
}

// Install deploys ArgoCD into the cluster using the Helm Go SDK.
func Install(ctx context.Context, log *zap.Logger) error {
	vals := Values()

	cfg, settings, err := helmSetup(log)
	if err != nil {
		return fmt.Errorf("helm setup: %w", err)
	}

	log.Info("downloading argocd chart", zap.String("repo", repoURL))
	ch, err := helmLocateChart(cfg, settings)
	if err != nil {
		return fmt.Errorf("locating argocd chart: %w", err)
	}
	log.Info("chart downloaded", zap.String("version", ch.Metadata.Version))

	if err := helmDeploy(ctx, cfg, ch, vals); err != nil {
		return fmt.Errorf("helm install argocd: %w", err)
	}
	log.Info("argocd helm release deployed")

	if err := waitForDeployments(ctx, log); err != nil {
		return fmt.Errorf("waiting for argocd deployments: %w", err)
	}
	log.Info("argocd deployments are ready")

	password, err := extractAdminPassword(ctx)
	if err != nil {
		log.Warn("could not extract admin password", zap.Error(err))
	} else {
		log.Info("argocd admin password extracted")
		printAccessInfo(password)
	}

	return nil
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

// helmLocateChart downloads and loads the ArgoCD chart from argo-helm.
func helmLocateChart(cfg *action.Configuration, settings *cli.EnvSettings) (*chart.Chart, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.RepoURL = repoURL

	chartPath, err := install.ChartPathOptions.LocateChart(chartName, settings)
	if err != nil {
		return nil, fmt.Errorf("locating argocd chart: %w", err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading argocd chart: %w", err)
	}
	return ch, nil
}

// helmDeploy runs the Helm install with a spinner for visual feedback.
func helmDeploy(ctx context.Context, cfg *action.Configuration, ch *chart.Chart, vals map[string]interface{}) error {
	install := action.NewInstall(cfg)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = helmTimeout

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Deploying ArgoCD to argocd namespace (this may take a few minutes)..."
	s.Start()
	_, err := install.RunWithContext(ctx, ch, vals)
	s.Stop()
	if err != nil {
		return fmt.Errorf("running helm install: %w", err)
	}
	return nil
}

// waitForDeployments polls the key ArgoCD deployments until they all
// report the Available condition, or the timeout expires.
func waitForDeployments(ctx context.Context, _ *zap.Logger) error {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating kubernetes client: %w", err)
	}

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Waiting for ArgoCD deployments to be ready..."
	s.Start()
	defer s.Stop()

	deadline := time.Now().Add(readyTimeout)
	for {
		ready := 0
		for _, name := range deploymentNames {
			dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue
			}
			if deploymentAvailable(dep) {
				ready++
			}
		}

		total := len(deploymentNames)
		s.Suffix = fmt.Sprintf(" Waiting for ArgoCD deployments (%d/%d ready)...", ready, total)

		if ready == total {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for argocd deployments (%d/%d ready)", ready, total)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// deploymentAvailable checks whether a Deployment has the Available condition set to True.
func deploymentAvailable(dep *appsv1.Deployment) bool {
	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentAvailable {
			return c.Status == "True"
		}
	}
	return false
}

// extractAdminPassword reads the initial admin password from the
// argocd-initial-admin-secret Secret. client-go returns already-decoded
// bytes from secret.Data, so no base64 decoding is needed.
func extractAdminPassword(ctx context.Context) (string, error) {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return "", fmt.Errorf("building kubeconfig: %w", err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("creating kubernetes client: %w", err)
	}

	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, "argocd-initial-admin-secret", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting argocd-initial-admin-secret: %w", err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", fmt.Errorf("password key not found in argocd-initial-admin-secret")
	}
	return string(password), nil
}

// printAccessInfo prints ArgoCD access details to stderr (matches project
// convention: spinners/UI output goes to stderr).
func printAccessInfo(password string) {
	fmt.Fprintf(os.Stderr, "\n"+
		"╭─────────────────────────────────────────╮\n"+
		"│           ArgoCD Access Info            │\n"+
		"├─────────────────────────────────────────┤\n"+
		"│  URL:      http://localhost:30080       │\n"+
		"│  User:     admin                        │\n"+
		"│  Password: %-29s│\n"+
		"╰─────────────────────────────────────────╯\n\n",
		password)
}
