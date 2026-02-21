package argocd

// Values returns ArgoCD Helm chart values matching the homelab reference
// configuration (bootstrap/argocd/values.yaml). Unlike Cilium, ArgoCD has
// no cluster-specific dynamic values.
func Values() map[string]interface{} {
	return map[string]interface{}{
		// Server: insecure mode, NodePort access on 30080.
		"server": map[string]interface{}{
			"extraArgs": []string{
				"--insecure",
				"--repo-server-plaintext",
			},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "50m", "memory": "128Mi"},
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
			},
			"ingress": map[string]interface{}{"enabled": false},
			"service": map[string]interface{}{
				"type":         "NodePort",
				"nodePortHttp": 30080,
			},
		},

		// Repo server — with hostPath volume for local gitops repo.
		"repoServer": map[string]interface{}{
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "50m", "memory": "128Mi"},
				"limits":   map[string]interface{}{"cpu": "500m", "memory": "512Mi"},
			},
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "gitops",
					"hostPath": map[string]interface{}{
						"path": "/local-gitops",
						"type": "Directory",
					},
				},
			},
			"volumeMounts": []interface{}{
				map[string]interface{}{
					"name":      "gitops",
					"mountPath": "/local-gitops",
					"readOnly":  true,
				},
			},
		},

		// Application controller.
		"controller": map[string]interface{}{
			"extraArgs": []string{"--repo-server-plaintext"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "100m", "memory": "256Mi"},
				"limits":   map[string]interface{}{"cpu": "1000m", "memory": "1Gi"},
			},
			"metrics": map[string]interface{}{
				"enabled": true,
				"serviceMonitor": map[string]interface{}{
					"enabled": false,
				},
			},
		},

		// Redis.
		"redis": map[string]interface{}{
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "50m", "memory": "64Mi"},
				"limits":   map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
			},
		},

		// Dex disabled.
		"dex": map[string]interface{}{"enabled": false},

		// Notifications controller.
		"notifications": map[string]interface{}{
			"enabled": true,
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "25m", "memory": "64Mi"},
				"limits":   map[string]interface{}{"cpu": "100m", "memory": "128Mi"},
			},
		},

		// ApplicationSet controller.
		"applicationSet": map[string]interface{}{
			"enabled":   true,
			"extraArgs": []string{"--repo-server-plaintext"},
			"resources": map[string]interface{}{
				"requests": map[string]interface{}{"cpu": "25m", "memory": "64Mi"},
				"limits":   map[string]interface{}{"cpu": "200m", "memory": "256Mi"},
			},
		},

		// Configs: CM, params, secret, RBAC, repositories.
		"configs": map[string]interface{}{
			"repositories": map[string]interface{}{
				"local-gitops": map[string]interface{}{
					"type": "git",
					"url":  "/local-gitops",
				},
			},
			"cm": map[string]interface{}{
				"admin.enabled":                          "true",
				"timeout.reconciliation":                 "180s",
				"statusbadge.enabled":                    "true",
				"application.resourceTrackingMethod":     "annotation",
				"resource.customizations.health.networking.k8s.io_Ingress": `hs = {}
hs.status = "Healthy"
hs.message = "Ingress is ready"
if obj.status ~= nil and obj.status.loadBalancer ~= nil then
  if obj.status.loadBalancer.ingress ~= nil then
    hs.message = "Ingress has address assigned"
  end
end
return hs
`,
			},
			"params": map[string]interface{}{
				"server.insecure":                true,
				"reposerver.disable.tls":         true,
				"controller.repo.server.plaintext": true,
			},
			"secret": map[string]interface{}{
				"createSecret": true,
			},
			"rbac": map[string]interface{}{
				"policy.default": "role:readonly",
				"policy.csv":     "g, admin, role:admin\n",
			},
		},
	}
}
