package grpcsync

import (
	"testing"
	"time"
)

func TestRequestDefaults(t *testing.T) {
	t.Parallel()
	r := Request{Apps: []string{"foo"}}
	if r.Timeout == 0 {
		r.Timeout = DefaultTimeout
	}
	if r.Timeout != 2*time.Minute {
		t.Fatalf("default timeout = %v, want 2m", r.Timeout)
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
