package upgrade

import (
	"context"
	"fmt"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/helm"
	"github.com/alicanalbayrak/sikifanso/internal/snapshot"
	"go.uber.org/zap"
)

// Result holds the outcome of a component upgrade.
type Result struct {
	Component    string
	OldVersion   string
	NewVersion   string
	SnapshotName string
	Skipped      bool
	SkipReason   string
}

// Opts configures an upgrade operation.
type Opts struct {
	ClusterName  string
	CLIVersion   string
	SkipSnapshot bool
	Log          *zap.Logger
}

// preUpgradeSnapshot captures state before upgrade, returns snapshot name.
func preUpgradeSnapshot(opts Opts, component string) (string, error) {
	if opts.SkipSnapshot {
		return "", nil
	}
	snapshotName := fmt.Sprintf("pre-upgrade-%s-%s", component, time.Now().Format("20060102-150405"))
	_, err := snapshot.Capture(opts.ClusterName, snapshotName, opts.CLIVersion)
	if err != nil {
		return "", fmt.Errorf("creating pre-upgrade snapshot: %w", err)
	}
	return snapshotName, nil
}

// upgradeComponent is the generic upgrade flow for a Helm-managed component.
func upgradeComponent(ctx context.Context, opts Opts, component, namespace, repoURL, chartName, releaseName string, vals map[string]interface{}) (*Result, error) {
	log := opts.Log

	cfg, settings, err := helm.Setup(log, namespace)
	if err != nil {
		return nil, fmt.Errorf("setting up helm for %s: %w", component, err)
	}

	currentVer, err := helm.CurrentVersion(cfg, releaseName)
	if err != nil {
		return nil, fmt.Errorf("getting current %s version: %w", component, err)
	}

	ch, err := helm.LocateChart(cfg, settings, helm.InstallParams{
		Namespace:   namespace,
		RepoURL:     repoURL,
		ChartName:   chartName,
		ReleaseName: releaseName,
	})
	if err != nil {
		return nil, fmt.Errorf("locating latest %s chart: %w", component, err)
	}

	newVer := ch.Metadata.Version
	if currentVer == newVer {
		return &Result{
			Component:  component,
			OldVersion: currentVer,
			NewVersion: newVer,
			Skipped:    true,
			SkipReason: "already at latest version",
		}, nil
	}

	snapshotName, err := preUpgradeSnapshot(opts, component)
	if err != nil {
		log.Warn("pre-upgrade snapshot failed, continuing", zap.Error(err))
	}

	log.Info("upgrading component",
		zap.String("component", component),
		zap.String("from", currentVer),
		zap.String("to", newVer),
	)

	upgradeErr := helm.Upgrade(ctx, cfg, ch, vals, helm.UpgradeParams{
		Namespace:     namespace,
		ReleaseName:   releaseName,
		RepoURL:       repoURL,
		ChartName:     chartName,
		Timeout:       10 * time.Minute,
		SpinnerSuffix: fmt.Sprintf(" Upgrading %s %s -> %s", component, currentVer, newVer),
	})

	if upgradeErr != nil {
		log.Error("upgrade failed, rolling back", zap.String("component", component), zap.Error(upgradeErr))
		if rbErr := helm.Rollback(cfg, releaseName); rbErr != nil {
			log.Error("rollback also failed", zap.Error(rbErr))
		}
		if snapshotName != "" {
			return nil, fmt.Errorf("upgrading %s: %w (snapshot %q available for manual recovery)", component, upgradeErr, snapshotName)
		}
		return nil, fmt.Errorf("upgrading %s: %w", component, upgradeErr)
	}

	return &Result{
		Component:    component,
		OldVersion:   currentVer,
		NewVersion:   newVer,
		SnapshotName: snapshotName,
	}, nil
}
