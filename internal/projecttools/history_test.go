package projecttools

import (
	"laraserve/internal/statefile"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskHistorySeparatesProjectsAndOmitsOutput(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(taskDir(home), 0700)
	for i, id := range []string{"111111111111", "222222222222"} {
		p := "project-a"
		if i == 1 {
			p = "project-b"
		}
		statefile.WriteJSON(taskFile(home, id), Task{ID: id, ProjectID: p, Label: "Artisan list", ProjectName: p, Status: "succeeded", Started: "2026-11-04T00:00:00Z"})
		os.WriteFile(filepath.Join(taskDir(home), id+".log"), []byte("private command output"), 0600)
	}
	tasks, e := ListTasks(home, "project-a")
	if e != nil || len(tasks) != 1 || tasks[0].ProjectID != "project-a" || tasks[0].Label != "Artisan list" || tasks[0].Output != "" {
		t.Fatal(tasks, e)
	}
}
