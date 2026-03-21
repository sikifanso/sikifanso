package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/cluster"
	"github.com/alicanalbayrak/sikifanso/internal/gitops"
	"github.com/alicanalbayrak/sikifanso/internal/preflight"
	"github.com/alicanalbayrak/sikifanso/internal/profile"
	"github.com/alicanalbayrak/sikifanso/internal/prompt"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func clusterCreateCmd() *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Create a new k3d cluster",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Cluster name",
				Value: defaultClusterName,
			},
			&cli.StringFlag{
				Name:  "bootstrap",
				Usage: "Bootstrap template repo URL",
				Value: gitops.DefaultBootstrapURL,
			},
			&cli.StringFlag{
				Name:  "bootstrap-version",
				Usage: "Bootstrap repo tag to clone (default: match CLI version; empty string forces HEAD)",
			},
			&cli.StringFlag{
				Name:  "profile",
				Usage: "Enable a predefined set of catalog apps (e.g. agent-dev, agent-safe, rag; comma-separated for composition)",
			},
		},
		Action: clusterCreateAction,
	}
}

func clusterCreateAction(ctx context.Context, cmd *cli.Command) error {
	name := cmd.String("name")
	if !cmd.IsSet("name") {
		name = prompt.String("Cluster name", defaultClusterName)
	}

	bootstrap := cmd.String("bootstrap")
	if !cmd.IsSet("bootstrap") {
		bootstrap = prompt.String("Bootstrap repo", gitops.DefaultBootstrapURL)
	}

	isDefaultBootstrap := bootstrap == gitops.DefaultBootstrapURL
	bootstrapVersion := resolveBootstrapVersion(
		version,
		isDefaultBootstrap,
		cmd.String("bootstrap-version"),
		cmd.IsSet("bootstrap-version"),
	)

	// Validate profile before creating the cluster — fail fast on typos.
	profileStr := cmd.String("profile")
	var profileApps []string
	if profileStr != "" {
		var err error
		profileApps, err = profile.Resolve(profileStr)
		if err != nil {
			return err
		}
	}

	zapLogger.Info("running preflight checks")
	if err := preflight.CheckDocker(ctx); err != nil {
		zapLogger.Error("preflight check failed", zap.Error(err))
		return err
	}
	zapLogger.Info("all preflight checks passed")

	sess, err := cluster.Create(ctx, zapLogger, name, cluster.Options{
		BootstrapURL:     bootstrap,
		BootstrapVersion: bootstrapVersion,
	})
	if err != nil {
		zapLogger.Error("cluster creation failed", zap.Error(err))
		return err
	}

	// Apply profile after cluster creation — enables catalog apps and commits.
	if len(profileApps) > 0 {
		zapLogger.Info("applying profile", zap.String("profile", profileStr), zap.Strings("apps", profileApps))
		if err := profile.Apply(sess.GitOpsPath, profileStr, profileApps, func(msg string) {
			zapLogger.Warn(msg)
		}); err != nil {
			return fmt.Errorf("applying profile: %w", err)
		}
		// Trigger ArgoCD sync so enabled apps deploy immediately.
		argocd.SyncAndReport(ctx, zapLogger, os.Stderr, syncOptsFromSession(sess))
	}

	printClusterInfo(sess)
	return nil
}

// resolveBootstrapVersion returns the bootstrap tag to pin, or "" for HEAD.
//
// Resolution order:
//  1. --bootstrap-version explicitly set → use that value (empty string forces HEAD)
//  2. Custom --bootstrap URL → "" (HEAD; custom repos may not share our tags)
//  3. Dev build ("dev") or pre-release/snapshot (contains "-", e.g. "0.4.1-next",
//     "v0.5.0-rc1" from goreleaser snapshot template) → "" (HEAD)
//  4. Release build → CLI version (e.g. "v0.5.0"), which must match a bootstrap repo tag
func resolveBootstrapVersion(cliVersion string, isDefaultBootstrap bool, explicitVersion string, versionSet bool) string {
	if versionSet {
		return explicitVersion
	}
	if !isDefaultBootstrap {
		return ""
	}
	if cliVersion == "dev" || strings.Contains(cliVersion, "-") {
		return ""
	}
	return cliVersion
}
