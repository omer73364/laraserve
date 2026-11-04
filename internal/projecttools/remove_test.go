package projecttools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"laraserve/internal/appsettings"
	"laraserve/internal/projects"
)

func TestRemoveRegistrationPreservesAndRestoresData(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	store := projects.New(home)
	p := &projects.Project{ID: "abcdef123456", Name: "Keep this name", Source: root, Root: root, Args: []string{root, "--http=true"}}
	if err := store.Save(p); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	private := filepath.Join(home, "database.sqlite")
	for _, f := range []string{source, private} {
		if err := os.WriteFile(f, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_, err := appsettings.Change(context.Background(), home, func(c *appsettings.Config) error { c.Projects[p.ID] = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = RemoveProject(context.Background(), home, p.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List()
	if err != nil || len(listed) != 0 {
		t.Fatal("project is still listed", err)
	}
	if err = store.Import(&projects.Project{ID: p.ID, Source: root, Root: root}); err != nil {
		t.Fatal(err)
	}
	listed, _ = store.List()
	if len(listed) != 0 {
		t.Fatal("legacy import resurrected project")
	}
	for _, f := range []string{source, private} {
		b, e := os.ReadFile(f)
		if e != nil || string(b) != "preserve" {
			t.Fatal("removed source or data", f, e)
		}
	}
	restored, err := RegisterExisting(home, root)
	if err != nil || restored.Removed || restored.Name != p.Name || len(restored.Args) != 2 {
		t.Fatal(restored, err)
	}
	pref, _ := appsettings.Read(home)
	if pref.Config.Projects[p.ID] {
		t.Fatal("removed project kept startup enabled")
	}
}
