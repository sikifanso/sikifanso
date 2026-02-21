package cluster

import (
	"fmt"
	"net"
)

// HostPorts holds the host-side port assignments for a k3d cluster.
// Container-internal ports (NodePorts) stay fixed; only host ports vary.
type HostPorts struct {
	APIServer int // default 6443 → container 6443
	HTTP      int // default 8080 → container 30082
	HTTPS     int // default 8443 → container 30083
	ArgoCDUI  int // default 30080 → container 30080
	HubbleUI  int // default 30081 → container 30081
}

var defaultPorts = HostPorts{
	APIServer: 6443,
	HTTP:      8080,
	HTTPS:     8443,
	ArgoCDUI:  30080,
	HubbleUI:  30081,
}

// resolveHostPorts returns host ports for a new cluster.
// It tries the defaults first (best UX for the single-cluster case).
// If any default port is taken, it allocates all five from the OS.
func resolveHostPorts() (HostPorts, error) {
	defaults := []int{
		defaultPorts.APIServer,
		defaultPorts.HTTP,
		defaultPorts.HTTPS,
		defaultPorts.ArgoCDUI,
		defaultPorts.HubbleUI,
	}

	if allAvailable(defaults) {
		return defaultPorts, nil
	}

	ports, err := findFreePorts(5)
	if err != nil {
		return HostPorts{}, fmt.Errorf("allocating free ports: %w", err)
	}

	return HostPorts{
		APIServer: ports[0],
		HTTP:      ports[1],
		HTTPS:     ports[2],
		ArgoCDUI:  ports[3],
		HubbleUI:  ports[4],
	}, nil
}

// allAvailable returns true if every port can be bound on localhost.
func allAvailable(ports []int) bool {
	for _, p := range ports {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			return false
		}
		ln.Close()
	}
	return true
}

// findFreePorts asks the OS for n ephemeral ports.
// It holds all listeners open until addresses are captured to avoid duplicates.
func findFreePorts(n int) ([]int, error) {
	listeners := make([]net.Listener, 0, n)
	ports := make([]int, 0, n)

	for i := 0; i < n; i++ {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			// Close any already-opened listeners before returning.
			for _, l := range listeners {
				l.Close()
			}
			return nil, err
		}
		listeners = append(listeners, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}

	for _, ln := range listeners {
		ln.Close()
	}

	return ports, nil
}
