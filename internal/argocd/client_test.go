package argocd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/session" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req sessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decoding request body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Username != "admin" || req.Password != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sessionResponse{Token: "test-token-123"})
	}))
	defer srv.Close()

	client, err := NewClient(context.Background(), srv.URL, "admin", "secret")
	if err != nil {
		t.Fatalf("NewClient() unexpected error: %v", err)
	}
	if client.token != "test-token-123" {
		t.Errorf("expected token %q, got %q", "test-token-123", client.token)
	}
}

func TestNewClient_AuthFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := NewClient(context.Background(), srv.URL, "admin", "wrong")
	if err == nil {
		t.Fatal("NewClient() expected error for bad credentials, got nil")
	}
}

// newTestClient creates a Client that is already authenticated and pointed at
// the given test server. This avoids repeating the auth handshake in every test.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	return &Client{
		baseURL:    srv.URL,
		token:      "test-token",
		httpClient: http.DefaultClient,
	}
}

func TestListApplications_Success(t *testing.T) {
	t.Parallel()

	payload := `{
		"items": [
			{
				"metadata": {"name": "app-one"},
				"status": {
					"sync": {"status": "Synced"},
					"health": {"status": "Healthy", "message": ""}
				}
			},
			{
				"metadata": {"name": "app-two"},
				"status": {
					"sync": {"status": "OutOfSync"},
					"health": {"status": "Degraded", "message": "container crashed"}
				}
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	apps, err := client.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() unexpected error: %v", err)
	}
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}

	if apps[0].Name != "app-one" {
		t.Errorf("expected name %q, got %q", "app-one", apps[0].Name)
	}
	if apps[0].SyncStatus != SyncStatusSynced {
		t.Errorf("expected sync status %q, got %q", SyncStatusSynced, apps[0].SyncStatus)
	}
	if apps[0].Health != HealthHealthy {
		t.Errorf("expected health %q, got %q", HealthHealthy, apps[0].Health)
	}

	if apps[1].Name != "app-two" {
		t.Errorf("expected name %q, got %q", "app-two", apps[1].Name)
	}
	if apps[1].SyncStatus != SyncStatusOutOfSync {
		t.Errorf("expected sync status %q, got %q", SyncStatusOutOfSync, apps[1].SyncStatus)
	}
	if apps[1].Health != HealthDegraded {
		t.Errorf("expected health %q, got %q", HealthDegraded, apps[1].Health)
	}
	if apps[1].Message != "container crashed" {
		t.Errorf("expected message %q, got %q", "container crashed", apps[1].Message)
	}
}

func TestListApplications_Empty(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items": []}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	apps, err := client.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() unexpected error: %v", err)
	}
	if len(apps) != 0 {
		t.Fatalf("expected 0 apps, got %d", len(apps))
	}
}

func TestListApplications_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.ListApplications(context.Background())
	if err == nil {
		t.Fatal("ListApplications() expected error for 500 response, got nil")
	}
}

func TestGetApplication_Success(t *testing.T) {
	t.Parallel()

	payload := `{
		"metadata": {"name": "my-app"},
		"status": {
			"sync": {"status": "Synced"},
			"health": {"status": "Healthy", "message": "all good"}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications/my-app" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	app, err := client.GetApplication(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("GetApplication() unexpected error: %v", err)
	}
	if app.Name != "my-app" {
		t.Errorf("expected name %q, got %q", "my-app", app.Name)
	}
	if app.SyncStatus != SyncStatusSynced {
		t.Errorf("expected sync status %q, got %q", SyncStatusSynced, app.SyncStatus)
	}
	if app.Health != HealthHealthy {
		t.Errorf("expected health %q, got %q", HealthHealthy, app.Health)
	}
	if app.Message != "all good" {
		t.Errorf("expected message %q, got %q", "all good", app.Message)
	}
}

func TestGetApplication_NotFound(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetApplication(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("GetApplication() expected error for 404, got nil")
	}
	if !errors.Is(err, ErrAppNotFound) {
		t.Errorf("expected error wrapping ErrAppNotFound, got: %v", err)
	}
}

func TestGetApplication_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.GetApplication(context.Background(), "some-app")
	if err == nil {
		t.Fatal("GetApplication() expected error for 500 response, got nil")
	}
	if errors.Is(err, ErrAppNotFound) {
		t.Error("expected non-ErrAppNotFound error for 500 status")
	}
}

func TestSyncApplication_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications/my-app/sync" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req syncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !req.Prune {
			t.Error("expected prune to be true")
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.SyncApplication(context.Background(), "my-app")
	if err != nil {
		t.Fatalf("SyncApplication() unexpected error: %v", err)
	}
}

func TestSyncApplication_Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "sync failed", http.StatusConflict)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	err := client.SyncApplication(context.Background(), "my-app")
	if err == nil {
		t.Fatal("SyncApplication() expected error for non-200 response, got nil")
	}
}
