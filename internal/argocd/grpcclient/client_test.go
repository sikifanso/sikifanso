package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestNewClient_UnresponsiveListener covers the failure mode a refused address
// does not: a listener that completes the TCP handshake and then never speaks.
// That is what k3d's load balancer looks like while ArgoCD is still starting, and
// the upstream apiclient dials it on a hardcoded context.Background(). Observed
// before dialTimeout existed: `app status` run seconds after `cluster start` hung
// for ten minutes with no output. NewClient must give up instead.
func TestNewClient_UnresponsiveListener(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = lis.Close() }()

	// Accept connections and hold them open without ever writing a byte.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func() {
				<-stop
				_ = conn.Close()
			}()
		}
	}()

	orig := dialTimeout
	dialTimeout = 2 * time.Second
	defer func() { dialTimeout = orig }()

	// Bound the test itself: on a regression this call blocks indefinitely, and a
	// plain blocking call would stall the suite until the panic timeout.
	returned := make(chan error, 1)
	go func() {
		_, err := NewClient(context.Background(), Options{
			Address:  lis.Addr().String(),
			Username: "admin",
			Password: "irrelevant",
		})
		returned <- err
	}()

	select {
	case err := <-returned:
		if err == nil {
			t.Fatal("NewClient succeeded against an unresponsive listener")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("NewClient hung past dialTimeout against an unresponsive listener")
	}
}

func TestNewClient_InvalidAddress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, err := NewClient(ctx, Options{
		Address:  "localhost:0",
		Username: "admin",
		Password: "wrong",
	})
	if err == nil {
		t.Fatal("expected error for unreachable address")
	}
}

func TestAppStatusFields(t *testing.T) {
	t.Parallel()
	s := AppStatus{
		Name:       "test",
		SyncStatus: "Synced",
		Health:     "Healthy",
	}
	if s.Name != "test" || s.SyncStatus != "Synced" || s.Health != "Healthy" {
		t.Fatalf("unexpected values: name=%s sync=%s health=%s", s.Name, s.SyncStatus, s.Health)
	}
}
