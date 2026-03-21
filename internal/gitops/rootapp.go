package gitops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// bootstrapManifests lists the manifests that ApplyRootApp applies to the cluster.
var bootstrapManifests = []string{
	"bootstrap/root-app.yaml",
	"bootstrap/root-catalog.yaml",
	"bootstrap/root-agents.yaml",
}

// ApplyRootApp reads the bootstrap ApplicationSet manifests from the gitops
// directory and applies them to the cluster via Server-Side Apply.
func ApplyRootApp(ctx context.Context, log *zap.Logger, gitopsDir string) error {
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %w", err)
	}

	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))

	for _, rel := range bootstrapManifests {
		if err := applyManifest(ctx, log, dc, mapper, filepath.Join(gitopsDir, rel)); err != nil {
			return err
		}
	}

	return nil
}

// applyManifest reads a single YAML manifest and applies it via Server-Side Apply.
func applyManifest(ctx context.Context, log *zap.Logger, dc dynamic.Interface, mapper *restmapper.DeferredDiscoveryRESTMapper, path string) error {
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

	gvk := obj.GroupVersionKind()
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return fmt.Errorf("mapping GVK %s for manifest %s: %w", gvk, filepath.Base(path), err)
	}

	log.Info("applying bootstrap manifest (SSA)",
		zap.String("name", obj.GetName()),
		zap.String("namespace", ns),
		zap.String("resource", mapping.Resource.Resource),
	)

	jsonData, err := json.Marshal(obj.Object)
	if err != nil {
		return fmt.Errorf("marshaling %s to JSON: %w", obj.GetName(), err)
	}

	_, err = dc.Resource(mapping.Resource).Namespace(ns).Patch(
		ctx,
		obj.GetName(),
		types.ApplyPatchType,
		jsonData,
		metav1.PatchOptions{
			FieldManager: "sikifanso",
			Force:        ptr.To(true),
		},
	)
	if err != nil {
		return fmt.Errorf("applying %s: %w", obj.GetName(), err)
	}

	log.Info("manifest applied", zap.String("name", obj.GetName()))
	return nil
}
