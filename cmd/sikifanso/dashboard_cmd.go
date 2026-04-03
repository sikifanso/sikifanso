package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/alicanalbayrak/sikifanso/internal/dashboard"
	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

func clusterDashboardCmd() *cli.Command {
	return &cli.Command{
		Name:  "dashboard",
		Usage: "Start the local web dashboard",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "addr",
				Usage: "Listen address",
				Value: ":9090",
			},
			&cli.BoolFlag{
				Name:  "no-browser",
				Usage: "Don't open browser automatically",
			},
		},
		Action: dashboardAction,
	}
}

func dashboardAction(ctx context.Context, cmd *cli.Command) error {
	clusterName := cmd.String("cluster")
	_, err := session.Load(clusterName)
	if err != nil {
		return fmt.Errorf("loading session for cluster %q: %w", clusterName, err)
	}

	addr := cmd.String("addr")
	srv := dashboard.NewServer(dashboard.ServerOpts{
		Addr:        addr,
		ClusterName: clusterName,
		Log:         zapLogger,
	})

	// Determine the URL for display and browser.
	displayAddr := addr
	if displayAddr[0] == ':' {
		displayAddr = "localhost" + displayAddr
	}
	url := "http://" + displayAddr

	// Start server.
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zapLogger.Error("dashboard server error", zap.Error(err))
		}
	}()

	fmt.Fprintf(os.Stderr, "Dashboard running at %s\n", color.CyanString(url))
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop\n")

	// Open browser.
	if !cmd.Bool("no-browser") {
		openBrowser(url)
	}

	// Wait for interrupt.
	sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-sigCtx.Done()

	fmt.Fprintln(os.Stderr, "\nShutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func openBrowser(url string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "linux":
		c = exec.Command("xdg-open", url)
	default:
		return
	}
	_ = c.Start()
}
