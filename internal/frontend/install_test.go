package frontend

import (
	"errors"
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
