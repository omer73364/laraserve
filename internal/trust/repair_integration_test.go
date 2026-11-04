//go:build linux

package trust

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"laraserve/internal/certs"
	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
)

func TestRealBrowserTrustRecovery(t *testing.T) {
	if os.Getenv("LARASERVE_TEST_TRUST_REPAIR") != "1" {
		t.Skip("opt-in private NSS download and temporary browser recovery")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("LARASERVE_HOME", filepath.Join(home, "state"))
	t.Setenv("PATH", "/nonexistent")
	h, err := paths.New()
	if err != nil {
		t.Fatal(err)
	}
	tool, err := runtime.EnsureNSS(h.Runtimes())
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(home, ".mozilla", "firefox", "default")
	os.MkdirAll(db, 0700)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command(tool, args...).CombinedOutput(); err != nil {
			t.Fatalf("NSS: %v %s", err, out)
		}
	}
	run("-d", "sql:"+db, "-N", "--empty-password")
	ca, _, err := certs.EnsureCA(h.CA(), certs.DefaultPermitted)
	if err != nil {
		t.Fatal(err)
	}
	entry := &manifest.CAEntry{Method: "linux", SHA256: ca.SHA256}
	Repair(ca, entry)
	if len(entry.NSSDBs) != 1 || !browserHasCA(tool, db, ca.SHA256) {
		t.Fatal("missing browser trust after recovery")
	}
	run("-d", "sql:"+db, "-D", "-n", certs.CAName)
	Repair(ca, entry)
	if len(entry.NSSDBs) != 1 || !browserHasCA(tool, db, ca.SHA256) {
		t.Fatal("reset profile was not repaired")
	}
	t.Log("Missing host tools, first-use private NSS download, CA fingerprint and trust flags, reset-profile repair, and saved profile tracking passed")
}
