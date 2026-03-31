package grpcsync

import (
	"testing"
	"time"
)

func TestRequestDefaults(t *testing.T) {
	t.Parallel()
	r := Request{Timeout: DefaultTimeout}
	if r.Timeout != 5*time.Minute {
		t.Fatalf("default timeout = %v, want 5m", r.Timeout)
	}
	r.Apps = []string{"foo"}
	if len(r.Apps) != 1 || r.Apps[0] != "foo" {
		t.Fatalf("unexpected apps: %v", r.Apps)
	}
}

func TestExitCodes(t *testing.T) {
	t.Parallel()
	if ExitSuccess != 0 {
		t.Fatal("ExitSuccess must be 0")
	}
	if ExitFailure != 1 {
		t.Fatal("ExitFailure must be 1")
	}
	if ExitTimeout != 2 {
		t.Fatal("ExitTimeout must be 2")
	}
}
