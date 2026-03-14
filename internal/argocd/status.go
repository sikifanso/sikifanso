package argocd

import (
	"context"
	"fmt"
	"io"

	"github.com/fatih/color"
	"go.uber.org/zap"
)

// SyncAndReport triggers a webhook sync and then polls ArgoCD for app status,
// printing a status table to w. If the REST client cannot be created (e.g. auth
// failure), it falls back to fire-and-forget sync with a warning.
func SyncAndReport(ctx context.Context, log *zap.Logger, w io.Writer, opts SyncOpts) {
	if err := Sync(ctx, log, opts.ClusterName, opts.ArgoURL); err != nil {
		log.Warn("argocd sync failed", zap.Error(err))
		fmt.Fprintln(w, "ArgoCD unreachable — will reconcile on next cluster start")
		return
	}

	ReportStatus(ctx, log, w, opts)
}

// ReportStatus polls ArgoCD for current app status and prints a status table.
// Unlike SyncAndReport, it does NOT trigger a sync — use this when a sync has
// already been performed (e.g. via SyncWithOpts).
func ReportStatus(ctx context.Context, log *zap.Logger, w io.Writer, opts SyncOpts) {
	client, err := NewClient(ctx, opts.ArgoURL, opts.Username, opts.Password)
	if err != nil {
		log.Debug("could not create ArgoCD REST client for status polling", zap.Error(err))
		fmt.Fprintln(w, "Status polling unavailable")
		return
	}

	apps, err := client.ListApplications(ctx)
	if err != nil {
		log.Debug("could not list applications for status", zap.Error(err))
		return
	}

	if len(apps) == 0 {
		fmt.Fprintln(w, "No ArgoCD applications found")
		return
	}

	printAppStatusTable(w, apps)
}

func printAppStatusTable(w io.Writer, apps []AppStatus) {
	nameW := len("NAME")
	syncW := len("SYNC")
	healthW := len("HEALTH")
	for _, a := range apps {
		if len(a.Name) > nameW {
			nameW = len(a.Name)
		}
		if len(a.SyncStatus) > syncW {
			syncW = len(a.SyncStatus)
		}
		if len(a.Health) > healthW {
			healthW = len(a.Health)
		}
	}

	fmt.Fprintf(w, "%-*s  %-*s  %-*s  %s\n", nameW, "NAME", syncW, "SYNC", healthW, "HEALTH", "")
	for _, a := range apps {
		indicator := color.GreenString("✓")
		if a.Health == HealthDegraded || a.Health == HealthMissing {
			indicator = color.RedString("✗")
		} else if a.SyncStatus != SyncStatusSynced || a.Health != HealthHealthy {
			indicator = color.YellowString("~")
		}

		fmt.Fprintf(w, "%-*s  %s  %s  %s\n", nameW, a.Name,
			colorizeStatus(a.SyncStatus, syncW),
			colorizeStatus(a.Health, healthW),
			indicator)
	}
}

// colorizeStatus pads the status value to width and applies color based on
// the well-known ArgoCD status values. Padding is applied before colorization
// so ANSI escape codes don't break column alignment.
func colorizeStatus(value string, width int) string {
	padded := fmt.Sprintf("%-*s", width, value)
	switch value {
	case SyncStatusSynced, HealthHealthy:
		return color.GreenString(padded)
	case SyncStatusOutOfSync, HealthProgressing, HealthSuspended, HealthMissing:
		return color.YellowString(padded)
	case HealthDegraded, HealthUnknown:
		return color.RedString(padded)
	default:
		return padded
	}
}
