package process

import (
	"runtime"
	"testing"
)

func TestPHPArgs(t *testing.T) {
	got := PHPArgs("artisan", "command with spaces")
	if runtime.GOOS == "windows" {
		if len(got) != 2 || got[0] != "artisan" {
			t.Fatal(got)
		}
	} else {
		if len(got) != 3 || got[0] != "php-cli" || got[1] != "artisan" {
			t.Fatal(got)
		}
	}
	if got[len(got)-1] != "command with spaces" {
		t.Fatal("split argument", got)
	}
}
