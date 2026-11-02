package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"laraserve/internal/paths"
)

func TestSupportReportExcludesPrivateDataAndDoesNotDownload(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private-user-secret")
	os.MkdirAll(root, 0700)
	t.Setenv("LARASERVE_HOME", root)
	t.Setenv("DB_PASSWORD", "sensitive-password")
	t.Setenv("LARASERVE_PHP_URL", "http://unreachable.invalid/should-not-download")
	// Malformed state must be diagnosable without copying its contents or error.
	os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{"password":"sensitive-manifest",BROKEN}`), 0600)
	out := filepath.Join(t.TempDir(), "report.json")
	if err := cmdDoctor("--report", out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{root, "private-user-secret", "sensitive-password", "sensitive-manifest", "unreachable.invalid", "DB_PASSWORD"} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("report contains private data: %s", secret)
		}
	}
	var report supportReport
	if err = json.Unmarshal(b, &report); err != nil || report.Schema != 1 {
		t.Fatal(err)
	}
	if len(report.Checks) == 0 || report.Checks[0].Status != "warning" {
		t.Fatal("missing state diagnosis")
	}
	if err = cmdDoctor("--report", out); err == nil {
		t.Fatal("overwrote existing report")
	}
	if _, err = os.Stat(filepath.Join(root, "runtimes")); !os.IsNotExist(err) {
		t.Fatal("doctor modified runtime cache")
	}
}

func TestSupportReportCountsOnlyCompletedTools(t *testing.T) {
	home := &paths.Home{Root: t.TempDir()}
	os.MkdirAll(filepath.Join(home.Runtimes(), "complete"), 0700)
	os.WriteFile(filepath.Join(home.Runtimes(), "complete", ".ready"), nil, 0600)
	os.MkdirAll(filepath.Join(home.Runtimes(), ".partial"), 0700)
	report := collectSupportReport(home)
	if report.CachedRuntimes != 1 {
		t.Fatal(report.CachedRuntimes)
	}
}
