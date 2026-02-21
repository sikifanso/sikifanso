package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/alicanalbayrak/sikifanso/internal/session"
	"github.com/fatih/color"
)

// stateString returns the cluster state with color applied.
func stateString(state string) string {
	switch state {
	case "running":
		return color.GreenString(state)
	case "stopped":
		return color.RedString(state)
	default:
		return color.YellowString("unknown")
	}
}

// printClusterInfo prints a formatted info box with all cluster service details.
func printClusterInfo(sess *session.Session) {
	lines := []string{
		fmt.Sprintf("State:           %s", stateString(sess.State)),
		"",
		fmt.Sprintf("ArgoCD URL:      %s", sess.Services.ArgoCD.URL),
		fmt.Sprintf("ArgoCD User:     %s", sess.Services.ArgoCD.Username),
		fmt.Sprintf("ArgoCD Password: %s", sess.Services.ArgoCD.Password),
		"",
		fmt.Sprintf("Hubble URL:      %s", sess.Services.Hubble.URL),
		"",
		fmt.Sprintf("GitOps Path:     %s", sess.GitOpsPath),
	}

	title := fmt.Sprintf("Cluster: %s", sess.ClusterName)
	footer := "Run: sikifanso cluster info"

	// Determine box width from widest content line.
	width := len(title)
	if len(footer) > width {
		width = len(footer)
	}
	for _, l := range lines {
		if len(l) > width {
			width = len(l)
		}
	}
	width += 4 // 2-char padding on each side

	hBar := strings.Repeat("─", width)

	// Center-align title and footer.
	titlePad := width - len(title)
	tLeft := titlePad / 2
	tRight := titlePad - tLeft

	footerPad := width - len(footer)
	fLeft := footerPad / 2
	fRight := footerPad - fLeft

	fmt.Fprintf(os.Stderr, "\n╭%s╮\n", hBar)
	fmt.Fprintf(os.Stderr, "│%s%s%s│\n", strings.Repeat(" ", tLeft), title, strings.Repeat(" ", tRight))
	fmt.Fprintf(os.Stderr, "├%s┤\n", hBar)

	for _, l := range lines {
		if l == "" {
			fmt.Fprintf(os.Stderr, "│%s│\n", strings.Repeat(" ", width))
		} else {
			fmt.Fprintf(os.Stderr, "│  %-*s  │\n", width-4, l)
		}
	}

	fmt.Fprintf(os.Stderr, "├%s┤\n", hBar)
	fmt.Fprintf(os.Stderr, "│%s%s%s│\n", strings.Repeat(" ", fLeft), footer, strings.Repeat(" ", fRight))
	fmt.Fprintf(os.Stderr, "╰%s╯\n\n", hBar)
}
