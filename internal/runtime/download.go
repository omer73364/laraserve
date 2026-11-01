package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	h := sha256.New()
	p := &downloadProgress{label: label, total: resp.ContentLength, last: time.Now()}
	n, copyErr := io.Copy(io.MultiWriter(f, h, p), io.LimitReader(resp.Body, 2<<30))
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
	if hex.EncodeToString(h.Sum(nil)) != source.SHA256 {
		return fmt.Errorf("checksum mismatch downloading %s; retry or check the configured source", label)
	}
	ui.OK("%s downloaded and verified (%.1f MB)", label, float64(n)/(1024*1024))
	return nil
}

type downloadProgress struct {
	label    string
	total, n int64
	last     time.Time
}

func (p *downloadProgress) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if time.Since(p.last) >= 5*time.Second {
		if p.total > 0 {
			ui.Info("%s: %.0f%%", p.label, float64(p.n)*100/float64(p.total))
		} else {
			ui.Info("%s: %.1f MB", p.label, float64(p.n)/(1024*1024))
		}
		p.last = time.Now()
	}
	return len(b), nil
}
