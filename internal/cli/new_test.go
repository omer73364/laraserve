package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewRejectsInvalidDestination(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(file, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{nil, {"a", "b"}, {""}, {root}, {file}} {
		if err := cmdNew(args); err == nil {
			t.Fatalf("expected rejection for %v", args)
		}
	}
	if b, err := os.ReadFile(file); err != nil || string(b) != "keep" {
		t.Fatal("existing file changed")
	}
}

func TestNewUsesBundledComposer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake runtime uses a POSIX shell")
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			php := filepath.Join(dir, "frankenphp")
			log := filepath.Join(dir, "args")
			script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + log + "'\n"
			if fail {
				script += "exit 7\n"
			} else {
				script += "shift 8\n/bin/mkdir -p \"$1\"\nprintf '' > \"$1/artisan\"\nprintf '%s' '{\"require\":{\"laravel/framework\":\"^12.0\"}}' > \"$1/composer.json\"\n"
			}
			if err := os.WriteFile(php, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "composer.phar"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("LARASERVE_HOME", filepath.Join(dir, "home"))
			t.Setenv("LARASERVE_FRANKENPHP", php)
			target := filepath.Join(dir, "my app")
			err := cmdNew([]string{target, "--version", "^12.0"})
			if (err != nil) != fail {
				t.Fatalf("unexpected result: %v", err)
			}
			b, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Join([]string{"php-cli", filepath.Join(dir, "composer.phar"), "create-project", "--no-interaction", "--prefer-dist", "--no-progress", "--", "laravel/laravel", target, "^12.0", ""}, "\n")
			if string(b) != want {
				t.Fatalf("unexpected Composer arguments: %s", b)
			}
		})
	}
}
