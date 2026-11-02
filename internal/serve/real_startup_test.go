package serve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
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

func TestRealCaddyStartupAndTrustIsolation(t *testing.T) {
	binary := os.Getenv("LARASERVE_TEST_FRANKENPHP")
	if binary == "" {
		t.Skip("set LARASERVE_TEST_FRANKENPHP for real Caddy startup verification")
	}
	root, state := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "public"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "public", "index.php"), []byte("<?php echo 'laraserve-startup-test';"), 0644); err != nil {
		t.Fatal(err)
	}
	issuer := httptest.NewTLSServer(http.NotFoundHandler())
	pair := issuer.TLS.Certificates[0]
	issuer.Close()
	cert, key := filepath.Join(state, "leaf.pem"), filepath.Join(state, "leaf.key")
	keyDER, err := x509.MarshalPKCS8PrivateKey(pair.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: pair.Certificate[0]}), 0600)
	os.WriteFile(key, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ready := make(chan struct{}, 1)
	done := make(chan error, 1)
	env := []string{"PATH=/nonexistent", "HOME=" + state, "XDG_DATA_HOME=" + filepath.Join(state, "data"), "XDG_CONFIG_HOME=" + filepath.Join(state, "config"), "PHP_INI_SCAN_DIR="}
	c := Config{NativePHP: os.Getenv("LARASERVE_TEST_NATIVE_PHP"), PHP: binary, Root: root, StateDir: state, Host: "127.0.0.1", Domain: "example.com", Port: port, HTTPS: true, Cert: cert, Key: key, Context: ctx, Env: env}
	go func() { done <- Run(c, func() { ready <- struct{}{} }) }()
	select {
	case err := <-done:
		t.Fatalf("server exited before ready: %v", err)
	case <-ctx.Done():
		t.Fatal("server never became ready")
	case <-ready:
	}
	roots := x509.NewCertPool()
	roots.AddCert(issuer.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	}}
	defer transport.CloseIdleConnections()
	response, err := (&http.Client{Transport: transport, Timeout: 2 * time.Second}).Get(c.URL() + "/")
	if err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	data, _ := io.ReadAll(response.Body)
	response.Body.Close()
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if string(data) != "laraserve-startup-test" {
		t.Fatalf("wrong application: %s", data)
	}
	logs, err := os.ReadFile(filepath.Join(state, "logs", "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(logs), "\n") {
		var event struct{ Msg string }
		if json.Unmarshal([]byte(line), &event) == nil && (event.Msg == "installing root certificate (you might be prompted for password)" || event.Msg == "failed to install root certificate") {
			t.Fatalf("Caddy attempted unmanaged trust installation: %s", line)
		}
	}
	t.Log("Own-process HTTPS readiness, actual PHP request, clean shutdown, and no trust installation passed")
}
