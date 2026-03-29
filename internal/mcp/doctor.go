package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/argocd"
	"github.com/alicanalbayrak/sikifanso/internal/argocd/grpcclient"
	"github.com/alicanalbayrak/sikifanso/internal/doctor"
	"github.com/alicanalbayrak/sikifanso/internal/infraconfig"
	"github.com/alicanalbayrak/sikifanso/internal/kube"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type doctorInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
}

type argocdAppsInput struct {
	Cluster string `json:"cluster" jsonschema:"Name of the cluster"`
}

func registerDoctorTools(s *mcp.Server, _ *Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "doctor",
		Description: "Run health checks on the cluster (Docker, nodes, Cilium, ArgoCD, apps, agents)",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input doctorInput) (*mcp.CallToolResult, any, error) {
		checks := doctor.InfraChecks()

		sess, err := session.Load(input.Cluster)
		if err != nil {
			results := doctor.Run(ctx, checks)
			results = append(results, doctor.Result{
				Name:  "k3d cluster",
				OK:    false,
				Cause: fmt.Sprintf("no session found for cluster %q", input.Cluster),
				Fix:   "sikifanso cluster create",
			})
			return textResult(formatDoctorResults(results))
		}

		// Build both typed and dynamic clients from a single REST config parse.
		restCfg, err := kube.RESTConfigForCluster(input.Cluster)
		if err != nil {
			results := doctor.Run(ctx, checks)
			results = append(results, doctor.Result{
				Name:  "k3d cluster",
				OK:    false,
				Cause: fmt.Sprintf("cannot connect to cluster: %v", err),
				Fix:   "sikifanso cluster start",
			})
			return textResult(formatDoctorResults(results))
		}

		cfg, cfgErr := infraconfig.Load(sess.GitOpsPath)
		if cfgErr != nil {
			cfg = infraconfig.Defaults()
		}

		cs, err := kubernetes.NewForConfig(restCfg)
		if err == nil {
			checks = append(checks, doctor.ClusterChecks(cs, cfg)...)
		}

		dynClient, err := dynamic.NewForConfig(restCfg)
		if err == nil {
			var grpcClient *grpcclient.Client
			if sess.Services.ArgoCD.GRPCAddress != "" {
				grpcClient, _ = grpcclient.NewClient(ctx, grpcclient.Options{
					Address:  sess.Services.ArgoCD.GRPCAddress,
					Username: sess.Services.ArgoCD.Username,
					Password: sess.Services.ArgoCD.Password,
				})
			}
			checks = append(checks, doctor.AppChecks(dynClient, sess.GitOpsPath, cfg, grpcClient)...)
		}

		results := doctor.Run(ctx, checks)
		return textResult(formatDoctorResults(results))
	})

	mcp.AddTool(s, &mcp.Tool{
		Name:        "argocd_apps",
		Description: "List ArgoCD applications with their sync and health status",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input argocdAppsInput) (*mcp.CallToolResult, any, error) {
		sess, r, sv, e := loadSession(input.Cluster)
		if sess == nil {
			return r, sv, e
		}

		client, err := argocd.NewClient(ctx,
			sess.Services.ArgoCD.URL,
			sess.Services.ArgoCD.Username,
			sess.Services.ArgoCD.Password,
		)
		if err != nil {
			return errResult(fmt.Errorf("connecting to ArgoCD: %w", err))
		}

		apps, err := client.ListApplications(ctx)
		if err != nil {
			return errResult(fmt.Errorf("listing applications: %w", err))
		}
		if len(apps) == 0 {
			return textResult("No ArgoCD applications found.")
		}

		var sb strings.Builder
		sb.WriteString("ArgoCD Applications:\n")
		for _, app := range apps {
			fmt.Fprintf(&sb, "  - %-25s sync:%-10s health:%s\n",
				app.Name, app.SyncStatus, app.Health)
		}
		return textResult(sb.String())
	})
}

func formatDoctorResults(results []doctor.Result) string {
	var sb strings.Builder
	sb.WriteString("Health Check Results:\n")
	for _, r := range results {
		if r.OK {
			fmt.Fprintf(&sb, "  [OK] %-20s %s\n", r.Name, r.Message)
		} else {
			msg := r.Cause
			if r.Message != "" {
				msg = r.Message
			}
			fmt.Fprintf(&sb, "  [!!] %-20s %s\n", r.Name, msg)
			if r.Cause != "" && r.Message != "" {
				fmt.Fprintf(&sb, "       -> %s\n", r.Cause)
			}
			if r.Fix != "" {
				fmt.Fprintf(&sb, "       -> Try: %s\n", r.Fix)
			}
		}
	}
	return sb.String()
}
