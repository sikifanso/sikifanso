package main

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/alicanalbayrak/sikifanso/internal/upgrade"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func upgradeCmd() *cli.Command {
	return &cli.Command{
		Name:  "upgrade",
		Usage: "Upgrade cluster components",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "all",
				Usage: "Upgrade all components",
			},
			&cli.BoolFlag{
				Name:  "skip-snapshot",
				Usage: "Skip pre-upgrade snapshot",
			},
		},
		Action: upgradeAction,
		Commands: []*cli.Command{
			upgradeCiliumCmd(),
			upgradeArgoCDCmd(),
		},
	}
}

func upgradeAction(ctx context.Context, cmd *cli.Command) error {
	if !cmd.Bool("all") {
		return cli.ShowSubcommandHelp(cmd)
	}

	clusterName := cmd.String("cluster")
	skipSnapshot := cmd.Bool("skip-snapshot")

	sess, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session: %w", err)
	}

	cfg, err := infraconfig.Load(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("loading infrastructure config: %w", err)
	}

	opts := upgrade.Opts{
		ClusterName:  clusterName,
		CLIVersion:   version,
		SkipSnapshot: skipSnapshot,
		Log:          zapLogger,
		InfraConfig:  cfg,
	}

	apiServerIP, err := apiServerIPForCluster(clusterName)
	if err != nil {
		zapLogger.Warn("could not detect API server IP, using empty string", zap.Error(err))
	}

	ciliumResult, err := upgrade.Cilium(ctx, opts, apiServerIP)
	if err != nil {
		return fmt.Errorf("upgrading cilium: %w", err)
	}
	printUpgradeResult(ciliumResult)

	argoResult, err := upgrade.ArgoCD(ctx, opts)
	if err != nil {
		return fmt.Errorf("upgrading argocd: %w", err)
	}
	printUpgradeResult(argoResult)

	return nil
}

func upgradeCiliumCmd() *cli.Command {
	return &cli.Command{
		Name:   "cilium",
		Usage:  "Upgrade Cilium CNI",
		Action: withSession(upgradeCiliumAction),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "skip-snapshot",
				Usage: "Skip pre-upgrade snapshot",
			},
		},
	}
}

func upgradeCiliumAction(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
	clusterName := cmd.String("cluster")

	cfg, err := infraconfig.Load(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("loading infrastructure config: %w", err)
	}

	apiServerIP, err := apiServerIPForCluster(clusterName)
	if err != nil {
		zapLogger.Warn("could not detect API server IP, using empty string", zap.Error(err))
	}

	opts := upgrade.Opts{
		ClusterName:  clusterName,
		CLIVersion:   version,
		SkipSnapshot: cmd.Bool("skip-snapshot"),
		Log:          zapLogger,
		InfraConfig:  cfg,
	}

	result, err := upgrade.Cilium(ctx, opts, apiServerIP)
	if err != nil {
		return fmt.Errorf("upgrading cilium: %w", err)
	}
	printUpgradeResult(result)
	return nil
}

func upgradeArgoCDCmd() *cli.Command {
	return &cli.Command{
		Name:   "argocd",
		Usage:  "Upgrade ArgoCD",
		Action: withSession(upgradeArgoCDAction),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "skip-snapshot",
				Usage: "Skip pre-upgrade snapshot",
			},
		},
	}
}

func upgradeArgoCDAction(ctx context.Context, cmd *cli.Command, sess *session.Session) error {
	clusterName := cmd.String("cluster")

	cfg, err := infraconfig.Load(sess.GitOpsPath)
	if err != nil {
		return fmt.Errorf("loading infrastructure config: %w", err)
	}

	opts := upgrade.Opts{
		ClusterName:  clusterName,
		CLIVersion:   version,
		SkipSnapshot: cmd.Bool("skip-snapshot"),
		Log:          zapLogger,
		InfraConfig:  cfg,
	}

	result, err := upgrade.ArgoCD(ctx, opts)
	if err != nil {
		return fmt.Errorf("upgrading argocd: %w", err)
	}
	printUpgradeResult(result)
	return nil
}

// apiServerIPForCluster extracts the API server IP from the kubeconfig
// REST config for the given cluster.
func apiServerIPForCluster(clusterName string) (string, error) {
	restCfg, err := kube.RESTConfigForCluster(clusterName)
	if err != nil {
		return "", fmt.Errorf("getting REST config: %w", err)
	}

	u, err := url.Parse(restCfg.Host)
	if err != nil {
		return "", fmt.Errorf("parsing API server URL %q: %w", restCfg.Host, err)
	}

	return u.Hostname(), nil
}

func printUpgradeResult(r *upgrade.Result) {
	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	if r.Skipped {
		fmt.Fprintf(os.Stderr, "%s: %s (%s)\n",
			r.Component, r.OldVersion, yellow(r.SkipReason))
		return
	}

	msg := fmt.Sprintf("%s: %s -> %s", r.Component, r.OldVersion, green(r.NewVersion))
	if r.SnapshotName != "" {
		msg += fmt.Sprintf(" (snapshot: %s)", r.SnapshotName)
	}
	fmt.Fprintln(os.Stderr, msg)
}
