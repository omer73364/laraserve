package projectcheck

import (
	"encoding/json"
	"laraserve/internal/paths"
	"laraserve/internal/versions"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func projectFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{"artisan": "<?php", "composer.json": `{"require":{"php":"^8.2","laravel/framework":"^12"}}`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCheckKeepsCompatibleRecordedNodeVersion(t *testing.T) {
	root := projectFixture(t)
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"engines":{"node":">=22"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	index := `[{"version":"v22.23.0","lts":"Jod","files":["linux-x64","linux-arm64","osx-arm64-tar","osx-x64-tar","win-x64-zip"]},{"version":"v24.10.0","lts":"Krypton","files":["linux-x64","linux-arm64","osx-arm64-tar","osx-x64-tar","win-x64-zip"]}]`
	if err := os.WriteFile(filepath.Join(cache, "node-releases.json"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	if err := versions.WriteLock(filepath.Join(cache, "node-project-"+paths.ProjectID(root)+".json"), versions.Pins{Node: "24.10.0"}); err != nil {
		t.Fatal(err)
	}
	check := checkByID(t, Inspect(root, cache), "node")
	if check.Status != "ok" || !strings.Contains(check.Detail, "24.10.0") {
		t.Fatalf("%+v", check)
	}
}
func checkByID(t *testing.T, r Report, id string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing check %s", id)
	return Check{}
}
func TestCheckExplainsAutomaticPreparationWithoutChangingFiles(t *testing.T) {
	root := projectFixture(t)
	cache := filepath.Join(t.TempDir(), "not-created")
	for name, data := range map[string]string{".env.example": "APP_KEY=private-example", "package.json": `{"devDependencies":{"laravel-mix":"^6"}}`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report := Inspect(root, cache)
	if report.Errors != 0 {
		t.Fatalf("%+v", report)
	}
	for _, id := range []string{"vendor", "environment", "node", "modules"} {
		if c := checkByID(t, report, id); c.Status != "automatic" {
			t.Fatalf("%+v", c)
		}
	}
	for _, path := range []string{cache, filepath.Join(root, ".env"), filepath.Join(root, "vendor"), filepath.Join(root, "node_modules")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("check modified project or cache", path, err)
		}
	}
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "private-example") {
		t.Fatal("environment value leaked")
	}
}
func TestCheckFindsConflictingLockfilesAndNodePin(t *testing.T) {
	root := projectFixture(t)
	cache := t.TempDir()
	for name, data := range map[string]string{"package.json": `{"engines":{"node":"<24"}}`, "package-lock.json": "{}", "yarn.lock": "", ".nvmrc": "26"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	index := `[{"version":"v22.23.0","lts":"Jod","files":["linux-x64","linux-arm64","osx-arm64-tar","osx-x64-tar","win-x64-zip"]}]`
	if err := os.WriteFile(filepath.Join(cache, "node-releases.json"), []byte(index), 0600); err != nil {
		t.Fatal(err)
	}
	report := Inspect(root, cache)
	if c := checkByID(t, report, "manager"); c.Status != "error" {
		t.Fatalf("%+v", c)
	}
	if c := checkByID(t, report, "node"); c.Status != "warning" || !strings.Contains(c.Detail, ".nvmrc") {
		t.Fatalf("%+v", c)
	}
}
func TestCheckRejectsInvalidEnvironmentAndDoesNotReadSecrets(t *testing.T) {
	root := projectFixture(t)
	cache := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("DB_PASSWORD=private-password"), 0600); err != nil {
		t.Fatal(err)
	}
	report := Inspect(root, cache)
	data, _ := json.Marshal(report)
	if strings.Contains(string(data), "private-password") || checkByID(t, report, "environment").Status != "ok" {
		t.Fatal(string(data))
	}
	if err := os.Remove(filepath.Join(root, ".env")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".env"), 0700); err != nil {
		t.Fatal(err)
	}
	if c := checkByID(t, Inspect(root, cache), "environment"); c.Status != "error" {
		t.Fatalf("%+v", c)
	}
}
func TestMissingProjectReturnsActionableError(t *testing.T) {
	r := Inspect(filepath.Join(t.TempDir(), "missing"), t.TempDir())
	if r.Errors != 1 || checkByID(t, r, "project").Status != "error" {
		t.Fatalf("%+v", r)
	}
}

func TestCheckRespectsExplicitFrontendOptOut(t *testing.T) {
	root := projectFixture(t)
	for name, data := range map[string]string{"package.json": "invalid JSON", ".laraserve.json": `{"services":{"auto_detect":false}}`} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r := Inspect(root, t.TempDir())
	if r.Errors != 0 || checkByID(t, r, "frontend").Status != "warning" {
		t.Fatalf("%+v", r)
	}
}
