package prepare

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposerPHPLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	dir := t.TempDir()
	php := filepath.Join(dir, "franken php's")
	log := filepath.Join(dir, "args")
	if err := os.WriteFile(php, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, invocation := range []string{
		`"$PHP_BINARY" -d allow_url_fopen=1 -dmemory_limit=-1 artisan package:discover --ansi 'argument with spaces' -d keep-me`,
		`php -d allow_url_fopen=1 artisan package:discover --ansi 'argument with spaces' -d keep-me`,
	} {
		cmd := exec.Command("/bin/sh", "-c", invocation)
		if err := runComposer(cmd, php, []string{"LOG=" + log, "PATH=/nonexistent", "PHP_BINARY=/wrong"}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		want := "php-cli\nartisan\npackage:discover\n--ansi\nargument with spaces\n-d\nkeep-me\n"
		if string(b) != want {
			t.Fatalf("got %q, want %q", b, want)
		}
		for _, entry := range cmd.Env {
			if strings.HasPrefix(entry, "PHP_BINARY=") {
				if _, err := os.Stat(strings.TrimPrefix(entry, "PHP_BINARY=")); !os.IsNotExist(err) {
					t.Fatal("temporary launcher was not removed")
				}
			}
		}
	}
}
