package argocd

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	versionpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/version"
	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeVersionServer implements the ArgoCD VersionService for testing.
type fakeVersionServer struct {
	versionpkg.UnimplementedVersionServiceServer
}

func (f *fakeVersionServer) Version(_ context.Context, _ *emptypb.Empty) (*versionpkg.VersionMessage, error) {
	return &versionpkg.VersionMessage{Version: "v3.99.0-test"}, nil
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

// TestWaitForGRPC_UnresponsiveListener covers the failure mode that a refused
// connection does not: a listener that completes the TCP handshake and then
// says nothing. This is what k3d's load balancer does when a host port is
// mapped to a NodePort no service claims — the dial appears to succeed and the
// gRPC handshake never finishes. WaitForGRPC must still honour its timeout
// rather than hang, so the caller gets a diagnosable error.
// TestWaitForGRPC_BlackHoleThenReady reproduces what k3d's load balancer actually
// does while ArgoCD is starting: accept TCP without ever completing the HTTP/2
// handshake, then serve real gRPC on the same port once the pod is up.
//
// TestWaitForGRPC_DelayedStart does not cover this. It closes the port, so dials are
// refused and fail fast, leaving the client healthy. A black hole instead poisons any
// client constructed during the dead window — apiclient.NewClient eagerly probes
// Version to choose its transport, and that choice sticks. WaitForGRPC must therefore
// build a fresh client per attempt; reusing one makes the wait fail for its entire
// budget while a new client connects immediately, which is what happened on
// `cluster start` in acceptance testing.
func TestWaitForGRPC_BlackHoleThenReady(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	tl, ok := lis.(*net.TCPListener)
	if !ok {
		t.Fatalf("expected *net.TCPListener, got %T", lis)
	}
	addr := tl.Addr().String()

	var mu sync.Mutex
	var held []net.Conn
	switched := make(chan struct{})
	rawDone := make(chan struct{})

	// Phase 1: accept connections and never answer.
	go func() {
		defer close(rawDone)
		for {
			select {
			case <-switched:
				return
			default:
			}
			_ = tl.SetDeadline(time.Now().Add(50 * time.Millisecond))
			conn, err := tl.Accept()
			if err != nil {
				continue // deadline expired; re-check switched
			}
			mu.Lock()
			held = append(held, conn)
			mu.Unlock()
		}
	}()

	srv := grpc.NewServer()
	versionpkg.RegisterVersionServiceServer(srv, &fakeVersionServer{})
	t.Cleanup(func() {
		srv.Stop()
		mu.Lock()
		for _, c := range held {
			_ = c.Close()
		}
		mu.Unlock()
	})

	// Phase 2: stop black-holing and serve gRPC on the same listener.
	go func() {
		time.Sleep(2 * time.Second)
		close(switched)
		<-rawDone
		_ = tl.SetDeadline(time.Time{})
		_ = srv.Serve(tl)
	}()

	origTimeout := grpcReadyTimeout
	grpcReadyTimeout = 30 * time.Second
	defer func() { grpcReadyTimeout = origTimeout }()

	// Bound the test: on a regression WaitForGRPC burns its whole budget.
	returned := make(chan error, 1)
	go func() { returned <- WaitForGRPC(context.Background(), zaptest.NewLogger(t), addr) }()

	select {
	case err := <-returned:
		if err != nil {
			t.Fatalf("WaitForGRPC should recover once the listener starts serving: %v", err)
		}
	case <-time.After(45 * time.Second):
		t.Fatal("WaitForGRPC never returned")
	}
}

func TestWaitForGRPC_UnresponsiveListener(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = lis.Close() }()

	// Accept connections and hold them open without ever writing a byte.
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				<-done
				_ = conn.Close()
			}()
		}
	}()

	origTimeout := grpcReadyTimeout
	grpcReadyTimeout = 2 * time.Second
	defer func() { grpcReadyTimeout = origTimeout }()

	// Bound the test itself: on a regression this call hangs indefinitely, and
	// a plain blocking call would stall the whole suite until the panic timeout.
	returned := make(chan error, 1)
	go func() { returned <- WaitForGRPC(context.Background(), zaptest.NewLogger(t), lis.Addr().String()) }()

	select {
	case err := <-returned:
		if err == nil {
			t.Fatal("WaitForGRPC succeeded against an unresponsive listener")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("WaitForGRPC hung past its timeout against an unresponsive listener")
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
