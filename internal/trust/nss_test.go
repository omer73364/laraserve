//go:build linux

package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"laraserve/internal/manifest"
)

func TestMissingCertutilUsesPrivateToolForImportAndRemoval(t *testing.T) {
	home := t.TempDir()
	db := filepath.Join(home, ".mozilla", "firefox", "test.default")
	if err := os.MkdirAll(db, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(home, "private-certutil")
	log := filepath.Join(home, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$NSS_TEST_LOG\"\n"
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("LARASERVE_HOME", filepath.Join(home, "laraserve"))
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("NSS_TEST_LOG", log)
	original := ensureNSSRuntime
	t.Cleanup(func() { ensureNSSRuntime = original })
	calls := 0
	ensureNSSRuntime = func(root string) (string, error) { calls++; return tool, nil }
	modified := installNSS(filepath.Join(home, "ca.pem"))
	if len(modified) != 1 || modified[0] != db {
		t.Fatalf("import not recorded: %v", modified)
	}
	if err := Remove(&manifest.CAEntry{Method: "linux", NSSDBs: modified}); err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "-A -t C,,") || !strings.Contains(string(output), "-D -n") || calls != 2 {
		t.Fatalf("wrong private helper lifecycle: calls=%d %s", calls, output)
	}
}

func TestNoBrowserProfilesDoesNotDownloadTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/nonexistent")
	original := ensureNSSRuntime
	t.Cleanup(func() { ensureNSSRuntime = original })
	ensureNSSRuntime = func(string) (string, error) { t.Fatal("downloaded tools without any browser database"); return "", nil }
	if modified := installNSS("/unused/ca.pem"); len(modified) != 0 {
		t.Fatal(modified)
	}
}

func TestSystemCertutilIsPreferred(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "certutil")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	original := ensureNSSRuntime
	t.Cleanup(func() { ensureNSSRuntime = original })
	ensureNSSRuntime = func(string) (string, error) { t.Fatal("ignored system certutil"); return "", nil }
	got, err := nssTool()
	if err != nil || got != tool {
		t.Fatalf("%s %v", got, err)
	}
}

func TestBrowserRemovalIsIdempotentForMissingCertificates(t *testing.T) {
	home := t.TempDir()
	db := filepath.Join(home, "profile")
	os.MkdirAll(db, 0700)
	tool := filepath.Join(home, "certutil")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\necho SEC_ERROR_UNKNOWN_CERT >&2\nexit 255\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home)
	if err := Remove(&manifest.CAEntry{Method: "linux", NSSDBs: []string{db, filepath.Join(home, "removed-profile")}}); err != nil {
		t.Fatal(err)
	}
}
