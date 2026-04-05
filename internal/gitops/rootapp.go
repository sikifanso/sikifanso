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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// infraManifests are applied first — they manage core platform apps
// (Cilium, ArgoCD) that must be healthy before workloads start.
var infraManifests = []string{
	"bootstrap/root-app.yaml",
}

// workloadManifests are applied after infrastructure apps are healthy.
var workloadManifests = []string{
	"bootstrap/root-catalog.yaml",
	"bootstrap/root-agents.yaml",
}

// ApplyRootApp reads the bootstrap ApplicationSet manifests from the gitops
// directory and applies them to the cluster via Server-Side Apply.
func ApplyRootApp(ctx context.Context, log *zap.Logger, restCfg *rest.Config, gitopsDir string) error {
	manifests := make([]string, 0, len(infraManifests)+len(workloadManifests))
	manifests = append(manifests, infraManifests...)
	manifests = append(manifests, workloadManifests...)
	return applyManifests(ctx, log, restCfg, gitopsDir, manifests)
}

// ApplyInfraManifests applies only the infrastructure ApplicationSet
// (root-app.yaml) that manages Cilium and ArgoCD.
func ApplyInfraManifests(ctx context.Context, log *zap.Logger, restCfg *rest.Config, gitopsDir string) error {
	return applyManifests(ctx, log, restCfg, gitopsDir, infraManifests)
}

// ApplyWorkloadManifests applies the catalog and agent ApplicationSets.
func ApplyWorkloadManifests(ctx context.Context, log *zap.Logger, restCfg *rest.Config, gitopsDir string) error {
	return applyManifests(ctx, log, restCfg, gitopsDir, workloadManifests)
}

func applyManifests(ctx context.Context, log *zap.Logger, restCfg *rest.Config, gitopsDir string, manifests []string) error {
	dc, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating dynamic client: %w", err)
	}

	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating discovery client: %w", err)
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(disc))

	for _, rel := range manifests {
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
