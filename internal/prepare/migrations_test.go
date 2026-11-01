package prepare

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMigrationsRetryUntilSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime requires POSIX shell")
	}
	dir := t.TempDir()
	php := filepath.Join(dir, "frankenphp")
	log := filepath.Join(dir, "calls")
	if err := os.WriteFile(php, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LOG\"\nexit \"$STATUS\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	// Reproduce a previous start that created SQLite but never migrated it.
	if _, err := EnsureState(dir); err != nil {
		t.Fatal(err)
	}
	s, err := EnsureState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Fresh {
		t.Fatal("expected existing database")
	}
	run := func(status string, force bool, wantFailure bool) {
		t.Helper()
		err := s.EnsureMigrations(php, dir, []string{"LOG=" + log, "STATUS=" + status}, force)
		if (err != nil) != wantFailure {
			t.Fatalf("unexpected migration result: %v", err)
		}
	}
	marker := filepath.Join(dir, "migrations.ready")
	run("1", false, true)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("failed migration must not be marked ready")
	}
	run("0", false, false)
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
	// Ready setup is skipped, but --migrate forces another attempt.
	run("1", false, false)
	run("1", true, true)
	run("0", false, false)
	// Replacing the database must invalidate old success state too.
	s.Fresh = true
	run("0", false, false)
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for i := 0; i < 5; i++ {
		want += "php-cli artisan migrate --force --no-interaction\n"
	}
	if string(b) != want {
		t.Fatalf("unexpected migration calls: %q", b)
	}
}
