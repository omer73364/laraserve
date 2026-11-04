package devservice

import (
	"laraserve/internal/statefile"
	"path/filepath"
	"testing"
)

func TestServiceStatusFollowsOwnerGeneration(t *testing.T) {
	dir := t.TempDir()
	if err := statefile.WriteJSON(filepath.Join(dir, "services.json"), Snapshot{Generation: "one", Services: []Status{{Name: "queue", State: "running"}}}); err != nil {
		t.Fatal(err)
	}
	if got := ReadStatus(dir, "two", "running"); len(got) != 0 {
		t.Fatal("reported stale generation", got)
	}
	for _, tc := range []struct{ owner, want string }{{"running", "running"}, {"stopping", "stopping"}, {"crashed", "crashed"}, {"unresponsive", "unknown"}, {"stopped", "stopped"}} {
		got := ReadStatus(dir, "one", tc.owner)
		if len(got) != 1 || got[0].State != tc.want {
			t.Fatal(tc, got)
		}
	}
}
