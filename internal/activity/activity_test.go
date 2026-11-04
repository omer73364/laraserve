package activity

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

func TestOutputKeepsDependencyCauseBeforeLongHelpFooter(t *testing.T) {
	cause := errors.New("exit status 1")
	for _, prefix := range []string{"npm error", "npm ERR!"} {
		output := &Output{}
		_, _ = output.Write([]byte(prefix + " code ERESOLVE\n" + prefix + " peer vue@^2 from old-adapter\n" + strings.Repeat(prefix+" help text\n", 40)))
		err := output.Error(cause)
		if !errors.Is(err, cause) || !strings.Contains(err.Error(), "ERESOLVE") || !strings.Contains(err.Error(), "peer vue@^2") {
			t.Fatal(err)
		}
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
