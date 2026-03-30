package grpcclient

import (
	"context"
	"testing"
)

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
