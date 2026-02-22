package preflight

import (
	"context"
	"fmt"

	"github.com/docker/docker/client"
)

// CheckDocker verifies the Docker daemon is reachable.
func CheckDocker(ctx context.Context) error {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("creating docker client: %w", err)
	}
	defer func() { _ = cli.Close() }()

	_, err = cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("pinging docker daemon: %w", err)
	}

	return nil
}
