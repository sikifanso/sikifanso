package grpcclient

import (
	"context"
	"fmt"
	"io"

	"go.uber.org/zap"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	applicationsetpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/applicationset"
	projectpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/project"
	sessionpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/session"
)

// Options configures the gRPC connection to an ArgoCD server.
type Options struct {
	Address  string // host:port, e.g. "localhost:30084"
	Username string
	Password string
}

// Client wraps the ArgoCD gRPC API via the official SDK.
type Client struct {
	apiClient apiclient.Client
	log       *zap.Logger
}

// NewClient connects to ArgoCD over gRPC, authenticates with the given
// credentials, and returns an authenticated Client.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	baseOpts := &apiclient.ClientOptions{
		ServerAddr: opts.Address,
		Insecure:   true,
		PlainText:  true,
	}

	unauthClient, err := apiclient.NewClient(baseOpts)
	if err != nil {
		return nil, fmt.Errorf("creating ArgoCD gRPC client: %w", err)
	}

	sessConn, sessClient, err := unauthClient.NewSessionClient()
	if err != nil {
		return nil, fmt.Errorf("creating session client: %w", err)
	}
	defer func() { _ = sessConn.Close() }()

	sessResp, err := sessClient.Create(ctx, &sessionpkg.SessionCreateRequest{
		Username: opts.Username,
		Password: opts.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("authenticating with ArgoCD: %w", err)
	}

	authOpts := &apiclient.ClientOptions{
		ServerAddr: opts.Address,
		Insecure:   true,
		PlainText:  true,
		AuthToken:  sessResp.GetToken(),
	}

	authClient, err := apiclient.NewClient(authOpts)
	if err != nil {
		return nil, fmt.Errorf("creating authenticated ArgoCD gRPC client: %w", err)
	}

	return &Client{
		apiClient: authClient,
		log:       zap.NewNop(),
	}, nil
}

// SetLogger configures the logger used by the client.
func (c *Client) SetLogger(log *zap.Logger) {
	c.log = log
}

// Close is a no-op for now; individual sub-clients are closed per-call.
func (c *Client) Close() {}

// newAppClient creates a per-call ApplicationServiceClient.
// The caller must close the returned io.Closer when done.
func (c *Client) newAppClient() (applicationpkg.ApplicationServiceClient, io.Closer, error) {
	conn, client, err := c.apiClient.NewApplicationClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating application client: %w", err)
	}
	return client, conn, nil
}

// newAppSetClient creates a per-call ApplicationSetServiceClient.
// The caller must close the returned io.Closer when done.
func (c *Client) newAppSetClient() (applicationsetpkg.ApplicationSetServiceClient, io.Closer, error) {
	conn, client, err := c.apiClient.NewApplicationSetClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating applicationset client: %w", err)
	}
	return client, conn, nil
}

// newProjectClient creates a per-call ProjectServiceClient.
// The caller must close the returned io.Closer when done.
func (c *Client) newProjectClient() (projectpkg.ProjectServiceClient, io.Closer, error) {
	conn, client, err := c.apiClient.NewProjectClient()
	if err != nil {
		return nil, nil, fmt.Errorf("creating project client: %w", err)
	}
	return client, conn, nil
}
