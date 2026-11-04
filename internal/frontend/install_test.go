package frontend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDependencyInstallError(t *testing.T) {
	for _, message := range []string{
		"npm error code ERESOLVE",
		"npm error Could not resolve dependency:\nnpm error peer vue@\"^2.0.0\" from @inertiajs/inertia-vue@0.5.5",
	} {
		cause := errors.New(message)
		err := dependencyInstallError("npm", cause)
		if !strings.Contains(err.Error(), "npm dependency conflict:") || !strings.Contains(err.Error(), message) || !errors.Is(err, cause) {
			t.Fatalf("missing explanation or original cause: %v", err)
		}
	}
	cause := errors.New("registry unavailable")
	if got := dependencyInstallError("npm", cause).Error(); got != "npm install failed: registry unavailable" {
		t.Fatal(got)
	}
	if got := dependencyInstallError("pnpm", errors.New("ERESOLVE")).Error(); strings.Contains(got, ".npmrc") {
		t.Fatal("offered npm recovery for another manager")
	}
}

func TestDependencyFingerprintIncludesConfigurationAndNode(t *testing.T) {
	root := t.TempDir()
	before, err := dependencyFingerprint(root, "22.23.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".npmrc", ".yarnrc.yml", "pnpm-workspace.yaml", "package-lock.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("private-registry-token-or-settings"), 0600); err != nil {
			t.Fatal(err)
		}
		after, err := dependencyFingerprint(root, "22.23.0")
		if err != nil || after == before || strings.Contains(after, "private") {
			t.Fatalf("%s: %q %v", name, after, err)
		}
		before = after
	}
	after, err := dependencyFingerprint(root, "24.10.0")
	if err != nil || after == before {
		t.Fatal("Node version did not invalidate dependencies", err)
	}
	if err := os.Remove(filepath.Join(root, ".npmrc")); err != nil {
		t.Fatal(err)
	}
	after, err = dependencyFingerprint(root, "22.23.0")
	if err != nil || after == before {
		t.Fatal("removed configuration did not invalidate dependencies", err)
	}
	if err := os.Mkdir(filepath.Join(root, ".npmrc"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := dependencyFingerprint(root, "22.23.0"); err == nil {
		t.Fatal("silently accepted unreadable frontend configuration")
	}
}
