package doctor

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// mockCheck is a simple Check implementation that returns predetermined results.
type mockCheck struct {
	results []Result
}

func (m mockCheck) Run(_ context.Context) []Result {
	return m.results
}

func TestRun_AllOK(t *testing.T) {
	checks := []Check{
		mockCheck{results: []Result{{Name: "a", OK: true, Message: "good"}}},
		mockCheck{results: []Result{{Name: "b", OK: true, Message: "fine"}}},
	}

	results := Run(context.Background(), checks)
	if len(results) != 2 {
		t.Fatalf("Run returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if !r.OK {
			t.Errorf("result %q: OK = false, want true", r.Name)
		}
	}
}

func TestRun_AllFailed(t *testing.T) {
	checks := []Check{
		mockCheck{results: []Result{{Name: "a", OK: false, Cause: "broken"}}},
		mockCheck{results: []Result{{Name: "b", OK: false, Cause: "also broken"}}},
	}

	results := Run(context.Background(), checks)
	if len(results) != 2 {
		t.Fatalf("Run returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.OK {
			t.Errorf("result %q: OK = true, want false", r.Name)
		}
	}
}

func TestRun_Mixed(t *testing.T) {
	checks := []Check{
		mockCheck{results: []Result{{Name: "ok-check", OK: true}}},
		mockCheck{results: []Result{{Name: "fail-check", OK: false, Cause: "oops"}}},
	}

	results := Run(context.Background(), checks)
	if len(results) != 2 {
		t.Fatalf("Run returned %d results, want 2", len(results))
	}
	if !results[0].OK {
		t.Errorf("results[0] %q: OK = false, want true", results[0].Name)
	}
	if results[1].OK {
		t.Errorf("results[1] %q: OK = true, want false", results[1].Name)
	}
}

func TestRun_MultiResultCheck(t *testing.T) {
	checks := []Check{
		mockCheck{results: []Result{
			{Name: "app-1", OK: true},
			{Name: "app-2", OK: false, Cause: "degraded"},
		}},
		mockCheck{results: []Result{{Name: "infra", OK: true}}},
	}

	results := Run(context.Background(), checks)
	if len(results) != 3 {
		t.Fatalf("Run returned %d results, want 3", len(results))
	}
	if results[0].Name != "app-1" || results[1].Name != "app-2" || results[2].Name != "infra" {
		t.Errorf("unexpected result order: %v, %v, %v", results[0].Name, results[1].Name, results[2].Name)
	}
}

func TestRun_Empty(t *testing.T) {
	results := Run(context.Background(), nil)
	if len(results) != 0 {
		t.Fatalf("Run with nil checks returned %d results, want 0", len(results))
	}
}

func TestRun_CheckReturnsNil(t *testing.T) {
	checks := []Check{
		mockCheck{results: nil},
	}

	results := Run(context.Background(), checks)
	if len(results) != 0 {
		t.Fatalf("Run returned %d results, want 0", len(results))
	}
}

func TestDeploymentAvailable(t *testing.T) {
	tests := []struct {
		name       string
		conditions []appsv1.DeploymentCondition
		want       bool
	}{
		{
			name: "Available=True",
			conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
			want: true,
		},
		{
			name: "Available=False",
			conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse},
			},
			want: false,
		},
		{
			name:       "no conditions",
			conditions: nil,
			want:       false,
		},
		{
			name: "only Progressing condition",
			conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
			want: false,
		},
		{
			name: "multiple conditions with Available=True",
			conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deploy := &appsv1.Deployment{}
			deploy.Status.Conditions = tt.conditions

			got := deploymentAvailable(deploy)
			if got != tt.want {
				t.Errorf("deploymentAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractDegradedCause(t *testing.T) {
	tests := []struct {
		name   string
		status map[string]interface{}
		want   string
	}{
		{
			name: "degraded resource with message",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind":      "Deployment",
						"name":      "my-app",
						"namespace": "default",
						"health": map[string]interface{}{
							"status":  "Degraded",
							"message": "container crash-looping",
						},
					},
				},
			},
			want: "Deployment my-app in namespace default: container crash-looping",
		},
		{
			name: "degraded resource without message",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind":      "StatefulSet",
						"name":      "db",
						"namespace": "data",
						"health": map[string]interface{}{
							"status": "Missing",
						},
					},
				},
			},
			want: "StatefulSet db in namespace data",
		},
		{
			name:   "no resources field",
			status: map[string]interface{}{},
			want:   "",
		},
		{
			name: "all healthy resources",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind":      "Deployment",
						"name":      "healthy-app",
						"namespace": "default",
						"health": map[string]interface{}{
							"status": "Healthy",
						},
					},
				},
			},
			want: "",
		},
		{
			name: "resources with no health field",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind": "ConfigMap",
						"name": "my-config",
					},
				},
			},
			want: "",
		},
		{
			name: "empty resources slice",
			status: map[string]interface{}{
				"resources": []interface{}{},
			},
			want: "",
		},
		{
			name: "mixed healthy and degraded",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind":      "Service",
						"name":      "svc",
						"namespace": "default",
						"health": map[string]interface{}{
							"status": "Healthy",
						},
					},
					map[string]interface{}{
						"kind":      "Deployment",
						"name":      "broken",
						"namespace": "default",
						"health": map[string]interface{}{
							"status":  "Degraded",
							"message": "replicas unavailable",
						},
					},
				},
			},
			want: "Deployment broken in namespace default: replicas unavailable",
		},
		{
			name: "empty health status string is treated as healthy",
			status: map[string]interface{}{
				"resources": []interface{}{
					map[string]interface{}{
						"kind":      "Pod",
						"name":      "p",
						"namespace": "ns",
						"health": map[string]interface{}{
							"status": "",
						},
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDegradedCause(tt.status)
			if got != tt.want {
				t.Errorf("extractDegradedCause() = %q, want %q", got, tt.want)
			}
		})
	}
}
