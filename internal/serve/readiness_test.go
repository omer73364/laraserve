package serve

import (
	"context"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestOccupiedPortNeverAnnouncesReadiness(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("socket unavailable: %v", err)
	}
	defer listener.Close()
	ready := false
	err = Run(Config{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, PHP: "/does-not-exist", StateDir: t.TempDir()}, func() { ready = true })
	if ready || err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestReadinessRejectsAnotherHTTPServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("unrelated server")) }))
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	err := waitReady(ctx, Config{Host: "127.0.0.1", Domain: "localhost", Port: port, readinessToken: "specific-launch"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unrelated server was accepted: %v", err)
	}
}

func TestReadinessValidatesLaunchAndTLSCertificate(t *testing.T) {
	const token = "unique-launch-token"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.__laraserve_ready_"+token {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(token))
	})
	server := httptest.NewTLSServer(handler)
	defer server.Close()
	cert := filepath.Join(t.TempDir(), "leaf.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	c := Config{Host: "127.0.0.1", Domain: "example.com", Port: server.Listener.Addr().(*net.TCPAddr).Port, HTTPS: true, Cert: cert, readinessToken: token}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitReady(ctx, c); err != nil {
		t.Fatal(err)
	}
}

func TestReadinessRouteAndExplicitTrustConfiguration(t *testing.T) {
	out := Caddyfile(Config{Root: "/project", Domain: "localhost", Host: "127.0.0.1", Port: 8080, readinessToken: "random-token"})
	for _, want := range []string{"pki {", "ca local {", "skip_install_trust", `respond /.__laraserve_ready_random-token "random-token" 200`} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s", want)
		}
	}
}

func TestServerFailureExplainsPortConflict(t *testing.T) {
	file := filepath.Join(t.TempDir(), "server.log")
	os.WriteFile(file, []byte(`{"level":"error","msg":"listening on 127.0.0.1:443: bind: address already in use"}`), 0600)
	err := serverFailure(Config{Port: 443}, errors.New("exit status 1"), file)
	if !strings.Contains(err.Error(), "--port") || !strings.Contains(err.Error(), file) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), `"level"`) {
		t.Fatal("raw JSON hides the actionable error")
	}
}

func TestFreePortSkipsOccupiedListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	selected, err := FreePort("127.0.0.1", port, false)
	if err != nil || selected == port {
		t.Fatalf("selected=%d occupied=%s err=%v", selected, strconv.Itoa(port), err)
	}
}
