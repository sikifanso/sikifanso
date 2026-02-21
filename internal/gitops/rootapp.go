package gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// gvrFrom derives a GroupVersionResource from the object's apiVersion and kind.
func gvrFrom(obj *unstructured.Unstructured) schema.GroupVersionResource {
	gv, _ := schema.ParseGroupVersion(obj.GetAPIVersion())
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: strings.ToLower(obj.GetKind()) + "s",
	}
}

// ApplyRootApp reads the root Application manifest from the gitops directory
// and applies it to the cluster via the dynamic client.
func ApplyRootApp(ctx context.Context, log *zap.Logger, gitopsDir string) error {
	manifestPath := filepath.Join(gitopsDir, "bootstrap", "root-app.yaml")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("reading root app manifest: %w", err)
	}

	var obj unstructured.Unstructured
	if err := yaml.Unmarshal(data, &obj.Object); err != nil {
		return fmt.Errorf("unmarshaling root app manifest: %w", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = "argocd"
	}

	gvr := gvrFrom(&obj)
	log.Info("applying root application",
		zap.String("name", obj.GetName()),
		zap.String("namespace", ns),
		zap.String("resource", gvr.Resource),
	)
	_, err = dc.Resource(gvr).Namespace(ns).Create(ctx, &obj, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Warn("root application already exists, skipping", zap.String("name", obj.GetName()))
			return nil
		}
		return fmt.Errorf("creating root application: %w", err)
	}

	log.Info("root application created", zap.String("name", obj.GetName()))
	return nil
}
