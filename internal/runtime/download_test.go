package runtime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func digest(b []byte) string                                              { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func TestLazyDownloadsAndOfflineCache(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Unix FrankenPHP fixture; Windows downloads covered by integration tests")
	}
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	calls := map[string]int{}
	files := map[string][]byte{"/php": []byte("#!/bin/sh\nexit 0\n"), "/composer": []byte("composer fixture")}
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Path]++
		b, ok := files[r.URL.Path]
		if !ok {
			return nil, fmt.Errorf("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: r}, nil
	})}
	t.Setenv("LARASERVE_FRANKENPHP", "")
	for kind, path := range map[string]string{"PHP": "/php", "COMPOSER": "/composer"} {
		t.Setenv("LARASERVE_"+kind+"_URL", "http://localhost"+path)
		t.Setenv("LARASERVE_"+kind+"_SHA256", digest(files[path]))
	}
	root := t.TempDir()
	rt, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if calls["/php"] != 1 || calls["/composer"] != 0 {
		t.Fatal("Composer was not lazy")
	}
	if err = EnsureComposer(rt); err != nil {
		t.Fatal(err)
	}
	if calls["/composer"] != 1 {
		t.Fatal("Composer was not downloaded")
	}
	// Every subsequent install must work with the network unavailable.
	files = map[string][]byte{}
	again, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = EnsureComposer(again); err != nil {
		t.Fatal(err)
	}
	if calls["/php"] != 1 || calls["/composer"] != 1 {
		t.Fatal("warm cache contacted network")
	}
}

func TestFailedDownloadNeverPublishesCache(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("wrong content")), ContentLength: 13, Request: r}, nil
	})}
	source := downloadSource{"https://example.test/tool", digest([]byte("expected"))}
	root := t.TempDir()
	_, err := installCache(root, "tool", func(d string) error { _, err := os.Stat(filepath.Join(d, "binary")); return err }, func(d string) error { return downloadFile(source, filepath.Join(d, "binary"), "test runtime") })
	if err == nil {
		t.Fatal("accepted corrupt download")
	}
	if _, err = os.Stat(filepath.Join(root, "tool", ".ready")); !os.IsNotExist(err) {
		t.Fatal("failed download was marked ready")
	}
	entries, _ := filepath.Glob(filepath.Join(root, ".tool-download-*"))
	if len(entries) != 0 {
		t.Fatal("partial download was left behind")
	}
}

func TestSourceRequiresChecksummedHTTPS(t *testing.T) {
	t.Setenv("LARASERVE_TEST_URL", "http://example.com/tool")
	t.Setenv("LARASERVE_TEST_SHA256", digest([]byte("test")))
	if _, err := sourceOverride("TEST", downloadSource{}); err == nil {
		t.Fatal("accepted public HTTP")
	}
	t.Setenv("LARASERVE_TEST_URL", "https://example.com/tool")
	t.Setenv("LARASERVE_TEST_SHA256", "")
	if _, err := sourceOverride("TEST", downloadSource{}); err == nil {
		t.Fatal("accepted missing checksum")
	}
}
