package scaffold

import (
	"laraserve/internal/paths"
	"os"
	"path/filepath"
	"testing"
)

func TestCreationRequiresANewOrEmptyDestination(t *testing.T) {
	base := t.TempDir()
	existing := filepath.Join(base, "existing")
	os.MkdirAll(existing, 0700)
	os.WriteFile(filepath.Join(existing, "keep"), []byte("keep"), 0600)
	for _, request := range []Request{{Path: existing}, {Path: "relative"}, {Path: filepath.Join(base, "new"), Starter: "arbitrary/package"}, {Path: filepath.Join(base, "new"), Database: "unknown"}, {Path: filepath.Join(base, "new"), Starter: "react", Version: "^8.0"}} {
		if request.Validate() == nil {
			t.Fatal("invalid creation accepted", request)
		}
	}
	good := Request{Path: filepath.Join(base, "new"), Starter: "vue", Database: "mysql"}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(existing, "keep"))
	if string(b) != "keep" {
		t.Fatal("existing project changed")
	}
}

func TestRetainedPrivateDataBlocksNewAppMigrations(t *testing.T) {
	home, base := t.TempDir(), t.TempDir()
	root := filepath.Join(base, "new-app")
	// A new empty/missing app path may still have a database from a removed app.
	id := paths.ProjectID(root)
	file := filepath.Join(home, "projects", id, "database.sqlite")
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte("retained data"), 0600)
	if err := guardCreationState(home, root); err == nil {
		t.Fatal("retained DB allowed new creation")
	}
	b, _ := os.ReadFile(file)
	if string(b) != "retained data" {
		t.Fatal("retained data changed")
	}
}

func TestCreationValidatesConnectionOptions(t *testing.T) {
	port := 70000
	if err := (Request{Path: filepath.Join(t.TempDir(), "app"), Port: &port}).Validate(); err == nil {
		t.Fatal("invalid port accepted")
	}
	if err := (Request{Path: filepath.Join(t.TempDir(), "app"), Domain: "https://example.test"}).Validate(); err == nil {
		t.Fatal("invalid domain accepted")
	}
	port = 8443
	https := true
	if err := (Request{Path: filepath.Join(t.TempDir(), "app"), Domain: "example.test", Port: &port, HTTPS: &https, Vite: true}).Validate(); err != nil {
		t.Fatal(err)
	}
}
