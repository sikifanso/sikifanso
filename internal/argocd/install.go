package argocd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/helm"
	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
	"github.com/briandowns/spinner"
	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// InstallResult holds metadata from a successful ArgoCD install.
type InstallResult struct {
	ChartVersion  string
	AdminPassword string
}

const (
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
// Chart coordinates and pre-merged values are provided by the caller
// via the InfraConfig. It returns an InstallResult with chart version
// and admin password.
func Install(ctx context.Context, log *zap.Logger, restCfg *rest.Config, chart infraconfig.ChartConfig, vals map[string]interface{}) (*InstallResult, error) {
	params := helm.InstallParams{
		Namespace:     chart.Namespace,
		RepoURL:       chart.RepoURL,
		ChartName:     chart.Chart,
		ReleaseName:   chart.ReleaseName,
		Timeout:       5 * time.Minute,
		CreateNS:      true,
		SpinnerSuffix: fmt.Sprintf(" Deploying ArgoCD to %s namespace (this may take a few minutes)...", chart.Namespace),
	}

	cfg, settings, err := helm.Setup(log, chart.Namespace)
	if err != nil {
		return nil, fmt.Errorf("helm setup: %w", err)
	}

	log.Info("downloading argocd chart", zap.String("repo", params.RepoURL))
	ch, err := helm.LocateChart(cfg, settings, params)
	if err != nil {
		return nil, fmt.Errorf("locating argocd chart: %w", err)
	}
	log.Info("chart downloaded", zap.String("version", ch.Metadata.Version))

	if err := helm.Deploy(ctx, cfg, ch, vals, params); err != nil {
		return nil, fmt.Errorf("helm install argocd: %w", err)
	}
	log.Info("argocd helm release deployed")

	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}

	if err := waitForDeployments(ctx, kubeClient, chart.Namespace); err != nil {
		return nil, fmt.Errorf("waiting for argocd deployments: %w", err)
	}
	log.Info("argocd deployments are ready")

	result := &InstallResult{ChartVersion: ch.Metadata.Version}

	password, err := extractAdminPassword(ctx, kubeClient, chart.Namespace)
	if err != nil {
		log.Warn("could not extract admin password", zap.Error(err))
	} else {
		log.Info("argocd admin password extracted")
		result.AdminPassword = password
	}

	return result, nil
}

// waitForDeployments polls the key ArgoCD deployments until they all
// report the Available condition, or the timeout expires.
func waitForDeployments(ctx context.Context, kubeClient kubernetes.Interface, namespace string) error {
	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Waiting for ArgoCD deployments to be ready..."
	s.Start()
	defer s.Stop()

	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()

	for {
		ready := 0
		for _, name := range deploymentNames {
			dep, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for argocd deployments (%d/%d ready)", ready, total)
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
func extractAdminPassword(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (string, error) {
	secret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, "argocd-initial-admin-secret", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting argocd-initial-admin-secret: %w", err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", fmt.Errorf("password key not found in argocd-initial-admin-secret")
	}
	return string(password), nil
}
