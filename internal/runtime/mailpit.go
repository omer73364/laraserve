package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"time"

	"laraserve/internal/statefile"
)

// EnsureMailpit fetches a publisher release on first use and verifies its digest.
// The cached release selection allows offline starts without bundling payloads.
func EnsureMailpit(root string) (string, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	metadata := filepath.Join(root, "mailpit-release.json")
	b, readErr := os.ReadFile(metadata)
	info, _ := os.Stat(metadata)
	if readErr != nil || info == nil || time.Since(info.ModTime()) > 24*time.Hour {
		fresh, err := checksumMetadata("https://api.github.com/repos/axllent/mailpit/releases/latest")
		if err != nil {
			if readErr != nil {
				return "", err
			}
		} else {
			var check struct {
				Tag string `json:"tag_name"`
			}
			if json.Unmarshal(fresh, &check) != nil || !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(check.Tag) {
				return "", fmt.Errorf("invalid Mailpit release metadata")
			}
			if err = statefile.WriteBytes(metadata, fresh); err != nil {
				return "", err
			}
			b = fresh
		}
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.Unmarshal(b, &release); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(release.Tag) {
		return "", fmt.Errorf("invalid Mailpit release version")
	}
	archiveName := "mailpit-" + goruntime.GOOS + "-" + goruntime.GOARCH + ".tar.gz"
	binary := "mailpit"
	if goruntime.GOOS == "windows" {
		archiveName = "mailpit-windows-" + goruntime.GOARCH + ".zip"
		binary += ".exe"
	}
	source, err := sourceOverride("MAILPIT", downloadSource{"https://github.com/axllent/mailpit/releases/download/" + release.Tag + "/" + archiveName, "publisher"})
	if err != nil {
		return "", err
	}
	validate := func(dir string) error {
		info, err := os.Stat(filepath.Join(dir, binary))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("missing Mailpit executable")
		}
		return nil
	}
	dir, err := installCache(root, cacheKey("mailpit-"+release.Tag+"-"+goruntime.GOOS+"-"+goruntime.GOARCH, source), validate, func(dir string) error {
		archive := filepath.Join(dir, ".archive")
		if err := downloadFile(source, archive, "Mailpit"); err != nil {
			return err
		}
		var err error
		if goruntime.GOOS == "windows" {
			err = extractZip(archive, dir, false)
		} else {
			err = extractRuntimeArchive(archive, dir, false)
		}
		if err != nil {
			return err
		}
		return os.Remove(archive)
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, binary), nil
}
