package upgrade

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
)

// ArgoCD upgrades the ArgoCD installation.
func ArgoCD(ctx context.Context, opts Opts) (*Result, error) {
	cfg := opts.InfraConfig
	vals := infraconfig.MergeValues(cfg.ArgoCDValues,
		infraconfig.ArgoCDRuntimeOverrides(cfg.Platform.NodePorts))

	return upgradeComponent(ctx, opts, "ArgoCD", cfg.ArgoCD, vals)
}
