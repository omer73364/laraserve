package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"laraserve/internal/ui"
)

func TestTrustReimportsExistingCA(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux trust installation")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	db := filepath.Join(home, ".mozilla", "firefox", "test.default")
	for _, dir := range []string{bin, db} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"cp", "update-ca-certificates", "certutil", "sudo"} {
		script := "#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$TRUST_LOG\"\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(home, "trust.log")
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("LARASERVE_HOME", filepath.Join(home, "laraserve"))
	t.Setenv("TRUST_LOG", log)
	oldYes := ui.Yes
	t.Cleanup(func() { ui.Yes = oldYes })
	for i := 0; i < 2; i++ {
		if err := cmdTrust([]string{"--yes"}); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "certutil -d sql:"+db+" -A"); n != 2 {
		t.Fatalf("expected two browser imports, got %d: %s", n, b)
	}
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	if c.man.CA == nil || len(c.man.CA.NSSDBs) != 1 || c.man.CA.NSSDBs[0] != db {
		t.Fatalf("browser trust not recorded: %+v", c.man.CA)
	}
}
