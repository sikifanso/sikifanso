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

// SyncApplication triggers a sync for the named application.
func (c *Client) SyncApplication(ctx context.Context, name string, opts SyncOptions) error {
	client, closer, err := c.newAppClient()
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	_, err = client.Sync(ctx, &applicationpkg.ApplicationSyncRequest{
		Name:  &name,
		Prune: &opts.Prune,
	})
	if err != nil {
		return fmt.Errorf("syncing application %q: %w", name, err)
	}
	return nil
}

// WatchApplication returns a channel of WatchEvents for the named application.
// The channel is closed when the stream ends or the context is cancelled.
// The closer is managed by the spawned goroutine.
func (c *Client) WatchApplication(ctx context.Context, name string) (<-chan WatchEvent, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}

	stream, err := client.Watch(ctx, &applicationpkg.ApplicationQuery{Name: &name})
	if err != nil {
		_ = closer.Close()
		return nil, fmt.Errorf("watching application %q: %w", name, err)
	}

	ch := make(chan WatchEvent, 16)
	go func() {
		defer func() { _ = closer.Close() }()
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			event, recvErr := stream.Recv()
			if recvErr != nil {
				return
			}
			we := WatchEvent{
				App:     toAppStatus(event.Application),
				Deleted: string(event.Type) == "DELETED",
			}
			select {
			case ch <- we:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// ResourceTree returns the full resource tree for the named application.
func (c *Client) ResourceTree(ctx context.Context, name string) ([]ResourceStatus, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	tree, err := client.ResourceTree(ctx, &applicationpkg.ResourcesQuery{ApplicationName: &name})
	if err != nil {
		return nil, fmt.Errorf("fetching resource tree for %q: %w", name, err)
	}

	result := make([]ResourceStatus, 0, len(tree.Nodes))
	for _, node := range tree.Nodes {
		rs := ResourceStatus{
			Group:     node.Group,
			Kind:      node.Kind,
			Namespace: node.Namespace,
			Name:      node.Name,
		}
		if node.Health != nil {
			rs.Health = string(node.Health.Status)
			rs.Message = node.Health.Message
		}
		result = append(result, rs)
	}
	return result, nil
}

// ManagedResources returns the list of managed resources with their live/target
// state for the named application.
func (c *Client) ManagedResources(ctx context.Context, name string) ([]ManagedResource, error) {
	client, closer, err := c.newAppClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	resp, err := client.ManagedResources(ctx, &applicationpkg.ResourcesQuery{ApplicationName: &name})
	if err != nil {
		return nil, fmt.Errorf("fetching managed resources for %q: %w", name, err)
	}

	result := make([]ManagedResource, 0, len(resp.Items))
	for _, item := range resp.Items {
		if item == nil {
			continue
		}
		result = append(result, ManagedResource{
			Group:       item.Group,
			Kind:        item.Kind,
			Namespace:   item.Namespace,
			Name:        item.Name,
			LiveState:   item.LiveState,
			TargetState: item.TargetState,
			Diff:        item.NormalizedLiveState,
		})
	}
	return result, nil
}
