package cluster

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/cilium"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	k3dclient "github.com/k3d-io/k3d/v5/pkg/client"
	k3dconfig "github.com/k3d-io/k3d/v5/pkg/config"
	configtypes "github.com/k3d-io/k3d/v5/pkg/config/types"
	conf "github.com/k3d-io/k3d/v5/pkg/config/v1alpha5"
	k3drt "github.com/k3d-io/k3d/v5/pkg/runtimes"
	k3d "github.com/k3d-io/k3d/v5/pkg/types"
)

// TODO - Make these configurable
// TODO - Renovate ?
const (
	k3sImage   = "rancher/k3s:v1.29.1-k3s2"
	k3dServers = 1
	k3dAgents  = 2
)

// Options configures cluster creation.
type Options struct {
	BootstrapURL string
}

// Create creates a new single-server k3d cluster using the SimpleConfig pipeline.
func Create(ctx context.Context, log *zap.Logger, name string, opts Options) (*session.Session, error) {
	exists, err := Exists(ctx, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("cluster %q already exists", name)
	}

	// Resolve host ports — tries defaults first, falls back to free ports.
	hp, err := resolveHostPorts()
	if err != nil {
		return nil, fmt.Errorf("resolving host ports: %w", err)
	}
	log.Info("resolved host ports",
		zap.Int("apiServer", hp.APIServer),
		zap.Int("http", hp.HTTP),
		zap.Int("https", hp.HTTPS),
		zap.Int("argocdUI", hp.ArgoCDUI),
		zap.Int("hubbleUI", hp.HubbleUI),
	)

	// Scaffold gitops repo before cluster creation so the directory exists for the volume mount.
	// Remove any stale session directory left over from a previous failed creation.
	if err := session.Remove(name); err != nil {
		log.Warn("failed to clean up stale session directory", zap.Error(err))
	}

	gitopsDir, err := session.GitOpsDir(name)
	if err != nil {
		return nil, fmt.Errorf("resolving gitops directory: %w", err)
	}
	if err := gitops.Scaffold(ctx, log, opts.BootstrapURL, gitopsDir); err != nil {
		return nil, fmt.Errorf("scaffolding gitops repo: %w", err)
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
		Servers: k3dServers,
		Agents:  k3dAgents,
		Image:   k3sImage,
		ExposeAPI: conf.SimpleExposureOpts{
			HostPort: fmt.Sprintf("%d", hp.APIServer),
		},
		Ports: []conf.PortWithNodeFilters{
			{Port: fmt.Sprintf("%d:30082", hp.HTTP), NodeFilters: []string{"server:*"}},
			{Port: fmt.Sprintf("%d:30083", hp.HTTPS), NodeFilters: []string{"server:*"}},
			{Port: fmt.Sprintf("%d:30080", hp.ArgoCDUI), NodeFilters: []string{"server:*"}},
			{Port: fmt.Sprintf("%d:30081", hp.HubbleUI), NodeFilters: []string{"server:*"}},
		},
		Volumes: []conf.VolumeWithNodeFilters{
			{
				Volume:      gitopsDir + ":/local-gitops",
				NodeFilters: []string{"all"},
			},
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
		return nil, fmt.Errorf("processing simple config: %w", err)
	}

	clusterCfg, err := k3dconfig.TransformSimpleToClusterConfig(ctx, k3drt.Docker, simpleCfg, "")
	if err != nil {
		return nil, fmt.Errorf("transforming config: %w", err)
	}

	clusterCfg, err = k3dconfig.ProcessClusterConfig(*clusterCfg)
	if err != nil {
		return nil, fmt.Errorf("processing cluster config: %w", err)
	}

	if err := k3dconfig.ValidateClusterConfig(ctx, k3drt.Docker, *clusterCfg); err != nil {
		return nil, fmt.Errorf("validating cluster config: %w", err)
	}

	if err := k3dclient.ClusterRun(ctx, k3drt.Docker, clusterCfg); err != nil {
		// Roll back on failure.
		log.Warn("cluster creation failed, rolling back", zap.Error(err))
		_ = k3dclient.ClusterDelete(ctx, k3drt.Docker, &clusterCfg.Cluster, k3d.ClusterDeleteOpts{SkipRegistryCheck: true})
		return nil, fmt.Errorf("creating cluster %q: %w", name, err)
	}

	log.Info("writing kubeconfig")
	if _, err := k3dclient.KubeconfigGetWrite(ctx, k3drt.Docker, &clusterCfg.Cluster, "", &k3dclient.WriteKubeConfigOptions{
		UpdateExisting:       true,
		UpdateCurrentContext: true,
		OverwriteExisting:    false,
	}); err != nil {
		return nil, fmt.Errorf("writing kubeconfig: %w", err)
	}

	ciliumResult, err := cilium.Install(ctx, log, name)
	if err != nil {
		return nil, fmt.Errorf("installing cilium: %w", err)
	}

	argocdResult, err := argocd.Install(ctx, log)
	if err != nil {
		return nil, fmt.Errorf("installing argocd: %w", err)
	}

	if err := argocd.CreateApplications(ctx, log,
		argocd.AppParams{
			Name: "cilium", Namespace: "kube-system",
			RepoURL: "https://helm.cilium.io/", ChartName: "cilium",
			ChartVersion: ciliumResult.ChartVersion,
			Values:       cilium.Values(ciliumResult.APIServerIP),
		},
		argocd.AppParams{
			Name: "argocd", Namespace: "argocd",
			RepoURL: "https://argoproj.github.io/argo-helm", ChartName: "argo-cd",
			ChartVersion: argocdResult.ChartVersion,
			Values:       argocd.Values(),
		},
	); err != nil {
		return nil, fmt.Errorf("creating argocd applications: %w", err)
	}

	// Apply root application from the scaffolded gitops repo.
	if err := gitops.ApplyRootApp(ctx, log, gitopsDir); err != nil {
		return nil, fmt.Errorf("applying root application: %w", err)
	}

	// Build and save session.
	sess := &session.Session{
		ClusterName:  name,
		State:        "running",
		CreatedAt:    time.Now(),
		BootstrapURL: opts.BootstrapURL,
		GitOpsPath:   gitopsDir,
		Services: session.ServiceInfo{
			ArgoCD: session.ArgoCDInfo{
				URL:          fmt.Sprintf("http://localhost:%d", hp.ArgoCDUI),
				Username:     "admin",
				Password:     argocdResult.AdminPassword,
				ChartVersion: argocdResult.ChartVersion,
			},
			Hubble: session.HubbleInfo{
				URL: fmt.Sprintf("http://localhost:%d", hp.HubbleUI),
			},
		},
		K3dConfig: session.K3dConfigInfo{
			Image:   k3sImage,
			Servers: k3dServers,
			Agents:  k3dAgents,
		},
	}

	if err := session.Save(sess); err != nil {
		log.Warn("failed to save session", zap.Error(err))
	}

	log.Info("k3d cluster created successfully", zap.String("cluster", name))
	return sess, nil
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

	// Clean up session directory (non-fatal).
	if err := session.Remove(name); err != nil {
		log.Warn("failed to remove session directory", zap.Error(err))
	}

	log.Info("k3d cluster deleted successfully", zap.String("cluster", name))
	return nil
}

// Stop stops a running k3d cluster and updates the session state.
func Stop(ctx context.Context, log *zap.Logger, name string) error {
	log.Info("looking up k3d cluster", zap.String("cluster", name))

	cluster, err := k3dclient.ClusterGet(ctx, k3drt.Docker, &k3d.Cluster{Name: name})
	if err != nil {
		return fmt.Errorf("cluster %q not found: %w", name, err)
	}

	log.Info("stopping k3d cluster", zap.String("cluster", name))

	if err := k3dclient.ClusterStop(ctx, k3drt.Docker, cluster); err != nil {
		return fmt.Errorf("stopping cluster %q: %w", name, err)
	}

	sess, err := session.Load(name)
	if err != nil {
		log.Warn("failed to load session", zap.Error(err))
	} else {
		sess.State = "stopped"
		if err := session.Save(sess); err != nil {
			log.Warn("failed to save session", zap.Error(err))
		}
	}

	log.Info("k3d cluster stopped successfully", zap.String("cluster", name))
	return nil
}

// Start starts a stopped k3d cluster and updates the session state.
func Start(ctx context.Context, log *zap.Logger, name string) error {
	log.Info("looking up k3d cluster", zap.String("cluster", name))

	cluster, err := k3dclient.ClusterGet(ctx, k3drt.Docker, &k3d.Cluster{Name: name})
	if err != nil {
		return fmt.Errorf("cluster %q not found: %w", name, err)
	}

	// Prevent k3d DNS fix that breaks Docker Desktop.
	// See: https://github.com/k3d-io/k3d/issues/1515
	os.Setenv("K3D_FIX_DNS", "0")

	// Gather environment info (host gateway IP etc.) required by ClusterStart
	// for host alias injection into CoreDNS.
	envInfo, err := k3dclient.GatherEnvironmentInfo(ctx, k3drt.Docker, cluster)
	if err != nil {
		return fmt.Errorf("gathering environment info: %w", err)
	}

	log.Info("starting k3d cluster", zap.String("cluster", name))

	if err := k3dclient.ClusterStart(ctx, k3drt.Docker, cluster, k3d.ClusterStartOpts{
		WaitForServer:   true,
		EnvironmentInfo: envInfo,
	}); err != nil {
		return fmt.Errorf("starting cluster %q: %w", name, err)
	}

	sess, err := session.Load(name)
	if err != nil {
		log.Warn("failed to load session", zap.Error(err))
	} else {
		sess.State = "running"
		if err := session.Save(sess); err != nil {
			log.Warn("failed to save session", zap.Error(err))
		}
	}

	log.Info("k3d cluster started successfully", zap.String("cluster", name))
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
