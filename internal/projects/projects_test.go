package projects

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryIdentityAndAmbiguousNames(t *testing.T) {
	base := t.TempDir()
	s := New(base)
	for _, parent := range []string{"one", "two"} {
		source := filepath.Join(base, parent, "app")
		os.MkdirAll(source, 0700)
		canonical, err := Canonical(source)
		if err != nil {
			t.Fatal(err)
		}
		id, err := Identity(source)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Save(&Project{ID: id, Name: "app", Root: canonical, Source: canonical, Args: []string{canonical, "--http=true"}}); err != nil {
			t.Fatal(err)
		}
		byPath, err := s.Find(source)
		if err != nil || byPath.ID != id {
			t.Fatal(byPath, err)
		}
	}
	if _, err := s.Find("app"); err == nil {
		t.Fatal("ambiguous project selected")
	}
	list, err := s.List()
	if err != nil || len(list) != 2 {
		t.Fatal(list, err)
	}
	alias := filepath.Join(base, "alias")
	if err = os.Symlink(list[0].Source, alias); err == nil {
		a, _ := Identity(alias)
		if a != list[0].ID {
			t.Fatal("symlink created a second identity")
		}
	}
	if err = s.Save(&Project{ID: "../escape"}); err == nil {
		t.Fatal("unsafe registry ID accepted")
	}
}
