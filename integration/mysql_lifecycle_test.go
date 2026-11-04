//go:build linux

package integration

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
)

func TestRealMySQLLifecycleRecovery(t *testing.T) {
	binary, php, distribution := os.Getenv("LARASERVE_TEST_LIFECYCLE_BINARY"), os.Getenv("LARASERVE_TEST_FRANKENPHP"), os.Getenv("LARASERVE_TEST_MYSQL_RUNTIME")
	if binary == "" || php == "" || distribution == "" {
		t.Skip("set lifecycle binary, FrankenPHP, and MySQL runtime test paths")
	}
	home := filepath.Join(t.TempDir(), "home")
	base := filepath.Join(t.TempDir(), "mysql")
	if err := cloneRuntime(distribution, base); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "my-database-app")
	for _, dir := range []string{"public", "vendor"} {
		os.MkdirAll(filepath.Join(source, dir), 0700)
	}
	os.WriteFile(filepath.Join(source, "composer.json"), []byte(`{"require":{"php":"^8.5","laravel/framework":"^12.0"}}`), 0600)
	os.WriteFile(filepath.Join(source, "vendor", "autoload.php"), []byte("<?php"), 0600)
	os.WriteFile(filepath.Join(source, "artisan"), []byte("<?php exit(17);"), 0600)
	os.WriteFile(filepath.Join(source, ".env"), []byte("DB_CONNECTION=mysql\n"), 0600)
	script := `<?php
 $p=new PDO('mysql:unix_socket='.getenv('DB_SOCKET').';dbname='.getenv('DB_DATABASE'),getenv('DB_USERNAME'),getenv('DB_PASSWORD'));
 if(isset($_GET['write'])){$p->exec('CREATE TABLE phase2_probe (id INT PRIMARY KEY)');$p->exec('INSERT INTO phase2_probe VALUES (7)');}
 $exists=$p->query("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='phase2_probe'")->fetchColumn();
 echo getenv('DB_DATABASE').':'.($exists?$p->query('SELECT id FROM phase2_probe')->fetchColumn():'empty');
`
	os.WriteFile(filepath.Join(source, "public", "index.php"), []byte(script), 0600)
	env := append(os.Environ(), "LARASERVE_HOME="+home, "LARASERVE_FRANKENPHP="+php, "LARASERVE_MYSQL_DIR="+base, "NO_COLOR=1")
	canonical, _ := projects.Canonical(source)
	id := paths.ProjectID(canonical)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		_ = supervisor.Stop(ctx, home, id)
	})
	run := func(args ...string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			log, _ := os.ReadFile(filepath.Join(home, "control", id+"-startup.log"))
			t.Fatalf("%v: %v\n%s\n%s", args, err, out, log)
		}
	}
	get := func(url, want string) {
		t.Helper()
		transport := &http.Transport{Proxy: nil}
		defer transport.CloseIdleConnections()
		response, err := (&http.Client{Transport: transport, Timeout: 5 * time.Second}).Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		b, _ := io.ReadAll(response.Body)
		if string(b) != want {
			t.Fatalf("HTTP %d %s, want %s", response.StatusCode, b, want)
		}
	}
	run("start", source, "--http", "--php-version", "8.5", "--phpmyadmin=false")
	owner, err := supervisor.Status(home, id)
	if err != nil || owner.Status != "running" {
		t.Fatal(owner, err)
	}
	get(owner.URL, "my_database_app:empty")
	get(owner.URL+"/?write=1", "my_database_app:7")
	if owner.PID <= 0 || owner.PID == os.Getpid() {
		t.Fatal("invalid disposable test owner")
	}
	process, err := os.FindProcess(owner.PID)
	if err != nil {
		t.Fatal(err)
	}
	if err = process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		state, err := supervisor.Status(home, id)
		transport := &http.Transport{Proxy: nil}
		response, requestErr := (&http.Client{Transport: transport, Timeout: time.Second}).Get(owner.URL)
		if response != nil {
			response.Body.Close()
		}
		transport.CloseIdleConnections()
		if err == nil && state.Status == "crashed" && requestErr != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("web process survived owner crash")
		}
		time.Sleep(50 * time.Millisecond)
	}
	run("start", id)
	recovered, err := supervisor.Status(home, id)
	if err != nil {
		t.Fatal(err)
	}
	get(recovered.URL, "my_database_app:7")
	run("restart", id)
	restarted, err := supervisor.Status(home, id)
	if err != nil {
		t.Fatal(err)
	}
	get(restarted.URL, "my_database_app:7")
	run("stop", id)
	t.Log("Real MySQL app-named empty schema, explicit probe write, owner crash, verified stale-lock recovery, restart, and persistent data passed")
}

// Only runtime program files are linked; database state is always fresh and
// private. Atomic repairs replace destination links without modifying sources.
func cloneRuntime(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, destination)
		}
		if err = os.Link(path, destination); err == nil {
			return nil
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		info, err := entry.Info()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		closeErr := output.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
