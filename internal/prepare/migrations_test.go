package prepare

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrationsRequireExplicitRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime requires POSIX shell")
	}
	dir := t.TempDir()
	php := filepath.Join(dir, "php")
	log := filepath.Join(dir, "calls")
	if err := os.WriteFile(php, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$LOG\"\nexit \"$STATUS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	s, err := EnsureState(dir)
	if err != nil {
		t.Fatal(err)
	}
	run := func(requested bool, status string) error {
		return s.EnsureMigrations(php, dir, []string{"LOG=" + log, "STATUS=" + status}, requested)
	}
	// Fresh, existing, and previously failed databases all remain untouched.
	if err := run(false, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("fresh database auto-migrated")
	}
	s.Fresh = false
	if err := run(true, "1"); err == nil {
		t.Fatal("migration failure ignored")
	}
	if err := run(false, "1"); err != nil {
		t.Fatal("implicitly retried failed migration")
	}
	if err := run(true, "0"); err != nil {
		t.Fatal(err)
	}
	// Every explicit request runs, even after a successful migration.
	if err := run(true, "0"); err != nil {
		t.Fatal(err)
	}
	if err := run(false, "1"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("php-cli artisan migrate --force --no-interaction\n", 3)
	if string(b) != want {
		t.Fatalf("unexpected migrations: %q", b)
	}
}
