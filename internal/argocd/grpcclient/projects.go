package grpcclient

import (
	"context"
	"fmt"

	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ListProjects returns a summary of every AppProject visible to the
// authenticated user.
func (c *Client) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	list, err := client.List(ctx, &projectpkg.ProjectQuery{})
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	summaries := make([]ProjectSummary, 0, len(list.Items))
	for i := range list.Items {
		summaries = append(summaries, toProjectSummary(list.Items[i]))
	}
	return summaries, nil
}

// GetProject returns a summary for a single AppProject by name.
func (c *Client) GetProject(ctx context.Context, name string) (*ProjectSummary, error) {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return nil, err
	}
	defer func() { _ = closer.Close() }()

	proj, err := client.Get(ctx, &projectpkg.ProjectQuery{Name: name})
	if err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	summary := toProjectSummary(*proj)
	return &summary, nil
}

// CreateProject creates a new AppProject with the given specification.
func (c *Client) CreateProject(ctx context.Context, spec ProjectSpec) error {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	destinations := make([]v1alpha1.ApplicationDestination, 0, len(spec.Destinations))
	for _, d := range spec.Destinations {
		destinations = append(destinations, v1alpha1.ApplicationDestination{
			Server:    d.Server,
			Namespace: d.Namespace,
		})
	}

	proj := &v1alpha1.AppProject{
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.Name,
		},
		Spec: v1alpha1.AppProjectSpec{
			Description:  spec.Description,
			Destinations: destinations,
			SourceRepos:  spec.Sources,
		},
	}

	_, err = client.Create(ctx, &projectpkg.ProjectCreateRequest{Project: proj})
	if err != nil {
		return fmt.Errorf("creating project %q: %w", spec.Name, err)
	}
	return nil
}

// DeleteProject deletes the AppProject with the given name.
func (c *Client) DeleteProject(ctx context.Context, name string) error {
	client, closer, err := c.newProjectClient()
	if err != nil {
		return err
	}
	defer func() { _ = closer.Close() }()

	_, err = client.Delete(ctx, &projectpkg.ProjectQuery{Name: name})
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", name, err)
	}
	return nil
}

// toProjectSummary converts an ArgoCD AppProject to the domain ProjectSummary type.
func toProjectSummary(proj v1alpha1.AppProject) ProjectSummary {
	dests := make([]string, 0, len(proj.Spec.Destinations))
	for _, d := range proj.Spec.Destinations {
		dests = append(dests, fmt.Sprintf("%s/%s", d.Server, d.Namespace))
	}
	return ProjectSummary{
		Name:         proj.Name,
		Description:  proj.Spec.Description,
		Destinations: dests,
		Sources:      proj.Spec.SourceRepos,
	}
}
