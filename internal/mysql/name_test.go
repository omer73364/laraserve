package mysql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseNames(t *testing.T) {
	for app, want := range map[string]string{"example-app": "example_app", "My Shop": "my_shop", "123-app": "app_123_app", "mysql": "app_mysql", "!!!": "app", "demo.laraserve": "demo"} {
		if got := DatabaseName(app); got != want {
			t.Errorf("%q: got %s want %s", app, got, want)
		}
	}
	a, b := DatabaseName(strings.Repeat("a", 80)), DatabaseName(strings.Repeat("a", 79)+"b")
	if len(a) > 64 || a == b || !validDatabaseName.MatchString(a) {
		t.Fatal(a, b)
	}
}

func TestDatabaseNamePersistsAndPreservesLegacy(t *testing.T) {
	fresh := t.TempDir()
	name, err := projectDatabaseName(fresh, "my-app")
	if err != nil || name != "my_app" {
		t.Fatal(name, err)
	}
	name, err = projectDatabaseName(fresh, "renamed-app")
	if err != nil || name != "my_app" {
		t.Fatal("changed persisted database", name, err)
	}
	legacy := t.TempDir()
	if err := os.WriteFile(filepath.Join(legacy, "initialized"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	name, err = projectDatabaseName(legacy, "my-app")
	if err != nil || name != "laraserve" {
		t.Fatal("abandoned legacy database", name, err)
	}
	if err := os.WriteFile(filepath.Join(fresh, "database-name"), []byte("bad`name"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := projectDatabaseName(fresh, "app"); err == nil {
		t.Fatal("accepted unsafe stored identifier")
	}
}
