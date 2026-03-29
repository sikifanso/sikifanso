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
