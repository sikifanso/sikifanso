package cluster

import (
	"fmt"
	"net"
	"testing"
)

func TestFindFreePorts(t *testing.T) {
	t.Parallel()
	ports, err := findFreePorts(5)
	if err != nil {
		t.Fatalf("findFreePorts(5): %v", err)
	}
	if len(ports) != 5 {
		t.Fatalf("got %d ports, want 5", len(ports))
	}

	seen := map[int]bool{}
	for _, p := range ports {
		if p <= 0 {
			t.Errorf("port %d is not positive", p)
		}
		if seen[p] {
			t.Errorf("duplicate port %d", p)
		}
		seen[p] = true
	}
}

func TestFindFreePortsZero(t *testing.T) {
	t.Parallel()
	ports, err := findFreePorts(0)
	if err != nil {
		t.Fatalf("findFreePorts(0): %v", err)
	}
	if len(ports) != 0 {
		t.Errorf("got %d ports, want 0", len(ports))
	}
}

func TestAllAvailableWithFreePorts(t *testing.T) {
	t.Parallel()
	ports, err := findFreePorts(3)
	if err != nil {
		t.Fatalf("findFreePorts: %v", err)
	}
	// Ephemeral ports we just released should be available.
	if !allAvailable(ports) {
		t.Errorf("allAvailable(%v) = false, want true", ports)
	}
}

func TestAllAvailableWithBoundPort(t *testing.T) {
	t.Parallel()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	bound := ln.Addr().(*net.TCPAddr).Port

	// A free port to mix in.
	free, err := findFreePorts(1)
	if err != nil {
		t.Fatalf("findFreePorts: %v", err)
	}

	if allAvailable([]int{free[0], bound}) {
		t.Errorf("allAvailable([%d, %d]) = true, want false (port %d is bound)",
			free[0], bound, bound)
	}

	// Sanity: single bound port.
	if allAvailable([]int{bound}) {
		t.Errorf("allAvailable([%d]) = true, want false", bound)
	}

	// Sanity: single free port.
	if !allAvailable([]int{free[0]}) {
		t.Errorf("allAvailable([%d]) = false, want true", free[0])
	}

	// Also test through a direct listen to be extra sure.
	_, listenErr := net.Listen("tcp", fmt.Sprintf(":%d", bound))
	if listenErr == nil {
		t.Errorf("expected error binding already-bound port %d", bound)
	}
}

func TestDefaultPortsIncludeArgoCDGRPC(t *testing.T) {
	t.Parallel()
	if defaultPorts.ArgoCDGRPC == 0 {
		t.Fatal("defaultPorts.ArgoCDGRPC is zero; expected a non-zero port")
	}
	if defaultPorts.ArgoCDGRPC == defaultPorts.ArgoCDUI {
		t.Errorf("ArgoCDGRPC (%d) must differ from ArgoCDUI (%d)", defaultPorts.ArgoCDGRPC, defaultPorts.ArgoCDUI)
	}
	if defaultPorts.ArgoCDGRPC == defaultPorts.HubbleUI {
		t.Errorf("ArgoCDGRPC (%d) must differ from HubbleUI (%d)", defaultPorts.ArgoCDGRPC, defaultPorts.HubbleUI)
	}
}
