package argocd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

var (
	appHealthTimeout  = 3 * time.Minute
	appHealthInterval = 5 * time.Second
)

// WaitForApplicationsHealthy polls the named ArgoCD Applications until all
// report Synced+Healthy, or the timeout expires. It uses the dynamic K8s
// client to read Application CRD status directly — no ArgoCD gRPC dependency.
func WaitForApplicationsHealthy(ctx context.Context, log *zap.Logger, restCfg *rest.Config, namespace string, names []string) error {
	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " Waiting for infrastructure applications to be healthy..."
	s.Start()
	defer s.Stop()

	ctx, cancel := context.WithTimeout(ctx, appHealthTimeout)
	defer cancel()

	log.Info("waiting for applications to be healthy", zap.Strings("apps", names))

	for {
		ready := 0
		for _, name := range names {
			synced, healthy, err := getAppStatus(ctx, dc, namespace, name)
			if err != nil {
				log.Debug("could not read application status", zap.String("app", name), zap.Error(err))
				continue
			}
			if synced && healthy {
				ready++
			}
		}

		s.Suffix = fmt.Sprintf(" Waiting for infrastructure applications (%d/%d healthy)...", ready, len(names))

		if ready == len(names) {
			log.Info("all infrastructure applications are healthy")
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for applications to be healthy (%d/%d ready)", ready, len(names))
		case <-time.After(appHealthInterval):
		}
	}
}

// getAppStatus reads the sync and health status of an ArgoCD Application CRD.
func getAppStatus(ctx context.Context, dc dynamic.Interface, namespace, name string) (synced, healthy bool, err error) {
	obj, err := dc.Resource(applicationGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return false, false, err
	}

	syncStatus, _, _ := unstructured.NestedString(obj.Object, "status", "sync", "status")
	healthStatus, _, _ := unstructured.NestedString(obj.Object, "status", "health", "status")

	return syncStatus == "Synced", healthStatus == "Healthy", nil
}
