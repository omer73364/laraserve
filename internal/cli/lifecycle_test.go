package cli

import (
	"strings"
	"testing"
)

func TestRestartDoesNotReplayOneShotMigrationOrConsent(t *testing.T) {
	options := newServeOptions()
	if err := parse(options.fs, []string{"/app", "--http", "--migrate", "--yes", "--allow-insecure-dependencies", "--mysql=false"}); err != nil {
		t.Fatal(err)
	}
	remembered := strings.Join(rememberServeArgs(options.fs, "/app"), " ")
	for _, forbidden := range []string{"migrate", "--yes", "--allow-insecure"} {
		if strings.Contains(remembered, forbidden) {
			t.Fatal("replayed one-shot action", remembered)
		}
	}
	if !strings.Contains(remembered, "--http=true") || !strings.Contains(remembered, "--mysql=false") {
		t.Fatal("lost persistent settings", remembered)
	}
}
