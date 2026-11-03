//go:build linux

package integration

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laraserve/internal/devservice"
	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
)

func TestRealDevelopmentWorkerLifecycle(t *testing.T) {
	binary, php := os.Getenv("LARASERVE_TEST_LIFECYCLE_BINARY"), os.Getenv("LARASERVE_TEST_FRANKENPHP")
	if binary == "" || php == "" {
		t.Skip("provide the real client and trusted PHP runtime")
	}
	home, root := filepath.Join(t.TempDir(), "home"), filepath.Join(t.TempDir(), "app")
	for _, dir := range []string{"public", "vendor"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"composer.json":       `{"require":{"php":"^8.5","laravel/framework":"^12.0"}}`,
		"vendor/autoload.php": "<?php",
		"public/index.php":    "<?php echo 'worker-proof';",
		".laraserve.json":     `{"services":{"queue":true,"scheduler":true},"database":"sqlite"}`,
		"artisan":             `<?php echo $argv[1]."\n";while(true){if($argv[1]==='queue:work'&&file_exists(__DIR__.'/fail-worker'))exit(7);usleep(100000);}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	canonical, _ := projects.Canonical(root)
	id := paths.ProjectID(canonical)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = supervisor.Stop(ctx, home, id)
	})
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = append(os.Environ(), "LARASERVE_HOME="+home, "LARASERVE_FRANKENPHP="+php, "NO_COLOR=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	start := func() { run("start", root, "--http", "--php-version", "8.5", "--mysql=false", "--phpmyadmin=false") }
	start()
	state, err := supervisor.Status(home, id)
	if err != nil || state.Status != "running" {
		t.Fatal(state, err)
	}
	statuses := devservice.ReadStatus(filepath.Join(home, "projects", id), state.Generation, state.Status)
	if len(statuses) != 2 || statuses[0].State != "running" || statuses[1].State != "running" {
		t.Fatal(statuses)
	}
	for _, name := range []string{"queue", "scheduler"} {
		b, err := os.ReadFile(filepath.Join(home, "projects", id, "logs", name+".log"))
		if err != nil || !strings.Contains(string(b), map[string]string{"queue": "queue:work", "scheduler": "schedule:work"}[name]) {
			t.Fatal(string(b), err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "fail-worker"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		state, err = supervisor.Status(home, id)
		if err == nil && state.Status == "failed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil || state.Status != "failed" {
		t.Fatal("worker exit did not fail its owner", state, err)
	}
	client := http.Client{Timeout: time.Second}
	if response, err := client.Get(state.URL); err == nil {
		response.Body.Close()
		t.Fatal("application survived worker failure")
	}
	os.Remove(filepath.Join(root, "fail-worker"))
	start()
	run("stop", root)
	state, err = supervisor.Status(home, id)
	if err != nil || state.Status != "stopped" {
		t.Fatal(state, err)
	}
	for _, status := range devservice.ReadStatus(filepath.Join(home, "projects", id), state.Generation, state.Status) {
		if status.State != "stopped" {
			t.Fatal(status)
		}
	}
}
