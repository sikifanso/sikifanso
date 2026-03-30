package grpcclient

import (
	"context"
	"fmt"

	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// ListApplicationSets returns a summary of every ApplicationSet visible to the
// authenticated user.
func (c *Client) ListApplicationSets(ctx context.Context) ([]AppSetSummary, error) {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	list, err := client.List(ctx, &applicationsetpkg.ApplicationSetListQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing applicationsets: %w", err)
	}

	summaries := make([]AppSetSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, AppSetSummary{
			Name:      list.Items[i].Name,
			Namespace: list.Items[i].Namespace,
		})
	}
	return summaries, nil
}

// GetApplicationSet returns the full ApplicationSet object for the given name.
func (c *Client) GetApplicationSet(ctx context.Context, name string) (*v1alpha1.ApplicationSet, error) {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	appSet, err := client.Get(ctx, &applicationsetpkg.ApplicationSetGetQuery{Name: name})
	if err != nil {
		return nil, fmt.Errorf("getting applicationset %q: %w", name, err)
	}
	return appSet, nil
}

// DeleteApplicationSet deletes the ApplicationSet with the given name.
func (c *Client) DeleteApplicationSet(ctx context.Context, name string) error {
	client, closer, err := c.newAppSetClient()
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	_, err = client.Delete(ctx, &applicationsetpkg.ApplicationSetDeleteRequest{Name: name})
	if err != nil {
		return fmt.Errorf("deleting applicationset %q: %w", name, err)
	}
	return nil
}
