package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrAppNotFound is returned when an application does not exist in ArgoCD.
var ErrAppNotFound = errors.New("application not found")

// Client is a thin REST client for the ArgoCD API.
// It uses net/http to avoid importing the heavy argo-cd/v2 module.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates an ArgoCD REST client and authenticates using the
// provided credentials. The returned client caches the Bearer token for
// subsequent requests.
func NewClient(ctx context.Context, baseURL, username, password string) (*Client, error) {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	token, err := c.authenticate(ctx, username, password)
	if err != nil {
		return nil, fmt.Errorf("authenticating with ArgoCD: %w", err)
	}
	c.token = token

	return c, nil
}

// Well-known ArgoCD sync and health status values.
const (
	SyncStatusSynced    = "Synced"
	SyncStatusOutOfSync = "OutOfSync"

	HealthHealthy     = "Healthy"
	HealthDegraded    = "Degraded"
	HealthProgressing = "Progressing"
	HealthSuspended   = "Suspended"
	HealthMissing     = "Missing"
	HealthUnknown     = "Unknown"
)

// AppStatus represents the sync and health status of an ArgoCD Application.
type AppStatus struct {
	Name       string `json:"name"`
	SyncStatus string `json:"syncStatus"`
	Health     string `json:"health"`
	Message    string `json:"message,omitempty"`
}

// applicationList is the response from GET /api/v1/applications.
type applicationList struct {
	Items []applicationItem `json:"items"`
}

// applicationItem is a single ArgoCD Application resource.
type applicationItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Sync struct {
			Status string `json:"status"`
		} `json:"sync"`
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message,omitempty"`
		} `json:"health"`
	} `json:"status"`
}

// sessionRequest is the body for POST /api/v1/session.
type sessionRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// sessionResponse is the response from POST /api/v1/session.
type sessionResponse struct {
	Token string `json:"token"`
}

// syncRequest is the body for POST /api/v1/applications/{name}/sync.
type syncRequest struct {
	Prune bool `json:"prune"`
}

func (c *Client) authenticate(ctx context.Context, username, password string) (string, error) {
	body, err := json.Marshal(sessionRequest{Username: username, Password: password})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/session", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("authentication failed: %s", resp.Status)
	}

	var result sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding session response: %w", err)
	}

	return result.Token, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)

	return c.httpClient.Do(req)
}

// ListApplications returns the sync and health status of all ArgoCD Applications.
func (c *Client) ListApplications(ctx context.Context) ([]AppStatus, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/applications", nil)
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("listing applications: %s", resp.Status)
	}

	var list applicationList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decoding application list: %w", err)
	}

	apps := make([]AppStatus, 0, len(list.Items))
	for _, item := range list.Items {
		apps = append(apps, AppStatus{
			Name:       item.Metadata.Name,
			SyncStatus: item.Status.Sync.Status,
			Health:     item.Status.Health.Status,
			Message:    item.Status.Health.Message,
		})
	}

	return apps, nil
}

// GetApplication returns the status of a single ArgoCD Application.
func (c *Client) GetApplication(ctx context.Context, name string) (*AppStatus, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/applications/"+url.PathEscape(name), nil)
	if err != nil {
		return nil, fmt.Errorf("getting application %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrAppNotFound, name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getting application %q: %s", name, resp.Status)
	}

	var item applicationItem
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return nil, fmt.Errorf("decoding application %q: %w", name, err)
	}

	return &AppStatus{
		Name:       item.Metadata.Name,
		SyncStatus: item.Status.Sync.Status,
		Health:     item.Status.Health.Status,
		Message:    item.Status.Health.Message,
	}, nil
}

// SyncApplication triggers a sync for a specific ArgoCD Application.
func (c *Client) SyncApplication(ctx context.Context, name string) error {
	body, err := json.Marshal(syncRequest{Prune: true})
	if err != nil {
		return err
	}

	resp, err := c.doRequest(ctx, http.MethodPost, "/api/v1/applications/"+url.PathEscape(name)+"/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("syncing application %q: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("syncing application %q: %s — %s", name, resp.Status, string(respBody))
	}

	return nil
}
