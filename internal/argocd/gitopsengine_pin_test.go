package argocd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// argoCDRequireRe matches the argo-cd/v3 version in go.mod's require block.
var argoCDRequireRe = regexp.MustCompile(`(?m)^\s+github\.com/argoproj/argo-cd/v3 (v\S+)$`)

// gitopsEngineReplaceRe captures the commit hash embedded in the gitops-engine replace
// target's pseudo-version (vX.Y.Z-<timestamp>-<12-char commit>).
var gitopsEngineReplaceRe = regexp.MustCompile(
	`(?m)^replace github\.com/argoproj/argo-cd/gitops-engine => \S+ v[\d.]+-\d{14}-([0-9a-f]{12})$`)

// modInfo is the subset of a module cache .info file we need. The go command records
// Origin only when the module was resolved from VCS or a proxy that reports it, so it
// can legitimately be absent.
type modInfo struct {
	Origin *struct {
		Hash string
	}
}

// verifyPin checks that the gitops-engine replace in goMod names a commit that prefixes
// tagCommit — the commit the required argo-cd/v3 version is tagged at.
func verifyPin(goMod []byte, tagCommit string) error {
	argo := argoCDRequireRe.FindSubmatch(goMod)
	if argo == nil {
		return fmt.Errorf("no argo-cd/v3 require directive found in go.mod")
	}
	pin := gitopsEngineReplaceRe.FindSubmatch(goMod)
	if pin == nil {
		return fmt.Errorf("no gitops-engine replace directive found in go.mod")
	}

	pinnedCommit := string(pin[1])
	if !strings.HasPrefix(tagCommit, pinnedCommit) {
		return fmt.Errorf("gitops-engine replace is pinned to commit %s, but argo-cd/v3 %s is tagged at %s\n"+
			"Repoint the replace in go.mod at the argo-cd release tag commit — note that argo-cd's own\n"+
			"require line for gitops-engine is a stale placeholder neutralised by its monorepo replace",
			pinnedCommit, argo[1], tagCommit)
	}
	return nil
}

// TestGitopsEnginePinMatchesArgoCDTag guards the invariant that go.mod's gitops-engine
// replace points at the same commit as the argo-cd/v3 release we require.
//
// argo-cd v3.3+ vendors gitops-engine as an untagged nested module, so the replace has to
// name a pseudo-version by hand. Getting it wrong is silent: a mismatched pin compiles,
// passes every other test, and ships a gitops-engine that does not correspond to the
// deployed server. The check is offline — the argo-cd tag's commit is already recorded in
// the module cache alongside the download.
func TestGitopsEnginePinMatchesArgoCDTag(t *testing.T) {
	goMod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}

	argo := argoCDRequireRe.FindSubmatch(goMod)
	if argo == nil {
		t.Fatalf("no argo-cd/v3 require directive found in go.mod")
	}

	if err := verifyPin(goMod, tagCommitFromModCache(t, "github.com/argoproj/argo-cd/v3", string(argo[1]))); err != nil {
		t.Error(err)
	}
}

// TestVerifyPinDetectsMismatch proves the guard can actually fail. The real go.mod cannot
// be corrupted for this — an invalid pseudo-version stops module resolution before any
// test runs — so the comparison is exercised against synthetic input.
func TestVerifyPinDetectsMismatch(t *testing.T) {
	const tagCommit = "564b94973b284b8de98da7cee6eeade2cb941e46"

	goMod := func(pin string) []byte {
		return []byte("require (\n\tgithub.com/argoproj/argo-cd/v3 v3.4.5\n)\n\n" +
			"replace github.com/argoproj/argo-cd/gitops-engine => github.com/argoproj/argo-cd/gitops-engine " + pin + "\n")
	}

	tests := []struct {
		name    string
		goMod   []byte
		wantErr bool
	}{
		{"matching pin", goMod("v0.0.0-20260709160802-564b94973b28"), false},
		{"stale commit", goMod("v0.0.0-20250908182407-97ad5b59a627"), true},
		{"missing replace", []byte("require (\n\tgithub.com/argoproj/argo-cd/v3 v3.4.5\n)\n"), true},
		{"missing require", []byte("replace github.com/argoproj/argo-cd/gitops-engine => " +
			"github.com/argoproj/argo-cd/gitops-engine v0.0.0-20260709160802-564b94973b28\n"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifyPin(tt.goMod, tagCommit); (err != nil) != tt.wantErr {
				t.Errorf("verifyPin error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// tagCommitFromModCache returns the VCS commit the given module version was resolved from,
// as recorded in the module cache. It skips the test when that metadata is unavailable.
func tagCommitFromModCache(t *testing.T, modulePath, version string) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Skipf("go env GOMODCACHE: %v", err)
	}
	cache := strings.TrimSpace(string(out))
	if cache == "" {
		t.Skip("GOMODCACHE is empty")
	}

	// Module paths here are all lower-case, so no !-escaping of upper-case letters applies.
	path := filepath.Join(cache, "cache", "download", filepath.FromSlash(modulePath), "@v", version+".info")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("module cache entry unavailable (%v) — run 'go mod download %s'", err, modulePath)
	}

	var info modInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	if info.Origin == nil || info.Origin.Hash == "" {
		t.Skipf("%s records no Origin.Hash — the module proxy did not report it", path)
	}
	return info.Origin.Hash
}
