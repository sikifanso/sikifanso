package appsetreconcile

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const refreshAnnotation = "argocd.argoproj.io/application-set-refresh"

var appSetGVR = schema.GroupVersionResource{
	Group:    "argoproj.io",
	Version:  "v1alpha1",
	Resource: "applicationsets",
}

// Reconciler triggers ApplicationSet reconciliation by patching the refresh
// annotation on the CR. The ArgoCD ApplicationSet controller detects this,
// reconciles, then removes the annotation.
type Reconciler struct {
	dynClient dynamic.Interface
	namespace string
}

// NewReconciler creates a Reconciler that patches ApplicationSets in the given
// namespace. If namespace is empty it defaults to "argocd".
func NewReconciler(restCfg *rest.Config, namespace string) (*Reconciler, error) {
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	if namespace == "" {
		namespace = "argocd"
	}
	return &Reconciler{dynClient: dynClient, namespace: namespace}, nil
}

// Trigger patches the refresh annotation on each named ApplicationSet,
// causing the controller to reconcile immediately.
func (r *Reconciler) Trigger(ctx context.Context, appSetNames ...string) error {
	patch := []byte(`{"metadata":{"annotations":{"` + refreshAnnotation + `":"true"}}}`)
	for _, name := range appSetNames {
		_, err := r.dynClient.Resource(appSetGVR).Namespace(r.namespace).Patch(
			ctx, name, types.MergePatchType, patch, metav1.PatchOptions{},
		)
		if err != nil {
			return fmt.Errorf("triggering reconciliation for ApplicationSet %q: %w", name, err)
		}
	}
	return nil
}
