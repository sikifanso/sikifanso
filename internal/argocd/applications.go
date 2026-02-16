package argocd

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var applicationGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applications",
}

// AppParams describes a Helm-based ArgoCD Application to register.
type AppParams struct {
	Name         string
	Namespace    string // target namespace (e.g. "kube-system")
	RepoURL      string
	ChartName    string
	ChartVersion string
	Values       map[string]interface{}
}

// CreateApplications registers each app as an ArgoCD Application CRD.
// If an Application already exists, it logs a warning and continues.
func CreateApplications(ctx context.Context, log *zap.Logger, apps ...AppParams) error {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	for _, app := range apps {
		obj, err := buildApplication(app)
		if err != nil {
			return fmt.Errorf("building application %q: %w", app.Name, err)
		}

		log.Info("creating argocd application", zap.String("name", app.Name))
		_, err = dc.Resource(applicationGVR).Namespace("argocd").Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			if apierrors.IsAlreadyExists(err) {
				log.Warn("argocd application already exists, skipping", zap.String("name", app.Name))
				continue
			}
			return fmt.Errorf("creating application %q: %w", app.Name, err)
		}
		log.Info("argocd application created", zap.String("name", app.Name))
	}

	return nil
}

func buildApplication(app AppParams) (*unstructured.Unstructured, error) {
	source := map[string]interface{}{
		"repoURL":        app.RepoURL,
		"chart":          app.ChartName,
		"targetRevision": app.ChartVersion,
		"helm": map[string]interface{}{
			"releaseName": app.Name,
		},
	}

	if len(app.Values) > 0 {
		valuesYAML, err := yaml.Marshal(app.Values)
		if err != nil {
			return nil, fmt.Errorf("marshaling values: %w", err)
		}
		source["helm"].(map[string]interface{})["values"] = string(valuesYAML)
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      app.Name,
				"namespace": "argocd",
				"finalizers": []interface{}{
					"resources-finalizer.argocd.argoproj.io",
				},
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source":  source,
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": app.Namespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"selfHeal": false,
						"prune":    false,
					},
					"syncOptions": []interface{}{
						"CreateNamespace=true",
						"ServerSideApply=true",
					},
				},
			},
		},
	}

	return obj, nil
}
