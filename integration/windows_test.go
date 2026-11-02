//go:build windows

package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laraserve/internal/certs"
	"laraserve/internal/env"
	"laraserve/internal/mysql"
	"laraserve/internal/prepare"
	"laraserve/internal/process"
	"laraserve/internal/runtime"
	"laraserve/internal/serve"
)

// Uses real vendor downloads, but never changes hosts or OS certificate stores.
func TestWindowsLaravelWorkflow(t *testing.T) {
	if os.Getenv("LARASERVE_TEST_WINDOWS") != "1" {
		t.Skip("set LARASERVE_TEST_WINDOWS=1 for real Windows runtime integration")
	}
	base := filepath.Join(t.TempDir(), "project with spaces")
	cache := os.Getenv("LARASERVE_TEST_CACHE")
	if cache == "" {
		cache = filepath.Join(t.TempDir(), "runtimes")
	}
	rt, err := runtime.EnsurePHP(cache, "8.3.36")
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.EnsureComposer(rt); err != nil {
		t.Fatal(err)
	}
	state, err := prepare.EnsureState(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatal(err)
	}
	environment := env.Build(rt, state.Dir, false, nil)
	if err = prepare.ComposerCreateProject(rt.PHP, rt.Composer, base, "^12.0", environment); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(base, ".env")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(base, "database", "database.sqlite")); !os.IsNotExist(err) {
		t.Fatal("creation hook created a database")
	}
	// Composer @php scripts must execute native php.exe without a Unix wrapper.
	cmd := process.PHP(rt.PHP, rt.Composer, "run-script", "post-autoload-dump", "--no-interaction")
	cmd.Dir = base
	cmd.Env = environment
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Composer scripts: %v %s", err, out)
	}
	mysqlBase, err := runtime.EnsureMySQL(cache)
	if err != nil {
		t.Fatal(err)
	}
	dbCtx, stopDB := context.WithTimeout(context.Background(), 3*time.Minute)
	defer stopDB()
	db, err := mysql.Start(dbCtx, mysqlBase, filepath.Join(t.TempDir(), "mysql state"), environment, "my-app")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Env["DB_HOST"] != "127.0.0.1" || db.Env["DB_SOCKET"] != "" || db.Env["DB_DATABASE"] != "my_app" {
		t.Fatal(db.Env)
	}
	appEnv := state.AppEnv(base, "https://localhost")
	for k, v := range db.Env {
		appEnv[k] = v
	}
	environment = env.Build(rt, state.Dir, false, appEnv)
	check := process.PHP(rt.PHP, "-r", `$p=new PDO('mysql:host='.getenv('DB_HOST').';port='.getenv('DB_PORT').';dbname='.getenv('DB_DATABASE'),getenv('DB_USERNAME'),getenv('DB_PASSWORD')); echo count($p->query('SHOW TABLES')->fetchAll());`)
	check.Env = environment
	if out, err := check.CombinedOutput(); err != nil || string(out) != "0" {
		t.Fatalf("unexpected initial schema: %s %v", out, err)
	}
	if err = state.EnsureMigrations(rt.PHP, base, environment, true); err != nil {
		t.Fatal(err)
	}
	ca, _, err := certs.EnsureCA(filepath.Join(t.TempDir(), "ca"), certs.DefaultPermitted)
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := certs.IssueLeaf(ca, filepath.Dir(ca.PEMPath), "localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ready, done := make(chan struct{}, 1), make(chan error, 1)
	config := serve.Config{PHP: rt.ServerPHP, NativePHP: rt.NativeCLI, Root: base, StateDir: state.Dir, Env: environment, AppEnv: appEnv, Domain: "localhost", Host: "127.0.0.1", Port: port, HTTPS: true, Cert: cert, Key: key, Context: ctx}
	go func() { done <- serve.Run(config, func() { ready <- struct{}{} }) }()
	// Ensure all child processes stop even if a request assertion fails.
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(15 * time.Second):
			t.Error("web processes did not stop")
		}
	}()
	select {
	case err := <-done:
		done <- err
		t.Fatalf("web startup: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-ready:
	}
	pem, err := os.ReadFile(ca.PEMPath)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	resp, err := (&http.Client{Transport: transport, Timeout: 10 * time.Second}).Get(config.URL())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Laravel") {
		t.Fatalf("Laravel response: %d %s", resp.StatusCode, body)
	}
	for _, path := range []string{"/%5c..%5c.env", "/..%5c.env"} {
		response, err := (&http.Client{Transport: transport, Timeout: 10 * time.Second}).Get(config.URL() + path)
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode == 200 || strings.Contains(string(data), "APP_NAME=") {
			t.Fatalf("unsafe Windows path served: %s", path)
		}
	}
	db.Close()
	db, err = mysql.Start(dbCtx, mysqlBase, db.Dir, env.Build(rt, state.Dir, false, nil), "renamed-app")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Env["DB_DATABASE"] != "my_app" {
		t.Fatal("database name changed on restart")
	}
	t.Log("Native PHP, Composer hooks, empty initial schema, explicit migrations, HTTPS, MySQL restart and cleanup passed")
}
