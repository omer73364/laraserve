package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"laraserve/internal/manifest"
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

func TestNormalStartupRepairsApprovedBrowserTrustAndHonorsOptOut(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux browser trust")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	db := filepath.Join(home, ".mozilla", "firefox", "default")
	for _, d := range []string{bin, db} {
		os.MkdirAll(d, 0700)
	}
	os.WriteFile(filepath.Join(db, "cert9.db"), nil, 0600)
	log := filepath.Join(home, "calls")
	script := `#!/bin/sh
 if [ "$3" = -A ]; then printf 'import\n' >> "$TRUST_LOG"; else exit 1; fi
`
	os.WriteFile(filepath.Join(bin, "certutil"), []byte(script), 0700)
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("LARASERVE_HOME", filepath.Join(home, "state"))
	t.Setenv("TRUST_LOG", log)
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := c.ensureTrust(nil, true, false)
	if err != nil {
		t.Fatal(err)
	}
	c.man.CA = &manifest.CAEntry{Method: "linux", SHA256: ca.SHA256, SHA1: ca.SHA1}
	oldYes := ui.Yes
	ui.Yes = false
	t.Cleanup(func() { ui.Yes = oldYes })
	if _, err = c.ensureTrust(nil, false, false); err != nil {
		t.Fatal(err)
	}
	if len(c.man.CA.NSSDBs) != 1 {
		t.Fatal("normal startup did not recover browser trust")
	}
	b, _ := os.ReadFile(log)
	if string(b) != "import\n" {
		t.Fatal(string(b))
	}
	if _, err = c.ensureTrust(nil, true, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(log)
	if string(after) != string(b) {
		t.Fatal("--no-trust performed trust operations")
	}
	// Even a regenerated CA must not remove existing OS trust under --no-trust.
	c.man.CA.SHA256 = "previous-approved-ca"
	if _, err = c.ensureTrust([]string{"new-domain.test"}, true, false); err != nil {
		t.Fatal(err)
	}
	if c.man.CA.SHA256 != "previous-approved-ca" {
		t.Fatal("opt-out changed previous trust record")
	}
}

func TestDashboardSetupInstallsTrustWithoutTerminalPrompt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux trust installation")
	}
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	os.MkdirAll(bin, 0700)
	for _, tool := range []string{"cp", "update-ca-certificates", "certutil", "sudo"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte("#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$TRUST_LOG\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("LARASERVE_HOME", filepath.Join(home, "state"))
	t.Setenv("TRUST_LOG", filepath.Join(home, "calls"))
	oldYes := ui.Yes
	ui.Yes = false
	t.Cleanup(func() { ui.Yes = oldYes })
	c, err := load()
	if err != nil {
		t.Fatal(err)
	}
	c.autoSetup = true
	if _, err = c.ensureTrust(nil, false, false); err != nil {
		t.Fatal(err)
	}
	if c.man.CA == nil {
		t.Fatal("automatic HTTPS setup skipped trust")
	}
	c.man.CA = nil
	os.WriteFile(filepath.Join(bin, "update-ca-certificates"), []byte("#!/bin/sh\nexit 1\n"), 0700)
	os.WriteFile(filepath.Join(bin, "sudo"), []byte("#!/bin/sh\nexit 1\n"), 0700)
	if _, err = c.ensureTrust(nil, false, false); err == nil {
		t.Fatal("failed automatic trust was silently accepted")
	}
}
