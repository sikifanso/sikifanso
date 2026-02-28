package main

import "testing"

func TestResolveBootstrapVersion(t *testing.T) {
	tests := []struct {
		name               string
		cliVersion         string
		isDefaultBootstrap bool
		explicitVersion    string
		versionSet         bool
		want               string
	}{
		{
			name:               "explicit version set",
			cliVersion:         "v0.5.0",
			isDefaultBootstrap: true,
			explicitVersion:    "v0.3.0",
			versionSet:         true,
			want:               "v0.3.0",
		},
		{
			name:               "explicit empty version (force HEAD)",
			cliVersion:         "v0.5.0",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         true,
			want:               "",
		},
		{
			name:               "custom bootstrap URL without explicit version",
			cliVersion:         "v0.5.0",
			isDefaultBootstrap: false,
			explicitVersion:    "",
			versionSet:         false,
			want:               "",
		},
		{
			name:               "dev build",
			cliVersion:         "dev",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         false,
			want:               "",
		},
		{
			name:               "snapshot build",
			cliVersion:         "0.4.1-next",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         false,
			want:               "",
		},
		{
			name:               "pre-release build",
			cliVersion:         "v0.5.0-rc1",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         false,
			want:               "",
		},
		{
			name:               "release build with default bootstrap",
			cliVersion:         "v0.5.0",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         false,
			want:               "v0.5.0",
		},
		{
			name:               "release build with v prefix (goreleaser .Tag)",
			cliVersion:         "v0.4.0",
			isDefaultBootstrap: true,
			explicitVersion:    "",
			versionSet:         false,
			want:               "v0.4.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveBootstrapVersion(tt.cliVersion, tt.isDefaultBootstrap, tt.explicitVersion, tt.versionSet)
			if got != tt.want {
				t.Errorf("resolveBootstrapVersion(%q, %v, %q, %v) = %q, want %q",
					tt.cliVersion, tt.isDefaultBootstrap, tt.explicitVersion, tt.versionSet, got, tt.want)
			}
		})
	}
}
