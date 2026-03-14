package upgrade

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
)

const (
	argocdRepoURL     = "https://argoproj.github.io/argo-helm"
	argocdChartName   = "argo-cd"
	argocdReleaseName = "argocd"
	argocdNamespace   = "argocd"
)

// ArgoCD upgrades the ArgoCD installation.
func ArgoCD(ctx context.Context, opts Opts) (*Result, error) {
	vals := argocd.Values()
	return upgradeComponent(ctx, opts, "ArgoCD", argocdNamespace, argocdRepoURL, argocdChartName, argocdReleaseName, vals)
}
