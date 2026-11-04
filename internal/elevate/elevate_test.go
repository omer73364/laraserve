package elevate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNonInteractiveAuthorizationUsesOSPrompt(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Skip("Linux user authentication")
	}
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "pkexec"), []byte("#!/bin/sh\nexit 0\n"), 0700)
	t.Setenv("PATH", bin)
	t.Setenv("DISPLAY", ":test")
	t.Setenv("SUDO_ASKPASS", "")
	cmd, e := Command("cp", "/private/a file", "/etc/hosts")
	if e != nil {
		t.Fatal(e)
	}
	if !strings.HasSuffix(cmd.Path, "pkexec") || len(cmd.Args) != 4 || cmd.Args[2] != "/private/a file" {
		t.Fatal(cmd.Args)
	}
}

func TestAuthorizationArgumentsPreserveSpecialPaths(t *testing.T) {
	if powershellLiteral("C:\\a'b $x") != "'C:\\a''b $x'" {
		t.Fatal("PowerShell literal not escaped")
	}
	cmd := windowsCopy("C:\\a'`$x", `C:\Windows\System32\drivers\etc\hosts`)
	if len(cmd.Args) != 5 || cmd.Args[3] != "-EncodedCommand" || strings.Contains(cmd.Args[4], "$x") {
		t.Fatal(cmd.Args)
	}
}
