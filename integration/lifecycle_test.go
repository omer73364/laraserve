//go:build linux

package integration

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
)

// Exercises separate real CLI processes, real Caddy/PHP, detached owners, and
// private state. No user hosts, trust store, projects, or databases are changed.
func TestRealProjectLifecycle(t *testing.T) {
	binary, php := os.Getenv("LARASERVE_TEST_LIFECYCLE_BINARY"), os.Getenv("LARASERVE_TEST_FRANKENPHP")
	if binary == "" || php == "" {
		t.Skip("set LARASERVE_TEST_LIFECYCLE_BINARY and LARASERVE_TEST_FRANKENPHP")
	}
	home := filepath.Join(t.TempDir(), "private-home")
	env := append(os.Environ(), "LARASERVE_HOME="+home, "LARASERVE_FRANKENPHP="+php, "NO_COLOR=1")
	run := func(wantSuccess bool, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if (err == nil) != wantSuccess {
			logs, _ := filepath.Glob(filepath.Join(home, "control", "*-startup.log"))
			for _, file := range logs {
				b, _ := os.ReadFile(file)
				t.Logf("Temporary startup log:\n%s", b)
			}
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	var sources []string
	for _, name := range []string{"one", "two"} {
		source := filepath.Join(t.TempDir(), name, "app")
		for _, dir := range []string{"public", "vendor"} {
			os.MkdirAll(filepath.Join(source, dir), 0700)
		}
		os.WriteFile(filepath.Join(source, "composer.json"), []byte(`{"require":{"php":"^8.5","laravel/framework":"^12.0"}}`), 0600)
		os.WriteFile(filepath.Join(source, "vendor", "autoload.php"), []byte("<?php"), 0600)
		os.WriteFile(filepath.Join(source, "artisan"), []byte("<?php file_put_contents(__DIR__.'/migration-calls',implode(' ',array_slice($argv,1)).\"\\n\",FILE_APPEND);"), 0600)
		os.WriteFile(filepath.Join(source, "public", "index.php"), []byte("<?php if(isset($_GET['db'])){echo getenv('DB_DATABASE');return;} echo '"+name+"';"), 0600)
		sources = append(sources, source)
	}
	// Upgrade a previous path-based registration without redirecting its private DB.
	legacyID := "abcdabcdabcd"
	legacySQLite := filepath.Join(home, "projects", legacyID, "database.sqlite")
	os.MkdirAll(filepath.Dir(legacySQLite), 0700)
	os.WriteFile(legacySQLite, []byte("existing private database"), 0600)
	legacy, err := manifest.Load(filepath.Join(home, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	legacy.Project(legacyID, sources[0])
	if err = legacy.Save(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, source := range sources {
			canonical, _ := projects.Canonical(source)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_ = supervisor.Stop(ctx, home, paths.ProjectID(canonical))
			cancel()
		}
	})
	store := projects.New(home)
	type backgroundResult struct {
		out []byte
		err error
	}
	concurrent := make(chan backgroundResult, 1)
	concurrentCommand := exec.Command(binary, "start", sources[1], "--http", "--php-version", "8.5", "--mysql=false", "--phpmyadmin=false")
	concurrentCommand.Env = env
	go func() { out, err := concurrentCommand.CombinedOutput(); concurrent <- backgroundResult{out, err} }()
	run(true, "start", sources[0], "--http", "--migrate", "--php-version", "8.5", "--mysql=false", "--phpmyadmin=false")
	first, err := store.Find(sources[0])
	if err != nil {
		t.Fatal(err)
	}
	firstState, err := supervisor.Status(home, first.ID)
	if err != nil || firstState.Status != "running" {
		t.Fatal(firstState, err)
	}
	get := func(url, want string) {
		t.Helper()
		client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}
		response, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		if string(b) != want {
			t.Fatalf("response %s, want %s", b, want)
		}
	}
	get(firstState.URL, "one")
	get(firstState.URL+"/?db=1", legacySQLite)
	preserved, _ := os.ReadFile(legacySQLite)
	if string(preserved) != "existing private database" {
		t.Fatal("legacy database was replaced")
	}

	if !strings.Contains(run(true, "projects"), first.ID) {
		t.Fatal("project missing from list")
	}
	run(false, "start", sources[0])
	run(false, "serve", sources[0], "--http")
	run(false, "clean", sources[0])
	select {
	case result := <-concurrent:
		if result.err != nil {
			t.Fatalf("concurrent start: %v %s", result.err, result.out)
		}
	case <-time.After(75 * time.Second):
		t.Fatal("concurrent start did not finish")
	}
	combined, err := manifest.Load(filepath.Join(home, "manifest.json"))
	if err != nil || len(combined.Projects) != 2 {
		t.Fatal("concurrent startup lost footprint records", err)
	}
	second, err := store.Find(sources[1])
	if err != nil {
		t.Fatal(err)
	}
	secondState, err := supervisor.Status(home, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstState.URL == secondState.URL {
		t.Fatal("projects claimed the same address")
	}
	get(secondState.URL, "two")
	run(false, "status", "app")
	run(true, "restart", first.ID)
	restarted, err := supervisor.Status(home, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Generation == firstState.Generation || restarted.URL != firstState.URL {
		t.Fatal("restart ownership or stable URL failed")
	}
	calls, err := os.ReadFile(filepath.Join(sources[0], "migration-calls"))
	if err != nil || strings.Count(string(calls), "migrate") != 1 {
		t.Fatal("restart replayed migration", string(calls), err)
	}
	unrelated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("unrelated")) }))
	defer unrelated.Close()
	run(true, "stop", first.ID)
	get(unrelated.URL, "unrelated")
	get(secondState.URL, "two")
	run(true, "start", first.ID)
	owner, err := supervisor.Status(home, first.ID)
	if err != nil || owner.PID <= 0 || owner.PID == os.Getpid() {
		t.Fatal(owner, err)
	}
	process, err := os.FindProcess(owner.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		state, err := supervisor.Status(home, first.ID)
		response, requestErr := (&http.Client{Timeout: time.Second}).Get(owner.URL)
		if response != nil {
			response.Body.Close()
		}
		if err == nil && state.Status == "crashed" && requestErr != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("crashed owner left a service listening", state, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	run(true, "start", first.ID)
	recovered, err := supervisor.Status(home, first.ID)
	if err != nil || recovered.Status != "running" {
		t.Fatal(recovered, err)
	}
	get(recovered.URL, "one")
	run(true, "stop", first.ID)
	run(true, "stop", second.ID)
	t.Log("Real background start, readiness, duplicate rejection, two apps, stable restart, migration opt-in, isolated stop, Linux crash cleanup, and recovery passed")
}
