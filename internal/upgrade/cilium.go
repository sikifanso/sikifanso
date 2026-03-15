package upgrade

import (
	"context"

	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
)

// Cilium upgrades the Cilium CNI.
func Cilium(ctx context.Context, opts Opts, apiServerIP string) (*Result, error) {
	cfg := opts.InfraConfig
	vals := infraconfig.MergeValues(cfg.CiliumValues,
		infraconfig.CiliumRuntimeOverrides(cfg.Platform.NodePorts, apiServerIP))

	return upgradeComponent(ctx, opts, "Cilium", cfg.Cilium, vals)
}
