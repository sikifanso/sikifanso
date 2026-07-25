package upgrade

import (
	"strings"
	"testing"
)

func TestSkipReason(t *testing.T) {
	tests := []struct {
		name           string
		currentVer     string
		newVer         string
		targetRevision string
		wantSkip       bool
		wantContains   string
	}{
		{
			name:         "same version unpinned",
			currentVer:   "1.19.6",
			newVer:       "1.19.6",
			wantSkip:     true,
			wantContains: "already at latest version",
		},
		{
			name:           "same version pinned",
			currentVer:     "1.19.6",
			newVer:         "1.19.6",
			targetRevision: "1.19.6",
			wantSkip:       true,
			wantContains:   "already at configured version 1.19.6",
		},
		{
			name:           "deployed newer than pin refuses downgrade",
			currentVer:     "1.20.0",
			newVer:         "1.19.6",
			targetRevision: "1.19.6",
			wantSkip:       true,
			wantContains:   "refusing to downgrade",
		},
		{
			name:       "deployed older proceeds",
			currentVer: "1.19.0",
			newVer:     "1.19.6",
			wantSkip:   false,
		},
		{
			name:       "prerelease is older than release",
			currentVer: "1.19.6-rc.1",
			newVer:     "1.19.6",
			wantSkip:   false,
		},
		{
			name:       "unparseable version proceeds",
			currentVer: "not-semver",
			newVer:     "1.19.6",
			wantSkip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := skipReason(tt.currentVer, tt.newVer, tt.targetRevision)
			if tt.wantSkip {
				if got == "" {
					t.Fatalf("skipReason(%q, %q, %q) = %q, want a skip reason",
						tt.currentVer, tt.newVer, tt.targetRevision, got)
				}
				if !strings.Contains(got, tt.wantContains) {
					t.Errorf("skipReason(%q, %q, %q) = %q, want it to contain %q",
						tt.currentVer, tt.newVer, tt.targetRevision, got, tt.wantContains)
				}
				return
			}
			if got != "" {
				t.Errorf("skipReason(%q, %q, %q) = %q, want no skip",
					tt.currentVer, tt.newVer, tt.targetRevision, got)
			}
		})
	}
}
