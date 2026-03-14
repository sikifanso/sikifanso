package helm

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/briandowns/spinner"
	"go.uber.org/zap"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
)

// InstallParams holds the configuration for a Helm chart installation.
type InstallParams struct {
	Namespace     string
	RepoURL       string
	ChartName     string
	ReleaseName   string
	Timeout       time.Duration
	CreateNS      bool
	SpinnerSuffix string
}

// Setup initialises Helm's action configuration and CLI settings.
func Setup(log *zap.Logger, namespace string) (*action.Configuration, *cli.EnvSettings, error) {
	settings := cli.New()
	cfg := &action.Configuration{}
	if err := cfg.Init(settings.RESTClientGetter(), namespace, "secret", func(format string, v ...interface{}) {
		log.Debug(fmt.Sprintf(format, v...))
	}); err != nil {
		return nil, nil, fmt.Errorf("initializing helm config: %w", err)
	}
	return cfg, settings, nil
}

// LocateChart downloads and loads a Helm chart from the specified repo.
func LocateChart(cfg *action.Configuration, settings *cli.EnvSettings, p InstallParams) (*chart.Chart, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = p.ReleaseName
	install.Namespace = p.Namespace
	install.RepoURL = p.RepoURL

	chartPath, err := install.LocateChart(p.ChartName, settings)
	if err != nil {
		return nil, fmt.Errorf("locating chart %s: %w", p.ChartName, err)
	}
	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", p.ChartName, err)
	}
	return ch, nil
}

// Deploy runs the Helm install with a spinner for visual feedback.
func Deploy(ctx context.Context, cfg *action.Configuration, ch *chart.Chart, vals map[string]interface{}, p InstallParams) error {
	install := action.NewInstall(cfg)
	install.ReleaseName = p.ReleaseName
	install.Namespace = p.Namespace
	install.CreateNamespace = p.CreateNS
	install.Wait = true
	install.Timeout = p.Timeout

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = p.SpinnerSuffix
	s.Start()
	_, err := install.RunWithContext(ctx, ch, vals)
	s.Stop()
	if err != nil {
		return fmt.Errorf("running helm install: %w", err)
	}
	return nil
}

// UpgradeParams holds the configuration for a Helm chart upgrade.
type UpgradeParams struct {
	Namespace     string
	ReleaseName   string
	RepoURL       string
	ChartName     string
	Timeout       time.Duration
	SpinnerSuffix string
}

// Upgrade runs the Helm upgrade with a spinner for visual feedback.
func Upgrade(ctx context.Context, cfg *action.Configuration, ch *chart.Chart, vals map[string]interface{}, p UpgradeParams) error {
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = p.Namespace
	upgrade.Wait = true
	upgrade.Timeout = p.Timeout

	s := spinner.New(spinner.CharSets[11], 120*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = p.SpinnerSuffix
	s.Start()
	_, err := upgrade.RunWithContext(ctx, p.ReleaseName, ch, vals)
	s.Stop()
	if err != nil {
		return fmt.Errorf("running helm upgrade: %w", err)
	}
	return nil
}

// CurrentVersion returns the chart version of the currently deployed release.
func CurrentVersion(cfg *action.Configuration, releaseName string) (string, error) {
	list := action.NewList(cfg)
	list.Filter = "^" + regexp.QuoteMeta(releaseName) + "$"
	list.Deployed = true

	releases, err := list.Run()
	if err != nil {
		return "", fmt.Errorf("listing releases: %w", err)
	}
	if len(releases) == 0 {
		return "", fmt.Errorf("release %q not found", releaseName)
	}
	return releases[0].Chart.Metadata.Version, nil
}

// Rollback rolls back a Helm release to the previous version.
func Rollback(cfg *action.Configuration, releaseName string) error {
	rollback := action.NewRollback(cfg)
	rollback.Version = 0
	rollback.Wait = true

	if err := rollback.Run(releaseName); err != nil {
		return fmt.Errorf("running helm rollback: %w", err)
	}
	return nil
}
