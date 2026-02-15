package cilium

// Values returns Cilium Helm chart values matching the homelab reference
// configuration. The apiServerIP is the Docker-internal IP of the k3d
// server-0 container so Cilium agents can reach the API server after
// kube-proxy is replaced.
func Values(apiServerIP string) map[string]interface{} {
	return map[string]interface{}{
		// Cluster connectivity — use the container IP, not localhost.
		"k8sServiceHost": apiServerIP,
		"k8sServicePort": 6443,

		// Full kube-proxy replacement with eBPF.
		"kubeProxyReplacement": true,

		// Ingress controller: Cilium replaces Traefik.
		"ingressController": map[string]interface{}{
			"enabled":          true,
			"default":          true,
			"loadbalancerMode": "shared",
			"service": map[string]interface{}{
				"type":             "NodePort",
				"insecureNodePort": 30082,
				"secureNodePort":   30083,
			},
		},

		// Hubble observability.
		"hubble": map[string]interface{}{
			"enabled": true,
			"relay": map[string]interface{}{
				"enabled": true,
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{"cpu": "50m", "memory": "64Mi"},
					"limits":   map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
				},
			},
			"ui": map[string]interface{}{
				"enabled": true,
				"ingress": map[string]interface{}{"enabled": false},
				"service": map[string]interface{}{
					"type":     "NodePort",
					"nodePort": 30081,
				},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{"cpu": "50m", "memory": "64Mi"},
					"limits":   map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
				},
			},
		},

		// Operator.
		"operator": map[string]interface{}{
			"replicas": 1,
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "50m", "memory": "128Mi"},
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "256Mi"},
			},
		},

		// Agent resources.
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{"cpu": "100m", "memory": "256Mi"},
			"limits":   map[string]interface{}{"cpu": "1000m", "memory": "1Gi"},
		},

		// IPAM.
		"ipam": map[string]interface{}{"mode": "kubernetes"},

		// eBPF datapath.
		"bpf": map[string]interface{}{
			"masquerade":        true,
			"hostLegacyRouting": false,
		},

		// Tunnel mode works well inside Docker.
		"routingMode":    "tunnel",
		"tunnelProtocol": "vxlan",

		// Network policy.
		"policyEnforcementMode": "default",

		// Debug off for normal use.
		"debug": map[string]interface{}{"enabled": false},
	}
}
