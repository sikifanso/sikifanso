package main

import (
	"slices"
	"testing"

	"github.com/urfave/cli/v3"
)

// collectCommandNames returns sorted command names, optionally filtering out hidden ones.
func collectCommandNames(cmds []*cli.Command, includeHidden bool) []string {
	names := make([]string, 0, len(cmds))
	for _, c := range cmds {
		if includeHidden || !c.Hidden {
			names = append(names, c.Name)
		}
	}
	slices.Sort(names)
	return names
}

// findCommand returns the command with the given name, or nil.
func findCommand(cmds []*cli.Command, name string) *cli.Command {
	for _, c := range cmds {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestTopLevelVisibleCommands(t *testing.T) {
	app := newApp()
	got := collectCommandNames(app.Commands, false)
	want := []string{"agent", "app", "cluster", "snapshot"}

	if !slices.Equal(got, want) {
		t.Errorf("visible top-level commands = %v, want %v", got, want)
	}
}

func TestMCPIsHidden(t *testing.T) {
	app := newApp()
	mcp := findCommand(app.Commands, "mcp")
	if mcp == nil {
		t.Fatal("mcp command not found")
	}
	if !mcp.Hidden {
		t.Error("mcp command should be hidden")
	}
}

func TestOldCommandsAbsent(t *testing.T) {
	app := newApp()
	removed := []string{"argocd", "catalog", "profile", "status", "doctor", "dashboard", "upgrade", "restore"}
	all := collectCommandNames(app.Commands, true)

	for _, name := range removed {
		if slices.Contains(all, name) {
			t.Errorf("old command %q should not exist as a top-level command", name)
		}
	}
}

func TestClusterSubcommands(t *testing.T) {
	app := newApp()
	cluster := findCommand(app.Commands, "cluster")
	if cluster == nil {
		t.Fatal("cluster command not found")
	}

	got := collectCommandNames(cluster.Commands, false)
	want := []string{"create", "dashboard", "delete", "doctor", "info", "profiles", "start", "stop", "upgrade"}

	if !slices.Equal(got, want) {
		t.Errorf("cluster subcommands = %v, want %v", got, want)
	}
}

func TestAppSubcommands(t *testing.T) {
	app := newApp()
	appCmd := findCommand(app.Commands, "app")
	if appCmd == nil {
		t.Fatal("app command not found")
	}

	got := collectCommandNames(appCmd.Commands, false)
	want := []string{"add", "diff", "disable", "enable", "list", "logs", "remove", "rollback", "status", "sync"}

	if !slices.Equal(got, want) {
		t.Errorf("app subcommands = %v, want %v", got, want)
	}
}

func TestAgentSubcommands(t *testing.T) {
	app := newApp()
	agent := findCommand(app.Commands, "agent")
	if agent == nil {
		t.Fatal("agent command not found")
	}

	got := collectCommandNames(agent.Commands, false)
	want := []string{"create", "delete", "list"}

	if !slices.Equal(got, want) {
		t.Errorf("agent subcommands = %v, want %v", got, want)
	}
}

func TestSnapshotSubcommands(t *testing.T) {
	app := newApp()
	snapshot := findCommand(app.Commands, "snapshot")
	if snapshot == nil {
		t.Fatal("snapshot command not found")
	}

	got := collectCommandNames(snapshot.Commands, false)
	want := []string{"capture", "delete", "list", "restore"}

	if !slices.Equal(got, want) {
		t.Errorf("snapshot subcommands = %v, want %v", got, want)
	}
}
