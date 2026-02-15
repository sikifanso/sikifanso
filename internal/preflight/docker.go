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
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	defer cli.Close()

	_, err = cli.Ping(ctx)
	if err != nil {
		return fmt.Errorf("docker daemon is not running: %w", err)
	}

	return nil
}
