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
// NOTE: the naive "+s" pluralisation only works for the known bootstrap
// manifests (e.g. ApplicationSet → applicationsets). If new kinds with
// irregular plurals are added, this must be replaced with a proper REST mapper.
func gvrFrom(obj *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(obj.GetAPIVersion())
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("parsing apiVersion %q: %w", obj.GetAPIVersion(), err)
	}
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: strings.ToLower(obj.GetKind()) + "s",
	}, nil
}

// bootstrapManifests lists the manifests that ApplyRootApp applies to the cluster.
var bootstrapManifests = []string{
	"bootstrap/root-app.yaml",
	"bootstrap/root-catalog.yaml",
}

// ApplyRootApp reads the bootstrap ApplicationSet manifests from the gitops
// directory and applies them to the cluster via the dynamic client.
func ApplyRootApp(ctx context.Context, log *zap.Logger, gitopsDir string) error {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	for _, rel := range bootstrapManifests {
		if err := applyManifest(ctx, log, dc, filepath.Join(gitopsDir, rel)); err != nil {
			return err
		}
	}

	return nil
}

// applyManifest reads a single YAML manifest and creates it in the cluster.
func applyManifest(ctx context.Context, log *zap.Logger, dc dynamic.Interface, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading manifest %s: %w", filepath.Base(path), err)
	}

	var obj unstructured.Unstructured
	if err := yaml.Unmarshal(data, &obj.Object); err != nil {
		return fmt.Errorf("unmarshaling manifest %s: %w", filepath.Base(path), err)
	}

	ns := obj.GetNamespace()
	if ns == "" {
		ns = "argocd"
	}

	gvr, err := gvrFrom(&obj)
	if err != nil {
		return fmt.Errorf("deriving GVR for manifest %s: %w", filepath.Base(path), err)
	}
	log.Info("applying bootstrap manifest",
		zap.String("name", obj.GetName()),
		zap.String("namespace", ns),
		zap.String("resource", gvr.Resource),
	)

	_, err = dc.Resource(gvr).Namespace(ns).Create(ctx, &obj, metav1.CreateOptions{})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			log.Warn("manifest already exists, skipping", zap.String("name", obj.GetName()))
			return nil
		}
		return fmt.Errorf("creating %s: %w", obj.GetName(), err)
	}

	log.Info("manifest created", zap.String("name", obj.GetName()))
	return nil
}
