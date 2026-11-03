//go:build linux

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"laraserve/internal/dashboard"
	"laraserve/internal/paths"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/projecttools"
	"laraserve/internal/supervisor"
)

func TestRealDashboardLifecycle(t *testing.T) {
	binary, php := os.Getenv("LARASERVE_TEST_LIFECYCLE_BINARY"), os.Getenv("LARASERVE_TEST_FRANKENPHP")
	if binary == "" || php == "" {
		t.Skip("set LARASERVE_TEST_LIFECYCLE_BINARY and LARASERVE_TEST_FRANKENPHP")
	}
	home, root := t.TempDir(), t.TempDir()
	for _, dir := range []string{"public", "vendor", "storage/logs"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{".env": "APP_NAME=Original\nDB_PASSWORD=very-secret\n", ".env.example": "APP_NAME=Example\nDB_PASSWORD=example-secret\nNEW_VARIABLE=default\n", "composer.json": `{"require":{"php":"^8.5","laravel/framework":"^12.0"}}`, "vendor/autoload.php": "<?php", "artisan": `<?php $cmd=$argv[1]??""; if($cmd==="migrate"){file_put_contents(__DIR__."/unexpected-migration","explicit");echo "migration requested";exit(0);} if($cmd==="fail"){echo "deliberate failure";exit(7);} if($cmd==="slow"){echo "started slow task\n";flush();sleep(20);exit(0);} echo json_encode(["memory"=>ini_get("memory_limit"),"database"=>getenv("DB_DATABASE")]);`, "public/index.php": "<?php if(isset($_GET['memory'])){echo ini_get('memory_limit');return;} echo 'dashboard-real-app';", "storage/logs/laravel.log": "INFO normal\nERROR dashboard marker <script>\n"}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := projects.Canonical(root)
	if err != nil {
		t.Fatal(err)
	}
	p := &projects.Project{ID: paths.ProjectID(source), Name: "Dashboard app", Source: source, Root: source, Args: []string{source, "--http=true", "--php-version=8.5", "--mysql=false", "--phpmyadmin=false", "--migrate=true"}}
	if err = projects.New(home).Save(p); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "dashboard")
	cmd.Env = append(os.Environ(), "LARASERVE_HOME="+home, "LARASERVE_FRANKENPHP="+php, "NO_COLOR=1")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Signal(syscall.SIGTERM)
			cmd.Wait()
		}
	}()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = supervisor.Stop(ctx, home, p.ID)
	})
	address := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			for _, field := range strings.Fields(scanner.Text()) {
				if strings.HasPrefix(field, "http://127.0.0.1:") && strings.Contains(field, "/#") {
					select {
					case address <- field:
					default:
					}
				}
			}
		}
	}()
	var raw string
	select {
	case raw = <-address:
	case <-time.After(10 * time.Second):
		t.Fatal("no dashboard address")
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + u.Host
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	request := func(method, path, body, csrf string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, origin+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := request("GET", "/api/projects", "", "")
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("unauthenticated data exposed")
	}
	body, _ := json.Marshal(map[string]string{"token": u.Fragment})
	response = request("POST", "/session", string(body), "")
	response.Body.Close()
	if response.StatusCode != 204 {
		t.Fatal("bootstrap failed", response.StatusCode)
	}
	response = request("GET", "/api/session", "", "")
	var session map[string]string
	err = json.NewDecoder(response.Body).Decode(&session)
	response.Body.Close()
	if err != nil || session["csrf"] == "" {
		t.Fatal(session, err)
	}
	csrf := session["csrf"]
	action := func(name string) {
		t.Helper()
		r := request("POST", "/api/projects/"+p.ID+"/"+name, "{}", csrf)
		defer r.Body.Close()
		if r.StatusCode != 202 {
			b, _ := io.ReadAll(r.Body)
			t.Fatalf("%s: %d %s", name, r.StatusCode, b)
		}
	}
	await := func(want string) dashboard.View {
		t.Helper()
		deadline := time.Now().Add(75 * time.Second)
		for time.Now().Before(deadline) {
			r := request("GET", "/api/projects", "", "")
			var views []dashboard.View
			e := json.NewDecoder(r.Body).Decode(&views)
			r.Body.Close()
			if e != nil || len(views) != 1 {
				t.Fatal(e, views)
			}
			v := views[0]
			if v.Operation != nil && v.Operation.Error != "" {
				t.Fatal(v.Operation.Error)
			}
			if v.Status == want && (v.Operation == nil || !v.Operation.Pending) {
				return v
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("timed out waiting for " + want)
		return dashboard.View{}
	}
	action("start")
	running := await("running")
	response, err = client.Get(running.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(b) != "dashboard-real-app" {
		t.Fatal(string(b))
	}
	if !running.Resources.Available || running.Resources.RSS == 0 {
		t.Fatal("no measured resources", running.Resources)
	}
	old, err := supervisor.Status(home, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/projects/"+p.ID+"/logs?source=laravel&q=ERROR", "", "")
	var logs dashboard.LogResult
	err = json.NewDecoder(response.Body).Decode(&logs)
	response.Body.Close()
	if err != nil || !strings.Contains(logs.Text, "dashboard marker") || strings.Contains(logs.Text, "INFO") {
		t.Fatal(logs, err)
	}
	response = request("GET", "/api/projects/"+p.ID+"/logs?source=server", "", "")
	err = json.NewDecoder(response.Body).Decode(&logs)
	response.Body.Close()
	if err != nil || logs.Text == "" {
		t.Fatal("server logs unavailable", err)
	}
	action("restart")
	restarted := await("running")
	current, _ := supervisor.Status(home, p.ID)
	if restarted.URL != running.URL || old.Generation == current.Generation {
		t.Fatal("restart did not preserve URL/change owner")
	}
	if _, err = os.Stat(filepath.Join(root, "unexpected-migration")); !os.IsNotExist(err) {
		t.Fatal("dashboard replayed a migration", err)
	}

	// Phase 4: edit configuration and masked environment through the same session.
	response = request("GET", "/api/projects/"+p.ID+"/settings", "", "")
	var config projectconfig.Document
	if err = json.NewDecoder(response.Body).Decode(&config); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	plain := false
	config.Config = projectconfig.Config{PHP: "8.5", HTTPS: &plain, Database: "sqlite", PHPSettings: map[string]string{"memory_limit": "192M"}}
	encoded, _ := json.Marshal(config)
	response = request("POST", "/api/projects/"+p.ID+"/settings", string(encoded), csrf)
	if response.StatusCode != 200 {
		b, _ := io.ReadAll(response.Body)
		t.Fatal(response.StatusCode, string(b))
	}
	response.Body.Close()
	response = request("POST", "/api/projects/"+p.ID+"/settings", string(encoded), csrf)
	response.Body.Close()
	if response.StatusCode != 409 {
		t.Fatal("stale settings accepted")
	}
	action("restart")
	updated := await("running")
	response, err = client.Get(updated.URL + "/?memory=1")
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if string(b) != "192M" {
		t.Fatal("PHP override not applied", string(b))
	}
	response = request("GET", "/api/projects/"+p.ID+"/environment", "", "")
	b, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if strings.Contains(string(b), "very-secret") || strings.Contains(string(b), "example-secret") {
		t.Fatal("environment secret leaked")
	}
	var environment projectconfig.Environment
	if err = json.Unmarshal(b, &environment); err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(map[string]any{"revision": environment.Revision, "changes": map[string]string{"APP_NAME": "Updated app"}})
	response = request("POST", "/api/projects/"+p.ID+"/environment", string(encoded), csrf)
	if response.StatusCode != 200 {
		b, _ := io.ReadAll(response.Body)
		t.Fatal(string(b))
	}
	response.Body.Close()
	b, _ = os.ReadFile(filepath.Join(root, ".env"))
	if !strings.Contains(string(b), "DB_PASSWORD=very-secret") || !strings.Contains(string(b), "Updated app") {
		t.Fatal("environment preservation failed", string(b))
	}
	runTask := func(command string, confirm bool) projecttools.Task {
		t.Helper()
		data, _ := json.Marshal(projecttools.TaskRequest{Kind: "artisan", Args: []string{command}, ConfirmSchema: confirm})
		r := request("POST", "/api/projects/"+p.ID+"/tasks", string(data), csrf)
		defer r.Body.Close()
		if r.StatusCode != 200 {
			b, _ := io.ReadAll(r.Body)
			t.Fatal(r.StatusCode, string(b))
		}
		var task projecttools.Task
		if e := json.NewDecoder(r.Body).Decode(&task); e != nil {
			t.Fatal(e)
		}
		return task
	}
	awaitTask := func(id string) projecttools.Task {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			r := request("GET", "/api/tasks/"+id, "", "")
			var task projecttools.Task
			e := json.NewDecoder(r.Body).Decode(&task)
			r.Body.Close()
			if e != nil {
				t.Fatal(e)
			}
			if task.Status != "queued" && task.Status != "running" {
				return task
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("task timed out")
		return projecttools.Task{}
	}
	task := awaitTask(runTask("about", false).ID)
	if task.Status != "succeeded" || task.ExitCode != 0 || !strings.Contains(task.Output, "192M") {
		t.Fatal(task)
	}
	data, _ := json.Marshal(projecttools.TaskRequest{Kind: "artisan", Args: []string{"migrate"}})
	response = request("POST", "/api/projects/"+p.ID+"/tasks", string(data), csrf)
	response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("migration did not require consent")
	}
	task = awaitTask(runTask("migrate", true).ID)
	if task.ExitCode != 0 {
		t.Fatal(task)
	}
	if b, e := os.ReadFile(filepath.Join(root, "unexpected-migration")); e != nil || string(b) != "explicit" {
		t.Fatal("explicit migration did not run", e)
	}
	task = awaitTask(runTask("fail", false).ID)
	if task.ExitCode != 7 || !strings.Contains(task.Output, "deliberate failure") {
		t.Fatal(task)
	}
	slow := runTask("slow", false)
	time.Sleep(300 * time.Millisecond)
	response = request("POST", "/api/tasks/"+slow.ID+"/cancel", "{}", csrf)
	response.Body.Close()
	task = awaitTask(slow.ID)
	if task.Status == "succeeded" {
		t.Fatal("cancelled task succeeded", task)
	}
	if state, e := supervisor.Status(home, p.ID); e != nil || state.Status != "running" {
		t.Fatal("task cancellation stopped app", state, e)
	}
	action("stop")
	await("stopped")
	action("start")
	await("running")
	if err = cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err, stderr.String())
	}
	waited = true
	state, err := supervisor.Status(home, p.ID)
	if err != nil || state.Status != "running" {
		t.Fatal("dashboard exit stopped app", state, err)
	}
	t.Log("Real dashboard lifecycle, settings/INI, masked environment, task output/exit/cancellation, explicit migrations, and independent project lifetime passed")
}
