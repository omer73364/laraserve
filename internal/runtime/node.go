package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"laraserve/internal/statefile"
)

type NodeRuntime struct{ Dir, Node, NPM, Version string }

// NodeVersions uses the publisher's release index, retaining it for offline
// restarts. Only stable LTS lines with an archive for this platform are selected.
func NodeVersions(root string) ([]string, error) {
	file := filepath.Join(root, "node-releases.json")
	b, readErr := os.ReadFile(file)
	info, _ := os.Stat(file)
	if readErr != nil || info == nil || time.Since(info.ModTime()) > 24*time.Hour {
		fresh, e := checksumMetadata("https://nodejs.org/dist/index.json")
		if e == nil {
			if _, e = nodeVersions(fresh); e != nil {
				return nil, e
			}
			if e = statefile.WriteBytes(file, fresh); e != nil {
				return nil, e
			}
			b = fresh
		} else if readErr != nil {
			return nil, e
		}
	}
	return nodeVersions(b)
}
func nodeVersions(b []byte) ([]string, error) {
	var releases []struct {
		Version string
		LTS     json.RawMessage
		Files   []string
	}
	if err := json.Unmarshal(b, &releases); err != nil {
		return nil, err
	}
	platform := goruntime.GOOS + "-" + nodeArch()
	switch goruntime.GOOS {
	case "darwin":
		platform = "osx-" + nodeArch() + "-tar"
	case "windows":
		platform = "win-" + nodeArch() + "-zip"
	}
	seen := map[uint64]bool{}
	var versions []string
	for _, r := range releases {
		v, e := semver.NewVersion(r.Version)
		if e != nil || v.Prerelease() != "" || string(r.LTS) == "false" || len(r.LTS) == 0 {
			continue
		}
		supported := false
		for _, file := range r.Files {
			if file == platform {
				supported = true
			}
		}
		if !supported || seen[v.Major()] {
			continue
		}
		seen[v.Major()] = true
		versions = append(versions, v.String())
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no Node LTS distributions for %s/%s", goruntime.GOOS, goruntime.GOARCH)
	}
	return versions, nil
}
func nodeArch() string {
	if goruntime.GOARCH == "amd64" {
		return "x64"
	}
	return goruntime.GOARCH
}
func EnsureNode(root, version string) (*NodeRuntime, error) {
	v, e := semver.NewVersion(version)
	if e != nil || v.String() != version || v.Prerelease() != "" {
		return nil, fmt.Errorf("invalid exact Node version %q", version)
	}
	platform := goruntime.GOOS
	if platform == "windows" {
		platform = "win"
	}
	extension := ".tar.gz"
	if platform == "win" {
		extension = ".zip"
	}
	name := "node-v" + version + "-" + platform + "-" + nodeArch() + extension
	source, e := sourceOverride("NODE", downloadSource{"https://nodejs.org/dist/v" + version + "/" + name, "publisher"})
	if e != nil {
		return nil, e
	}
	layout := func(dir string) *NodeRuntime {
		r := &NodeRuntime{Dir: dir, Version: version, Node: filepath.Join(dir, "bin", "node"), NPM: filepath.Join(dir, "lib", "node_modules", "npm", "bin", "npm-cli.js")}
		if goruntime.GOOS == "windows" {
			r.Node = filepath.Join(dir, "node.exe")
			r.NPM = filepath.Join(dir, "node_modules", "npm", "bin", "npm-cli.js")
		}
		return r
	}
	validate := func(dir string) error {
		r := layout(dir)
		for _, p := range []string{r.Node, r.NPM} {
			info, err := os.Stat(p)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("missing Node executable %s", p)
			}
		}
		return nil
	}
	dir, e := installCache(root, cacheKey("node-"+version+"-"+goruntime.GOOS+"-"+goruntime.GOARCH, source), validate, func(dir string) error {
		archive := filepath.Join(dir, ".archive")
		if err := downloadFile(source, archive, "Node "+version); err != nil {
			return err
		}
		extract := extractDistribution
		if goruntime.GOOS == "windows" {
			extract = func(a, d string) error { return extractZip(a, d, true) }
		}
		if err := extract(archive, dir); err != nil {
			return err
		}
		return os.Remove(archive)
	})
	if e != nil {
		return nil, e
	}
	return layout(dir), nil
}

func nodeChecksumURL(raw string) string {
	return strings.TrimSuffix(raw, path.Base(raw)) + "SHASUMS256.txt"
}
