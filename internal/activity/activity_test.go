package activity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProgressRetainsCompletedStagesAndActualFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "progress.json")
	SetFile(file)
	Plan("Inspect project", "Install dependencies", "Start services")
	Start("Inspect project")
	Start("Install dependencies")
	Detail("npm install")
	Finish(errors.New("registry unavailable"))
	p := Read(file)
	if p.Steps[0].Status != "completed" || p.Steps[1].Status != "failed" || p.Steps[2].Status != "pending" || p.Error != "registry unavailable" {
		t.Fatal(p)
	}
}
func TestFailureShowsCurrentError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "output")
	os.WriteFile(file, []byte("setup output\n  [error] port 8443 is already in use\n      Choose another port.\n"), 0600)
	got := Failure(file, "failed; see log")
	if got != "port 8443 is already in use\nChoose another port." {
		t.Fatal(got)
	}
}
