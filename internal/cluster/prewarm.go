package cluster

import (
	"context"
	"fmt"
	"runtime"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"go.uber.org/zap"
)

// prewarmGitOpsMount works around a Docker Desktop bug that otherwise makes
// cluster creation fail whenever a cluster was created earlier in the same hour.
//
// Docker Desktop shares /Users into its Linux VM over virtiofs with
// entry_timeout=3600,attr_timeout=3600, and nothing invalidates the guest's
// dentry cache when the host unlinks a path. Creating a cluster deletes and
// re-scaffolds the gitops directory, so the VM keeps serving the *previous*
// cluster's cached (now-deleted) dentries for up to an hour. k3d then copies its
// entrypoint scripts into the created-but-not-yet-started server container, and
// the daemon's copy-to-stopped-container path resolves bind sources through that
// stale guest cache:
//
//	error while creating mount source path '/host_mnt/...': mkdir ...: no such file or directory
//
// The container still starts — without its entrypoint — and crash-loops with
// "exec /bin/k3d-entrypoint.sh failed", which surfaces as an opaque
// "failed to get ready ... status=restarting".
//
// Starting any container with the same bind heals the cache, because the start
// path resolves bind sources host-side. So we start a throwaway one first.
//
// Best-effort by design: every failure path only logs at debug. A genuine mount
// problem will fail loudly moments later in ClusterRun with a better message,
// and on native Linux there is no VM and nothing to warm.
func prewarmGitOpsMount(ctx context.Context, log *zap.Logger, gitopsDir, k3sImage string) {
	// Docker runs natively here; there is no guest cache to go stale.
	if runtime.GOOS == "linux" {
		return
	}

	docker, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Debug("mount prewarm skipped: cannot create docker client", zap.Error(err))
		return
	}
	defer func() { _ = docker.Close() }()

	// Reuse the k3s image rather than pulling anything. If it is not present the
	// cache cannot be stale for this path either: staleness requires an earlier
	// cluster to have mounted it, which would have left the image behind.
	images, err := docker.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", k3sImage)),
	})
	if err != nil || len(images) == 0 {
		log.Debug("mount prewarm skipped: k3s image not present locally",
			zap.String("image", k3sImage), zap.Error(err))
		return
	}

	created, err := docker.ContainerCreate(ctx,
		&container.Config{
			Image:      k3sImage,
			Entrypoint: []string{"/bin/true"},
		},
		&container.HostConfig{
			Binds:      []string{fmt.Sprintf("%s:/prewarm", gitopsDir)},
			AutoRemove: false,
		},
		nil, nil, "")
	if err != nil {
		log.Debug("mount prewarm skipped: container create failed", zap.Error(err))
		return
	}
	defer func() {
		_ = docker.ContainerRemove(ctx, created.ID, container.RemoveOptions{Force: true})
	}()

	// The start itself is the fix; the container exits immediately and we do not
	// care about its result.
	if err := docker.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		log.Debug("mount prewarm skipped: container start failed", zap.Error(err))
		return
	}

	log.Debug("gitops mount prewarmed", zap.String("dir", gitopsDir))
}
