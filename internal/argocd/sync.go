package argocd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// webhookPayload is a minimal GitHub push event that matches our local repo URL.
var webhookPayload = []byte(`{"repository":{"clone_url":"/local-gitops","html_url":"/local-gitops","ssh_url":"/local-gitops"}}`)

// Sync sends webhook push events to both the ArgoCD server (to invalidate
// the repo-server cache) and the ApplicationSet controller (to trigger
// immediate reconciliation).
func Sync(ctx context.Context, log *zap.Logger, clusterName, argocdURL string) error {
	// 1. ArgoCD server webhook — invalidates repo-server cache.
	if err := sendWebhook(ctx, log, argocdURL+"/api/webhook"); err != nil {
		return fmt.Errorf("argocd server webhook: %w", err)
	}

	// 2. ApplicationSet controller webhook — triggers reconciliation.
	//    Reachable only inside the cluster, so we proxy through the API server.
	if err := sendAppSetWebhook(ctx, log, clusterName); err != nil {
		return fmt.Errorf("applicationset webhook: %w", err)
	}

	return nil
}

func sendWebhook(ctx context.Context, log *zap.Logger, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(webhookPayload))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")

	log.Info("sending webhook", zap.String("url", url))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("returned %s", resp.Status)
	}
	return nil
}

// sendAppSetWebhook proxies a webhook through the Kubernetes API server
// to the ApplicationSet controller service (ClusterIP, port 7000).
func sendAppSetWebhook(ctx context.Context, log *zap.Logger, clusterName string) error {
	kubeContext := "k3d-" + clusterName

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: kubeContext},
	).ClientConfig()
	if err != nil {
		return fmt.Errorf("building kubeconfig for context %s: %w", kubeContext, err)
	}

	cs, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating clientset: %w", err)
	}

	log.Info("sending webhook to applicationset controller", zap.String("context", kubeContext))
	result := cs.CoreV1().RESTClient().Post().
		Namespace("argocd").
		Resource("services").
		Name("argocd-applicationset-controller:http-webhook").
		SubResource("proxy").
		Suffix("api", "webhook").
		SetHeader("Content-Type", "application/json").
		SetHeader("X-GitHub-Event", "push").
		Body(webhookPayload).
		Do(ctx)

	if err := result.Error(); err != nil {
		return err
	}
	return nil
}
