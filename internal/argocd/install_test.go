package argocd

import (
	"context"
	"net"
	"testing"
	"time"

	versionpkg "github.com/argoproj/argo-cd/v2/pkg/apiclient/version"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeVersionServer implements the ArgoCD VersionService for testing.
type fakeVersionServer struct {
	versionpkg.UnimplementedVersionServiceServer
}

func (f *fakeVersionServer) Version(_ context.Context, _ *emptypb.Empty) (*versionpkg.VersionMessage, error) {
	return &versionpkg.VersionMessage{Version: "v2.99.0-test"}, nil
}

func startFakeVersionServer(t *testing.T) (addr string, stop func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	versionpkg.RegisterVersionServiceServer(srv, &fakeVersionServer{})
	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.GracefulStop
}

func TestWaitForGRPC_Success(t *testing.T) {
	addr, stop := startFakeVersionServer(t)
	defer stop()

	ctx := context.Background()
	log := zaptest.NewLogger(t)

	if err := WaitForGRPC(ctx, log, addr); err != nil {
		t.Fatalf("WaitForGRPC should succeed: %v", err)
	}
}

func TestWaitForGRPC_Timeout(t *testing.T) {
	// Use an address where nothing is listening.
	addr := "127.0.0.1:1" // port 1 is almost certainly not running a gRPC server

	ctx := context.Background()
	log := zaptest.NewLogger(t)

	// Override timeout to keep the test fast.
	origTimeout := grpcReadyTimeout
	grpcReadyTimeout = 3 * time.Second
	defer func() { grpcReadyTimeout = origTimeout }()

	err := WaitForGRPC(ctx, log, addr)
	if err == nil {
		t.Fatal("WaitForGRPC should have timed out")
	}
}

func TestWaitForGRPC_DelayedStart(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := lis.Addr().String()
	// Close immediately — nothing is serving yet.
	_ = lis.Close()

	ctx := context.Background()
	log := zaptest.NewLogger(t)

	// Start the server after a short delay. Use a channel to detect listen
	// failures so we don't call t.Logf on a potentially finished test.
	srv := grpc.NewServer()
	versionpkg.RegisterVersionServiceServer(srv, &fakeVersionServer{})
	listenErr := make(chan error, 1)

	go func() {
		time.Sleep(2 * time.Second)
		lis2, err := net.Listen("tcp", addr)
		if err != nil {
			listenErr <- err
			return
		}
		listenErr <- nil
		_ = srv.Serve(lis2)
	}()
	t.Cleanup(srv.GracefulStop)

	if err := WaitForGRPC(ctx, log, addr); err != nil {
		// Check if the goroutine failed to re-listen before blaming WaitForGRPC.
		select {
		case lErr := <-listenErr:
			if lErr != nil {
				t.Skipf("port %s was reclaimed by OS: %v", addr, lErr)
			}
		default:
		}
		t.Fatalf("WaitForGRPC should succeed after delayed start: %v", err)
	}
}
