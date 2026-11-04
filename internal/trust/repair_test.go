//go:build linux

package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"laraserve/internal/certs"
	"laraserve/internal/manifest"
)

func TestRepairMissingResetAndNewBrowserProfiles(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	db := filepath.Join(home, ".mozilla", "firefox", "default")
	for _, d := range []string{bin, db} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ca, _, err := certs.EnsureCA(filepath.Join(home, "ca"), certs.DefaultPermitted)
	if err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
 db=${2#sql:}
 case "$3" in
 -A)
  printf 'import\n' >> "$IMPORT_LOG"
  [ "$FAIL_IMPORT" != 1 ] || exit 1
  /bin/cp "$CA_TEST_CERT" "$db/imported.pem" ;;
 -L)
  [ -f "$db/imported.pem" ] || exit 255
  if [ "${4-}" = -n ]; then /bin/cat "$db/imported.pem"; else printf 'Laraserve Local CA C,,\n'; fi ;;
 esac
`
	if err = os.WriteFile(filepath.Join(bin, "certutil"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("CA_TEST_CERT", ca.PEMPath)
	log := filepath.Join(home, "imports")
	t.Setenv("IMPORT_LOG", log)
	entry := &manifest.CAEntry{Method: "linux", SHA256: ca.SHA256}
	// Failed imports remain retryable without a new consent prompt or trust command.
	t.Setenv("FAIL_IMPORT", "1")
	Repair(ca, entry)
	if len(entry.NSSDBs) != 0 {
		t.Fatal("recorded failed import")
	}
	t.Setenv("FAIL_IMPORT", "0")
	Repair(ca, entry)
	if len(entry.NSSDBs) != 1 {
		t.Fatal(entry.NSSDBs)
	}
	t.Setenv("FAIL_IMPORT", "1")
	Repair(ca, entry)
	b, _ := os.ReadFile(log)
	if strings.Count(string(b), "import") != 2 {
		t.Fatal("healthy profile was reimported", string(b))
	}
	// Recorded paths alone are insufficient: reset browser databases need repair.
	if err = os.Remove(filepath.Join(db, "imported.pem")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAIL_IMPORT", "0")
	Repair(ca, entry)
	newDB := filepath.Join(home, ".mozilla", "firefox", "new-profile")
	os.MkdirAll(newDB, 0700)
	os.WriteFile(filepath.Join(newDB, "cert9.db"), nil, 0600)
	Repair(ca, entry)
	b, _ = os.ReadFile(log)
	if strings.Count(string(b), "import") != 4 || len(entry.NSSDBs) != 2 {
		t.Fatal(string(b), entry.NSSDBs)
	}
}
