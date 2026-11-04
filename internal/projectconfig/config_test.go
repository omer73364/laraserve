package projectconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSettingsValidationAndAtomicConflict(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	doc, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{PHP: "8.3", Database: "managed", PHPSettings: map[string]string{"memory_limit": "256M"}, PHPRaw: "opcache.revalidate_freq=2"}
	saved, err := Save(home, root, "project", doc.Revision, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Save(home, root, "project", doc.Revision, Config{}); err == nil {
		t.Fatal("stale write accepted")
	}
	cfg.PHP = "8.4"
	if _, err = Save(home, root, "project", saved.Revision, cfg); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(filepath.Join(home, "history", "project", "*-config.bak"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	for _, bad := range []Config{{PHPRaw: "extension=/tmp/code.so"}, {PHPRaw: "auto_prepend_file=/tmp/code.php"}, {PHPSettings: map[string]string{"memory_limit": "256M\nauto_prepend_file=x"}}, {PHPSettings: map[string]string{"memory_limit": "128M"}, PHPRaw: "memory_limit=256M"}, {Domain: "bad..test"}, {PHP: "../../bin/php"}} {
		if bad.Validate() == nil {
			t.Fatal("invalid config accepted", bad)
		}
	}
	file := filepath.Join(root, ".laraserve.json")
	os.Remove(file)
	other := filepath.Join(home, "other")
	os.WriteFile(other, []byte(`{}`), 0600)
	if os.Symlink(other, file) == nil {
		if _, err = Read(root); err == nil {
			t.Fatal("symlink config accepted")
		}
	}
}
func TestEnvironmentMaskingPreservationAndConflict(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	original := "# keep comment\r\nAPP_NAME=Old\r\nDB_PASSWORD='very-secret'\r\nCUSTOM_VALUE=opaque\r\n"
	os.WriteFile(filepath.Join(root, ".env"), []byte(original), 0600)
	os.WriteFile(filepath.Join(root, ".env.example"), []byte("APP_NAME=Example\nDB_PASSWORD=example-secret\nMISSING=example\n"), 0600)
	doc, err := ReadEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range doc.Variables {
		if v.Key == "DB_PASSWORD" && (v.Value != "" || v.Example != "" || !v.Secret) {
			t.Fatal("secret leaked", v)
		}
		if v.Key == "CUSTOM_VALUE" && !v.Secret {
			t.Fatal("unknown variable not private")
		}
	}
	saved, err := UpdateEnvironment(home, root, "project", doc.Revision, map[string]string{"APP_NAME": "New app", "MISSING": "$literal\\path"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, ".env"))
	if !strings.Contains(string(b), "# keep comment") || !strings.Contains(string(b), "DB_PASSWORD='very-secret'") {
		t.Fatal("unchanged contents lost", string(b))
	}
	if _, err = UpdateEnvironment(home, root, "project", doc.Revision, map[string]string{"APP_NAME": "stale"}); err == nil {
		t.Fatal("stale environment accepted")
	}
	if _, err = UpdateEnvironment(home, root, "project", saved.Revision, map[string]string{"BAD\nKEY": "x"}); err == nil {
		t.Fatal("newline injection accepted")
	}
	backups, _ := filepath.Glob(filepath.Join(home, "history", "project", "*-env.bak"))
	if len(backups) != 1 {
		t.Fatal(backups)
	}
	info, _ := os.Stat(filepath.Join(root, ".env"))
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatal("environment not private")
	}
}

func TestEnvironmentQuotesRoundTrip(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	doc, err := ReadEnvironment(root)
	if err != nil {
		t.Fatal(err)
	}
	value := `Omar's $literal "quoted" \path`
	saved, err := UpdateEnvironment(home, root, "project", doc.Revision, map[string]string{"APP_NAME": value})
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Variables) != 1 || saved.Variables[0].Value != value {
		t.Fatal(saved)
	}
}
