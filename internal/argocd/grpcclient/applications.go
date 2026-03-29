package grpcclient

import (
	"context"
	"errors"
	"fmt"

	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// ErrAppNotFound is returned when a requested application does not exist.
var ErrAppNotFound = errors.New("application not found")

// ListApplications returns a summary status for every application visible to
// the authenticated user.
func (c *Client) ListApplications(ctx context.Context) ([]AppStatus, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	list, err := client.List(ctx, &applicationpkg.ApplicationQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}

	statuses := make([]AppStatus, 0, len(list.Items))
	for i := range list.Items {
		statuses = append(statuses, toAppStatus(list.Items[i]))
	}
	return statuses, nil
}

// GetApplication returns the detailed status for a single application by name.
func (c *Client) GetApplication(ctx context.Context, name string) (*AppDetail, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	app, err := client.Get(ctx, &applicationpkg.ApplicationQuery{Name: &name})
	if err != nil {
		return nil, fmt.Errorf("getting application %q: %w", name, err)
	}

	detail := &AppDetail{
		AppStatus: toAppStatus(*app),
		Resources: toResourceStatuses(app.Status.Resources),
	}
	return detail, nil
}

// toAppStatus converts an ArgoCD Application to the domain AppStatus type.
func toAppStatus(app v1alpha1.Application) AppStatus {
	return AppStatus{
		Name:       app.Name,
		SyncStatus: string(app.Status.Sync.Status),
		Health:     string(app.Status.Health.Status),
		Message:    app.Status.Health.Message,
	}
}

// toResourceStatuses converts the ArgoCD ResourceStatus slice to the domain type.
func toResourceStatuses(resources []v1alpha1.ResourceStatus) []ResourceStatus {
	result := make([]ResourceStatus, 0, len(resources))
	for _, r := range resources {
		rs := ResourceStatus{
			Group:     r.Group,
			Kind:      r.Kind,
			Namespace: r.Namespace,
			Name:      r.Name,
			Status:    string(r.Status),
		}
		if r.Health != nil {
			rs.Health = string(r.Health.Status)
			rs.Message = r.Health.Message
		}
		result = append(result, rs)
	}
	return result
}
