package upgrade

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/cilium"
)

const (
	ciliumRepoURL     = "https://helm.cilium.io/"
	ciliumChartName   = "cilium"
	ciliumReleaseName = "cilium"
	ciliumNamespace   = "kube-system"
)

// Cilium upgrades the Cilium CNI.
func Cilium(ctx context.Context, opts Opts, apiServerIP string) (*Result, error) {
	vals := cilium.Values(apiServerIP)
	return upgradeComponent(ctx, opts, "Cilium", ciliumNamespace, ciliumRepoURL, ciliumChartName, ciliumReleaseName, vals)
}
