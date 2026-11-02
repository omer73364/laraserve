package serve

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestRealNativePHPBackend(t *testing.T) {
	php := os.Getenv("LARASERVE_TEST_NATIVE_PHP")
	if php == "" {
		t.Skip("set LARASERVE_TEST_NATIVE_PHP to exercise the selected PHP backend")
	}
	root := t.TempDir()
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.php"), []byte("<?php echo PHP_VERSION . ':' . ($_SERVER['HTTPS'] ?? 'off');"), 0600); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(t.TempDir(), "asset.txt")
	if err := os.WriteFile(asset, []byte("public-storage"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(asset, filepath.Join(root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port, cleanup, err := startNative(ctx, Config{NativePHP: php, StateDir: state, Env: []string{"PHP_INI_SCAN_DIR="}}, root, io.Discard, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); cleanup() }()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	resp, err := http.Get(base + "/linked.txt")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "public-storage" {
		t.Fatalf("public storage link failed: %s", b)
	}
	for _, path := range []string{"/%5c..%5c.env", "/..%5c.env"} {
		response, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("unsafe backslash path %s: HTTP %d", path, response.StatusCode)
		}
	}
	req, _ := http.NewRequest("GET", base+"/index.php", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "localhost:8443")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.HasSuffix(string(b), ":on") {
		t.Fatalf("HTTPS request context lost: %s", b)
	}
}
