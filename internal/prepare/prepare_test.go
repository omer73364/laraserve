package prepare

import (
	"strings"
	"testing"
)

func TestComposerLegacyPolicyIsExplicitAndScoped(t *testing.T) {
	original := []string{"PATH=/private", "COMPOSER_POLICY_ADVISORIES_BLOCK=1"}
	normal := composerPolicyEnv(original, nil)
	if !strings.Contains(strings.Join(normal, "\n"), "COMPOSER_POLICY_ADVISORIES_BLOCK=1") {
		t.Fatal("default policy changed")
	}
	allowed := composerPolicyEnv(original, []bool{true})
	joined := strings.Join(allowed, "\n")
	if strings.Contains(joined, "COMPOSER_POLICY_ADVISORIES_BLOCK=1") || !strings.Contains(joined, "COMPOSER_POLICY_ADVISORIES_BLOCK=0") {
		t.Fatal(joined)
	}
	if original[1] != "COMPOSER_POLICY_ADVISORIES_BLOCK=1" {
		t.Fatal("caller environment mutated")
	}
}
