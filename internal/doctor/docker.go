package doctor

import (
	"context"
	"fmt"
	"runtime"

	"github.com/alicanalbayrak/sikifanso/internal/preflight"
	"github.com/docker/docker/client"
)

const checkNameDocker = "Docker daemon"

// DockerCheck verifies that the Docker daemon is reachable and reports its version.
type DockerCheck struct{}

func (DockerCheck) Run(ctx context.Context) []Result {
	if err := preflight.CheckDocker(ctx); err != nil {
		return []Result{{
			Name:  checkNameDocker,
			OK:    false,
			Cause: err.Error(),
			Fix:   dockerFixHint(),
		}}
	}

	// Docker is alive, now get version for display.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return []Result{{Name: checkNameDocker, OK: true, Message: "running"}}
	}
	defer func() { _ = cli.Close() }()

	ver, err := cli.ServerVersion(ctx)
	if err != nil {
		return []Result{{Name: checkNameDocker, OK: true, Message: "running"}}
	}

	return []Result{{
		Name:    checkNameDocker,
		OK:      true,
		Message: fmt.Sprintf("running (v%s)", ver.Version),
	}}
}

func dockerFixHint() string {
	if runtime.GOOS == "darwin" {
		return "open -a Docker"
	}
	return "systemctl start docker"
}
