package projecttools

import (
	"os"
	"path/filepath"
	"testing"

	"laraserve/internal/projects"
)

func TestRegisterExistingPreservesFilesAndRecord(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	files := map[string]string{"artisan": "<?php", "composer.json": `{"require":{"laravel/framework":"^12.0","php":"^8.4"}}`, ".env": "DB_PASSWORD=keep-private\n", ".laraserve.json": `{"php":"8.4","database":"sqlite","services":{"queue":true}}`}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := RegisterExisting(home, root)
	if err != nil {
		t.Fatal(err)
	}
	canonical, canonicalErr := projects.Canonical(root)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if p.Source != canonical || p.PHP != "8.4" || p.Database != "SQLite" || len(p.Args) != 1 {
		t.Fatalf("wrong registration: %#v", p)
	}
	p.Name = "Remember this name"
	p.Args = append(p.Args, "--http=true")
	if err = projects.New(home).Save(p); err != nil {
		t.Fatal(err)
	}
	again, err := RegisterExisting(home, root)
	if err != nil || again.Name != p.Name || len(again.Args) != 2 {
		t.Fatalf("registration replaced saved options: %#v, %v", again, err)
	}
	for name, body := range files {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(b) != body {
			t.Fatalf("source changed: %s", name)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != len(files) {
		t.Fatal("registration created project files")
	}
	if _, err = os.Stat(filepath.Join(home, "databases")); !os.IsNotExist(err) {
		t.Fatal("registration provisioned a database")
	}
}
func TestRegisterExistingRejectsInvalidSources(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	for _, path := range []string{"relative", root, filepath.Join(root, "missing")} {
		if _, err := RegisterExisting(home, path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
	os.WriteFile(filepath.Join(root, "artisan"), []byte("<?php"), 0600)
	os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"some/package":"*"}}`), 0600)
	if _, err := RegisterExisting(home, root); err == nil {
		t.Fatal("accepted non-Laravel app")
	}
	os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"laravel/framework":"^12.0"}}`), 0600)
	os.WriteFile(filepath.Join(root, ".laraserve.json"), []byte(`{"invalid":true}`), 0600)
	if _, err := RegisterExisting(home, root); err == nil {
		t.Fatal("accepted invalid saved settings")
	}
}

func TestRegisterExistingKeepsLegacyRegistryIdentity(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	original := &projects.Project{ID: "abcdef123456", Name: "Existing registration", Source: root, Root: root, Args: []string{root, "--http=true"}}
	if err := projects.New(home).Save(original); err != nil {
		t.Fatal(err)
	}
	found, err := RegisterExisting(home, root)
	if err != nil || found.ID != original.ID || found.Name != original.Name {
		t.Fatal("replaced existing identity", found, err)
	}
	all, err := projects.New(home).List()
	if err != nil || len(all) != 1 {
		t.Fatal("created a duplicate registration", err)
	}
}
