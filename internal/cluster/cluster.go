package cluster

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"

	"github.com/alicanalbayrak/sikifanso/internal/cilium"
	k3dclient "github.com/k3d-io/k3d/v5/pkg/client"
	k3dconfig "github.com/k3d-io/k3d/v5/pkg/config"
	configtypes "github.com/k3d-io/k3d/v5/pkg/config/types"
	conf "github.com/k3d-io/k3d/v5/pkg/config/v1alpha5"
	k3drt "github.com/k3d-io/k3d/v5/pkg/runtimes"
	k3d "github.com/k3d-io/k3d/v5/pkg/types"
)

// Create creates a new single-server k3d cluster using the SimpleConfig pipeline.
func Create(ctx context.Context, log *zap.Logger, name string) error {
	exists, err := Exists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("cluster %q already exists", name)
	}

	// Prevent k3d DNS fix that breaks Docker Desktop.
	// See: https://github.com/k3d-io/k3d/issues/1515
	os.Setenv("K3D_FIX_DNS", "0")

	log.Info("creating k3d cluster", zap.String("cluster", name))

	simpleCfg := conf.SimpleConfig{
		TypeMeta: configtypes.TypeMeta{
			Kind:       "Simple",
			APIVersion: conf.ApiVersion,
		},
		ObjectMeta: configtypes.ObjectMeta{
			Name: name,
		},
		Servers: 1,
		Agents:  2,
		Image: "rancher/k3s:v1.29.1-k3s2",
		ExposeAPI: conf.SimpleExposureOpts{
			HostPort: "6443",
		},
		Ports: []conf.PortWithNodeFilters{
			{Port: "80:30082", NodeFilters: []string{"server:*"}},
			{Port: "443:30083", NodeFilters: []string{"server:*"}},
			{Port: "30081:30081", NodeFilters: []string{"server:*"}},
		},
		Options: conf.SimpleConfigOptions{
			K3sOptions: conf.SimpleConfigOptionsK3s{
				ExtraArgs: []conf.K3sArgWithNodeFilters{
					{Arg: "--flannel-backend=none", NodeFilters: []string{"server:*"}},
					{Arg: "--disable-network-policy", NodeFilters: []string{"server:*"}},
					{Arg: "--disable=traefik", NodeFilters: []string{"server:*"}},
					{Arg: "--disable=servicelb", NodeFilters: []string{"server:*"}},
				},
			},
		},
	}

	if err := k3dconfig.ProcessSimpleConfig(&simpleCfg); err != nil {
		return fmt.Errorf("processing simple config: %w", err)
	}

	clusterCfg, err := k3dconfig.TransformSimpleToClusterConfig(ctx, k3drt.Docker, simpleCfg, "")
	if err != nil {
		return fmt.Errorf("transforming config: %w", err)
	}

	clusterCfg, err = k3dconfig.ProcessClusterConfig(*clusterCfg)
	if err != nil {
		return fmt.Errorf("processing cluster config: %w", err)
	}

	if err := k3dconfig.ValidateClusterConfig(ctx, k3drt.Docker, *clusterCfg); err != nil {
		return fmt.Errorf("validating cluster config: %w", err)
	}

	if err := k3dclient.ClusterRun(ctx, k3drt.Docker, clusterCfg); err != nil {
		// Roll back on failure.
		log.Warn("cluster creation failed, rolling back", zap.Error(err))
		_ = k3dclient.ClusterDelete(ctx, k3drt.Docker, &clusterCfg.Cluster, k3d.ClusterDeleteOpts{SkipRegistryCheck: true})
		return fmt.Errorf("creating cluster %q: %w", name, err)
	}

	log.Info("writing kubeconfig")
	if _, err := k3dclient.KubeconfigGetWrite(ctx, k3drt.Docker, &clusterCfg.Cluster, "", &k3dclient.WriteKubeConfigOptions{
		UpdateExisting:       true,
		UpdateCurrentContext: true,
		OverwriteExisting:    false,
	}); err != nil {
		return fmt.Errorf("writing kubeconfig: %w", err)
	}

	if err := cilium.Install(ctx, log, name); err != nil {
		return fmt.Errorf("installing cilium: %w", err)
	}

	log.Info("k3d cluster created successfully", zap.String("cluster", name))
	return nil
}

// Delete deletes the k3d cluster with the given name and removes its kubeconfig entry.
func Delete(ctx context.Context, log *zap.Logger, name string) error {
	log.Info("looking up k3d cluster", zap.String("cluster", name))

	cluster, err := k3dclient.ClusterGet(ctx, k3drt.Docker, &k3d.Cluster{Name: name})
	if err != nil {
		return fmt.Errorf("cluster %q not found: %w", name, err)
	}

	log.Info("deleting k3d cluster", zap.String("cluster", name))

	if err := k3dclient.ClusterDelete(ctx, k3drt.Docker, cluster, k3d.ClusterDeleteOpts{SkipRegistryCheck: true}); err != nil {
		return fmt.Errorf("deleting cluster %q: %w", name, err)
	}

	log.Info("removing kubeconfig entries", zap.String("cluster", name))
	if err := k3dclient.KubeconfigRemoveClusterFromDefaultConfig(ctx, cluster); err != nil {
		log.Warn("failed to clean up kubeconfig", zap.Error(err))
	}

	log.Info("k3d cluster deleted successfully", zap.String("cluster", name))
	return nil
}

// Exists checks whether a k3d cluster with the given name already exists.
func Exists(ctx context.Context, name string) (bool, error) {
	clusters, err := k3dclient.ClusterList(ctx, k3drt.Docker)
	if err != nil {
		return false, fmt.Errorf("listing clusters: %w", err)
	}
	for _, c := range clusters {
		if c.Name == name {
			return true, nil
		}
	}
	return false, nil
}
