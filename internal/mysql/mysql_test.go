package mysql

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("LARASERVE_MYSQL_TEST_HELPER") == "1" {
		fakeMySQL()
		return
	}
	os.Exit(m.Run())
}

func fakeMySQL() {
	dir := os.Getenv("MYSQL_TEST_DIR")
	if filepath.Base(os.Args[0]) == "mysql" {
		if _, err := os.Stat(filepath.Join(dir, "running")); err != nil {
			os.Exit(1)
		}
		sql, _ := io.ReadAll(os.Stdin)
		if strings.Contains(string(sql), "CREATE DATABASE") {
			_ = os.WriteFile(filepath.Join(dir, "provisioned"), []byte("yes"), 0o600)
		}
		os.Exit(0)
	}
	for _, arg := range os.Args[1:] {
		if arg == "--initialize-insecure" {
			f, _ := os.OpenFile(filepath.Join(dir, "initialized-count"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			_, _ = f.WriteString("init\n")
			_ = f.Close()
			os.Exit(0)
		}
	}
	if os.Getenv("MYSQL_TEST_CRASH") == "1" {
		os.Exit(9)
	}
	_ = os.WriteFile(filepath.Join(dir, "server-args"), []byte(strings.Join(os.Args[1:], "\n")), 0o600)
	// Register before advertising readiness so cancellation cannot race startup.
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)
	_ = os.WriteFile(filepath.Join(dir, "running"), []byte("yes"), 0o600)
	<-ch
	_ = os.Remove(filepath.Join(dir, "running"))
	os.Exit(0)
}

func fakeRuntime(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("managed MySQL uses Unix sockets")
	}
	dir := t.TempDir()
	oldQuery := queryDatabase
	queryDatabase = func(ctx context.Context, base, socket string, env []string, statement string) error {
		if _, err := os.Stat(filepath.Join(dir, "running")); err != nil {
			return err
		}
		if strings.Contains(statement, "CREATE DATABASE") {
			return os.WriteFile(filepath.Join(dir, "provisioned"), []byte(statement), 0600)
		}
		return nil
	}
	t.Cleanup(func() { queryDatabase = oldQuery })
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"mysql", "mysqld"} {
		if err := os.WriteFile(filepath.Join(bin, name), b, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return dir, append(os.Environ(), "LARASERVE_MYSQL_TEST_HELPER=1", "MYSQL_TEST_DIR="+dir)
}

func TestManagedLifecycle(t *testing.T) {
	base, env := fakeRuntime(t)
	state := filepath.Join(base, "state")
	start := func() *Server {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		t.Cleanup(cancel)
		s, err := Start(ctx, base, state, env, "my-app")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		return s
	}
	s := start()
	if s.Env["DB_DATABASE"] != "my_app" {
		t.Fatal(s.Env)
	}
	provisioned, err := os.ReadFile(filepath.Join(base, "provisioned"))
	if err != nil || !strings.Contains(string(provisioned), "CREATE DATABASE IF NOT EXISTS `my_app`") || !strings.Contains(string(provisioned), "GRANT ALL PRIVILEGES ON `my_app`.*") {
		t.Fatalf("incorrect provisioning: %s %v", provisioned, err)
	}
	password := s.Env["DB_PASSWORD"]
	if len(password) != 64 || s.Env["DB_CONNECTION"] != "mysql" || s.Env["DB_SOCKET"] != s.Socket {
		t.Fatalf("invalid app connection configuration")
	}
	if _, err := Start(context.Background(), base, state, env); err == nil {
		t.Fatal("second server accepted the same data directory")
	}
	args, _ := os.ReadFile(filepath.Join(base, "server-args"))
	for _, required := range []string{"--no-defaults", "--skip-networking", "--mysqlx=OFF"} {
		if !strings.Contains(string(args), required) {
			t.Fatalf("missing isolation flag %s", required)
		}
	}
	if _, err := os.Stat(filepath.Join(base, "provisioned")); err != nil {
		t.Fatal("database was not provisioned")
	}
	s.Close()
	if _, err := os.Stat(filepath.Dir(s.Socket)); !os.IsNotExist(err) {
		t.Fatal("socket directory was not cleaned")
	}
	s = start()
	if s.Env["DB_PASSWORD"] != password {
		t.Fatal("credentials changed across restart")
	}
	s.Close()
	b, _ := os.ReadFile(filepath.Join(base, "initialized-count"))
	if string(b) != "init\n" {
		t.Fatal("existing database was reinitialized")
	}
}

func TestEarlyServerExit(t *testing.T) {
	base, env := fakeRuntime(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state := filepath.Join(base, "state")
	_, err := Start(ctx, base, state, append(env, "MYSQL_TEST_CRASH=1"))
	if err == nil || !strings.Contains(err.Error(), "before becoming ready") {
		t.Fatalf("expected startup failure, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "running.lock")); !os.IsNotExist(err) {
		t.Fatal("failed startup left a lock")
	}
}

// Run against a downloaded generic MySQL distribution when available.
func TestRealMySQL(t *testing.T) {
	base := os.Getenv("LARASERVE_TEST_MYSQL_DIR")
	if base == "" {
		t.Skip("set LARASERVE_TEST_MYSQL_DIR for the real MySQL integration test")
	}
	state := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		s, err := Start(ctx, base, state, os.Environ())
		if err != nil {
			if log, readErr := os.ReadFile(filepath.Join(state, "server.log")); readErr == nil {
				t.Logf("MySQL server log: %s", log)
			}
			t.Fatal(err)
		}
		t.Cleanup(s.Close)
		sql := "CREATE TABLE IF NOT EXISTS persistence_test (id INT PRIMARY KEY); INSERT IGNORE INTO persistence_test VALUES (42); SELECT id FROM persistence_test;"
		if i == 1 {
			sql = "SELECT id FROM persistence_test;"
		}
		cmd := exec.CommandContext(ctx, filepath.Join(base, "bin", "mysql"), "--no-defaults", "--no-login-paths",
			"--protocol=SOCKET", "--socket="+s.Socket, "--user=laraserve", "--database=laraserve", "--batch", "--skip-column-names")
		cmd.Env = append(os.Environ(), "MYSQL_PWD="+s.Env["DB_PASSWORD"])
		cmd.Stdin = strings.NewReader(sql)
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "42" {
			t.Fatalf("MySQL app-user persistence query failed: %v %s", err, out)
		}
		s.Close()
	}
}
