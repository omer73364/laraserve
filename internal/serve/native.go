package serve

import (
	"context"
	"fmt"
	"io"
	"laraserve/internal/process"
	"net"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// Native PHP serves only a loopback backend; Caddy remains the public TLS server.
func startNative(ctx context.Context, c Config, root string, log io.Writer, stop context.CancelFunc) (int, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	router, err := os.CreateTemp(c.StateDir, "native-router-*.php")
	if err != nil {
		return 0, nil, err
	}
	path := router.Name()
	defer router.Close()
	// Requests come exclusively from the local Caddy proxy. Set HTTPS before
	// including PHP so Laravel and phpMyAdmin see the original request scheme.
	code := `<?php
 if (($_SERVER['HTTP_X_FORWARDED_PROTO'] ?? '') === 'https') {
   $_SERVER['HTTPS'] = 'on'; $_SERVER['REQUEST_SCHEME'] = 'https';
   $_SERVER['SERVER_PORT'] = (string) (parse_url('https://' . ($_SERVER['HTTP_X_FORWARDED_HOST'] ?? $_SERVER['HTTP_HOST']), PHP_URL_PORT) ?: 443);
 }
 $root = $_SERVER['DOCUMENT_ROOT'];
 $uri = rawurldecode(parse_url($_SERVER['REQUEST_URI'], PHP_URL_PATH) ?: '/');
 if (strpos($uri, '\\') !== false || (DIRECTORY_SEPARATOR === '\\' && strpos($uri, ':') !== false) || in_array('..', explode('/', $uri), true)) { http_response_code(404); return; }
 $file = realpath($root . $uri);
 // Public storage links are intentionally allowed, just like Laravel's router.
 if ($file && is_file($file)) {
   if (substr($file, -4) === '.php') { require $file; return; }
   return false;
 }
 require $root . '/index.php';
`
	if _, err = router.WriteString(code); err != nil {
		os.Remove(path)
		return 0, nil, err
	}
	if err = router.Close(); err != nil {
		os.Remove(path)
		return 0, nil, err
	}
	cmd := exec.CommandContext(ctx, c.NativePHP, "-S", "127.0.0.1:"+strconv.Itoa(port), "-t", root, path)
	cmd.Dir = root
	cmd.Env = c.Env
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Cancel = func() error { return process.Stop(cmd) }
	cmd.WaitDelay = 5 * time.Second
	if err = cmd.Start(); err != nil {
		os.Remove(path)
		return 0, nil, err
	}
	done := make(chan struct{})
	var exitErr error
	go func() {
		exitErr = cmd.Wait()
		close(done)
		if ctx.Err() == nil {
			stop()
		}
	}()
	cleanup := func() { <-done; os.Remove(path) }
	for i := 0; i < 100; i++ {
		select {
		case <-done:
			cleanup()
			return 0, nil, fmt.Errorf("PHP backend exited: %v", exitErr)
		default:
		}
		if conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 100*time.Millisecond); err == nil {
			conn.Close()
			return port, cleanup, nil
		}
		select {
		case <-ctx.Done():
			cleanup()
			return 0, nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	stop()
	cleanup()
	return 0, nil, fmt.Errorf("PHP backend did not start")
}

func nativeProxy(port int) string { return "\treverse_proxy 127.0.0.1:" + strconv.Itoa(port) + "\n" }
