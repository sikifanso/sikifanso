package argocd

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"
)

func fakeApp(name, syncStatus, healthStatus string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": syncStatus,
				},
				"health": map[string]interface{}{
					"status": healthStatus,
				},
			},
		},
	}
}

func TestGetAppStatus_SyncedHealthy(t *testing.T) {
	scheme := runtime.NewScheme()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			applicationGVR: "ApplicationList",
		},
		fakeApp("cilium", "Synced", "Healthy"),
	)

	synced, healthy, err := getAppStatus(context.Background(), dc, "argocd", "cilium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !synced || !healthy {
		t.Fatalf("expected synced=true healthy=true, got synced=%v healthy=%v", synced, healthy)
	}
}

func TestGetAppStatus_Progressing(t *testing.T) {
	scheme := runtime.NewScheme()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			applicationGVR: "ApplicationList",
		},
		fakeApp("cilium", "OutOfSync", "Progressing"),
	)

	synced, healthy, err := getAppStatus(context.Background(), dc, "argocd", "cilium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if synced || healthy {
		t.Fatalf("expected synced=false healthy=false, got synced=%v healthy=%v", synced, healthy)
	}
}

func TestGetAppStatus_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	dc := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{
			applicationGVR: "ApplicationList",
		},
	)

	_, _, err := getAppStatus(context.Background(), dc, "argocd", "missing")
	if err == nil {
		t.Fatal("expected error for missing app")
	}
}

func TestWaitForApplicationsHealthy_AlreadyHealthy(t *testing.T) {
	origTimeout := appHealthTimeout
	origInterval := appHealthInterval
	appHealthTimeout = 5 * time.Second
	appHealthInterval = 100 * time.Millisecond
	defer func() {
		appHealthTimeout = origTimeout
		appHealthInterval = origInterval
	}()

	// We cannot use WaitForApplicationsHealthy directly because it creates
	// a dynamic client from rest.Config. Instead, test the polling logic
	// via a local HTTP test server that serves the K8s API.
	// For unit testing, we validate getAppStatus above and do an integration-style
	// test here with a fake REST server.

	// Start a minimal fake API server that returns healthy apps.
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/argoproj.io/v1alpha1/namespaces/argocd/applications/cilium", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind": "Application",
			"metadata": {"name": "cilium", "namespace": "argocd"},
			"status": {"sync": {"status": "Synced"}, "health": {"status": "Healthy"}}
		}`))
	})
	mux.HandleFunc("/apis/argoproj.io/v1alpha1/namespaces/argocd/applications/argo-cd", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind": "Application",
			"metadata": {"name": "argo-cd", "namespace": "argocd"},
			"status": {"sync": {"status": "Synced"}, "health": {"status": "Healthy"}}
		}`))
	})

	srv := &http.Server{Addr: "127.0.0.1:0", Handler: mux}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	restCfg := &rest.Config{Host: "http://" + ln.Addr().String()}
	log := zaptest.NewLogger(t)

	err = WaitForApplicationsHealthy(context.Background(), log, restCfg, "argocd", []string{"cilium", "argo-cd"})
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
}

func TestWaitForApplicationsHealthy_Timeout(t *testing.T) {
	origTimeout := appHealthTimeout
	origInterval := appHealthInterval
	appHealthTimeout = 2 * time.Second
	appHealthInterval = 100 * time.Millisecond
	defer func() {
		appHealthTimeout = origTimeout
		appHealthInterval = origInterval
	}()

	// Server always returns Progressing.
	mux := http.NewServeMux()
	mux.HandleFunc("/apis/argoproj.io/v1alpha1/namespaces/argocd/applications/cilium", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind": "Application",
			"metadata": {"name": "cilium", "namespace": "argocd"},
			"status": {"sync": {"status": "OutOfSync"}, "health": {"status": "Progressing"}}
		}`))
	})

	srv := &http.Server{Addr: "127.0.0.1:0", Handler: mux}
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	restCfg := &rest.Config{Host: "http://" + ln.Addr().String()}
	log := zaptest.NewLogger(t)

	err = WaitForApplicationsHealthy(context.Background(), log, restCfg, "argocd", []string{"cilium"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
