package grpcclient

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient"
	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	applicationsetpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/applicationset"
	projectpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/project"
	sessionpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/session"
)

// Options configures the gRPC connection to an ArgoCD server.
type Options struct {
	Address  string // host:port for gRPC — ArgoCD multiplexes gRPC on the same port as HTTP
	Username string
	Password string
}

// AddressFromURL extracts host:port from an HTTP URL. ArgoCD multiplexes gRPC
// and HTTP on the same port via CMux, so the gRPC address is the same as the
// HTTP URL's host:port.
func AddressFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parsing ArgoCD URL: %w", err)
	}
	return u.Host, nil
}

// Client wraps the ArgoCD gRPC API via the official SDK.
type Client struct {
	apiClient apiclient.Client
	log       *zap.Logger
}

// dialTimeout bounds the whole connect-and-authenticate sequence.
//
// The upstream apiclient does not honour our context for dialling:
// apiclient.NewClient eagerly probes the Version endpoint to decide whether
// gRPC-web is needed, and NewSessionClient dials — both on a hardcoded
// context.Background() (newConn → waitForReady → WaitForStateChange). A refused
// connection fails fast, but an address that accepts TCP and never completes the
// HTTP/2 handshake blocks forever; that is exactly what k3d's load balancer looks
// like while ArgoCD is still starting. No caller supplies a deadline, so the
// bound has to live here.
var dialTimeout = 30 * time.Second

// NewClient connects to ArgoCD over gRPC, authenticates with the given
// credentials, and returns an authenticated Client. It gives up after
// dialTimeout rather than blocking indefinitely on an unreachable server.
func NewClient(ctx context.Context, opts Options) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()

	type result struct {
		client *Client
		err    error
	}
	done := make(chan result, 1)
	go func() {
		c, err := connect(ctx, opts)
		done <- result{client: c, err: err}
	}()

	select {
	case res := <-done:
		return res.client, res.err
	case <-ctx.Done():
		// The goroutine may still be wedged inside the upstream dial; it is
		// abandoned and dies with the process.
		return nil, fmt.Errorf("timed out connecting to ArgoCD gRPC at %s after %s", opts.Address, dialTimeout)
	}
}

// connect performs the blocking connect-and-authenticate sequence. It must be
// called under a bound — see NewClient.
func connect(ctx context.Context, opts Options) (*Client, error) {
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

// FromSessionCreds creates an authenticated gRPC client using the ArgoCD URL
// and credentials typically stored in a session.
func FromSessionCreds(ctx context.Context, argocdURL, username, password string) (*Client, error) {
	addr, err := AddressFromURL(argocdURL)
	if err != nil {
		return nil, err
	}
	return NewClient(ctx, Options{
		Address:  addr,
		Username: username,
		Password: password,
	})
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
