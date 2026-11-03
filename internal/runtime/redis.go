package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"laraserve/internal/process"
	"laraserve/internal/statefile"
)

// EnsureRedis prefers an explicitly supplied or installed server. On Linux amd64
// it can privately extract Ubuntu's maintained packages; no package manager,
// maintainer scripts, root access or system service is involved.
func EnsureRedis(ctx context.Context, root string) (string, error) {
	if binary := os.Getenv("LARASERVE_REDIS"); binary != "" {
		return filepath.Abs(binary)
	}
	if binary, err := exec.LookPath("redis-server"); err == nil {
		return filepath.Abs(binary)
	}
	if goruntime.GOOS != "linux" || goruntime.GOARCH != "amd64" {
		return "", fmt.Errorf("automatic Redis installation supports Linux amd64; install redis-server or set LARASERVE_REDIS, or leave private Redis disabled to use an external connection")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	metadata := filepath.Join(root, "redis-packages-v1.json")
	var packages []legacyPackage
	b, readErr := os.ReadFile(metadata)
	info, _ := os.Stat(metadata)
	if readErr != nil || info == nil || time.Since(info.ModTime()) > 24*time.Hour {
		var fresh []legacyPackage
		var err error
		for _, name := range []string{"redis-server", "redis-tools", "libjemalloc2", "liblzf1", "liblua5.1-0", "lua-cjson", "lua-bitop", "libatomic1", "libssl3", "libsystemd0", "libgcrypt20", "libgpg-error0", "liblz4-1", "liblzma5", "libzstd1", "libcap2"} {
			var pkg legacyPackage
			pkg, err = ubuntuRedisPackage(name)
			if err != nil {
				break
			}
			fresh = append(fresh, pkg)
		}
		if err != nil {
			if readErr != nil {
				return "", err
			}
		} else {
			b, _ = json.Marshal(fresh)
			if err = statefile.WriteBytes(metadata, b); err != nil {
				return "", err
			}
		}
	}
	if err := json.Unmarshal(b, &packages); err != nil {
		return "", err
	}
	if len(packages) == 0 {
		return "", fmt.Errorf("empty Redis package selection")
	}
	sum := sha256.Sum256(b)
	key := fmt.Sprintf("redis-linux-amd64-%x", sum[:10])
	validate := func(dir string) error {
		cmd := exec.CommandContext(ctx, filepath.Join(dir, "bin", "redis-server"), "--version")
		out, err := process.CombinedOutputOwned(cmd)
		if err != nil {
			return fmt.Errorf("private Redis needs glibc 2.35+ and working support libraries: %w: %s", err, out)
		}
		if !strings.Contains(string(out), "Redis server v=") {
			return fmt.Errorf("invalid Redis executable")
		}
		return nil
	}
	dir, err := installCache(root, key, validate, func(dir string) error {
		for _, pkg := range packages {
			archive := filepath.Join(dir, ".package.deb")
			if err := downloadFile(pkg.Source, archive, "Redis: "+pkg.Name); err != nil {
				return err
			}
			if err := extractDebData(archive, dir); err != nil {
				return err
			}
			if err := os.Remove(archive); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0700); err != nil {
			return err
		}
		// Debian provides the server name as a symlink to the shared executable.
		// The library-only extractor intentionally skips non-library links.
		if err := os.Symlink("redis-check-rdb", filepath.Join(dir, "usr", "bin", "redis-server")); err != nil {
			return err
		}
		script := `#!/bin/sh
root=${0%/bin/redis-server}
export LD_LIBRARY_PATH="$root/usr/lib/x86_64-linux-gnu:$root/lib/x86_64-linux-gnu"
exec "$root/usr/bin/redis-server" "$@"
`
		return os.WriteFile(filepath.Join(dir, "bin", "redis-server"), []byte(script), 0700)
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "bin", "redis-server"), nil
}
func ubuntuRedisPackage(name string) (legacyPackage, error) {
	for _, suite := range []string{"jammy-updates", "jammy-security", "jammy"} {
		for _, component := range []string{"main", "universe"} {
			endpoint := "https://archive.ubuntu.com/ubuntu/dists/" + suite + "/" + component + "/binary-amd64/Packages.gz"
			entries, err := readPackageIndex(endpoint)
			if err != nil {
				return legacyPackage{}, err
			}
			var matches []string
			for file := range entries {
				if strings.HasPrefix(path.Base(file), name+"_") && strings.HasSuffix(file, "_amd64.deb") {
					matches = append(matches, file)
				}
			}
			sort.Strings(matches)
			if len(matches) > 0 {
				file := matches[len(matches)-1]
				return legacyPackage{Name: name, Source: downloadSource{"https://archive.ubuntu.com/ubuntu/" + file, entries[file]}}, nil
			}
		}
	}
	return legacyPackage{}, fmt.Errorf("Ubuntu's package index does not contain %s", name)
}
