package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"laraserve/internal/ui"
)

type downloadSource struct{ URL, SHA256 string }

var downloadClient = &http.Client{Timeout: 20 * time.Minute}

func sourceOverride(kind string, source downloadSource) (downloadSource, error) {
	if raw := os.Getenv("LARASERVE_" + kind + "_URL"); raw != "" {
		source = downloadSource{raw, os.Getenv("LARASERVE_" + kind + "_SHA256")}
	}
	u, err := url.Parse(source.URL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return source, fmt.Errorf("%s download URL must use HTTPS (HTTP is allowed for localhost mirrors)", kind)
	}
	if source.SHA256 == "publisher" {
		return source, nil
	}
	b, err := hex.DecodeString(source.SHA256)
	if err != nil || len(b) != 32 {
		return source, fmt.Errorf("%s download requires a valid LARASERVE_%s_SHA256", kind, kind)
	}
	source.SHA256 = strings.ToLower(source.SHA256)
	return source, nil
}

func cacheKey(name string, source downloadSource) string {
	h := sha256.Sum256([]byte(source.URL + "\n" + source.SHA256))
	return name + "-" + hex.EncodeToString(h[:])[:20]
}

// installCache publishes only complete installations and serializes concurrent
// clients. OS locks are released automatically if a client is interrupted.
func installCache(root, key string, validate func(string) error, populate func(string) error) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	unlock, err := lockDownload(filepath.Join(root, key+".lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	dir := filepath.Join(root, key)
	if _, err = os.Stat(filepath.Join(dir, ".ready")); err == nil && validate(dir) == nil {
		return dir, nil
	}
	// Only this key's abandoned staging directories are removed, under its lock.
	stale, _ := filepath.Glob(filepath.Join(root, "."+key+"-download-*"))
	for _, p := range stale {
		_ = os.RemoveAll(p)
	}
	tmp, err := os.MkdirTemp(root, "."+key+"-download-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err = populate(tmp); err != nil {
		return "", err
	}
	if err = validate(tmp); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(tmp, ".ready"), []byte("ready\n"), 0600); err != nil {
		return "", err
	}
	if err = os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err = os.Rename(tmp, dir); err != nil {
		return "", err
	}
	return dir, nil
}

func downloadFile(source downloadSource, dst, label string) error {
	h := sha256.New()
	var verifier hash.Hash = h
	expected := source.SHA256
	if expected == "publisher" {
		var err error
		verifier, expected, err = publisherChecksum(source.URL)
		if err != nil {
			return fmt.Errorf("verify %s: %w", label, err)
		}
	}
	ui.Info("Downloading %s (first use)", label)
	resp, err := downloadClient.Get(source.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w; connect to the internet and retry", label, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", label, resp.StatusCode)
	}
	// Redirects must retain transport security for public sources.
	if strings.HasPrefix(source.URL, "https://") && resp.Request.URL.Scheme != "https" {
		return fmt.Errorf("insecure redirect downloading %s", label)
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	p := ui.NewProgress(label, resp.ContentLength)
	defer p.Finish()
	n, copyErr := io.Copy(io.MultiWriter(f, verifier, p), io.LimitReader(resp.Body, 2<<30))
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download %s: %w", label, copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return fmt.Errorf("incomplete download of %s", label)
	}
	if hex.EncodeToString(verifier.Sum(nil)) != expected {
		return fmt.Errorf("checksum mismatch downloading %s; retry or check the configured source", label)
	}
	p.Finish()
	ui.OK("%s downloaded and verified (%s)", label, ui.Bytes(n))
	return nil
}
