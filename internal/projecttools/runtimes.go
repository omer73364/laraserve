package projecttools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"laraserve/internal/filelock"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
	"laraserve/internal/supervisor"
)

type CacheEntry struct {
	Kind      string `json:"kind,omitempty"`
	Version   string `json:"version,omitempty"`
	NPM       string `json:"npm,omitempty"`
	Key       string `json:"key"`
	Bytes     int64  `json:"bytes"`
	Protected bool   `json:"protected"`
	Reason    string `json:"reason,omitempty"`
}
type Inventory struct {
	PHP       []string     `json:"php"`
	MySQL     []string     `json:"mysql"`
	Installed []CacheEntry `json:"installed"`
}

var cacheName = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*-[a-f0-9]{20}$`)
var cacheVersion = regexp.MustCompile(`^(php|mysql|node)-([0-9]+\.[0-9]+\.[0-9]+)-`)

func RuntimeInventory(home string) (Inventory, error) {
	result := Inventory{PHP: runtime.PHPVersions(), MySQL: runtime.MySQLVersions(), Installed: []CacheEntry{}}
	root := filepath.Join(home, "runtimes")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	registered, err := projects.New(home).List()
	if err != nil {
		return result, err
	}
	references := map[string]bool{}
	missingSnapshot := false
	for _, p := range registered {
		e, err := execution(home, p.ID)
		if err != nil {
			missingSnapshot = true
			continue
		}
		for _, path := range []string{e.Runtime.Dir, e.Runtime.PHP, e.Runtime.NativeCLI, e.Runtime.ServerPHP, e.Runtime.Composer, e.Runtime.INI} {
			rel, err := filepath.Rel(root, path)
			if err != nil || strings.HasPrefix(rel, "..") {
				continue
			}
			references[strings.Split(rel, string(filepath.Separator))[0]] = true
		}
		// Runtime-specific dependencies and alternate source hashes for the selected
		// MySQL version remain available across restarts and library preparation.
		for _, entry := range entries {
			if p.MySQL != "" && strings.HasPrefix(entry.Name(), "mysql-"+p.MySQL+"-") {
				references[entry.Name()] = true
			}
		}
	}
	for _, entry := range entries {
		key := entry.Name()
		if !entry.IsDir() || !cacheName.MatchString(key) {
			continue
		}
		path := filepath.Join(root, key)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if _, err = os.Stat(filepath.Join(path, ".ready")); err != nil {
			continue
		}
		item := CacheEntry{Key: key}
		if parts := cacheVersion.FindStringSubmatch(key); parts != nil {
			item.Kind, item.Version = parts[1], parts[2]
			if item.Kind == "node" {
				item.NPM = cachedNPMVersion(path)
			}
		}
		item.Protected = references[key] || missingSnapshot
		if references[key] {
			item.Reason = "Referenced by a registered project's recorded runtime"
		} else if missingSnapshot {
			item.Reason = "A registered project needs one restart before references can be verified"
		}
		// Keep shared Composer, NSS, legacy libraries and platform helper payloads.
		if !(strings.HasPrefix(key, "php-") || strings.HasPrefix(key, "mysql-")) {
			item.Protected = true
			item.Reason = "Shared dependency cache"
			if item.Kind == "node" {
				item.Reason = "Shared Node runtime; retained for project commands and frontend tasks"
			}
		}
		if strings.HasPrefix(key, "php-") && strings.Contains(key, "-ini-") {
			item.Protected = true
			item.Reason = "Platform configuration dependency"
		}
		err = filepath.WalkDir(path, func(file string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type().IsRegular() {
				info, e := entry.Info()
				if e != nil {
					return e
				}
				item.Bytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return result, err
		}
		result.Installed = append(result.Installed, item)
	}
	sort.Slice(result.Installed, func(i, j int) bool { return result.Installed[i].Key < result.Installed[j].Key })
	return result, nil
}

// Inspect package metadata without executing npm or contacting the registry.
func cachedNPMVersion(dir string) string {
	for _, name := range []string{filepath.Join("lib", "node_modules", "npm", "package.json"), filepath.Join("node_modules", "npm", "package.json")} {
		data, err := projectconfig.ReadRegular(filepath.Join(dir, name), 1<<20)
		if err != nil {
			continue
		}
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			return pkg.Version
		}
	}
	return ""
}

// RemoveUnusedCache rechecks ownership, references, and the selected cache key
// under startup and download locks. It never touches databases or project state.
func RemoveUnusedCache(ctx context.Context, home, key string) error {
	rootInfo, rootErr := os.Lstat(filepath.Join(home, "runtimes"))
	if rootErr != nil {
		return rootErr
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache cleanup requires a regular private runtime directory")
	}
	if !cacheName.MatchString(key) {
		return fmt.Errorf("invalid cache key")
	}
	release, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return err
	}
	defer release()
	if err := RequireNoTasks(home, ""); err != nil {
		return err
	}
	states, err := supervisor.States(home)
	if err != nil {
		return err
	}
	tasks, err := supervisor.States(taskDir(home))
	if err != nil {
		return err
	}
	states = append(states, tasks...)
	for _, state := range states {
		switch state.Status {
		case "starting", "running", "stopping", "unresponsive":
			return fmt.Errorf("stop all projects and wait for tasks before removing cached runtimes")
		}
	}
	unlock, ok, err := filelock.Try(filepath.Join(home, "runtimes", key+".lock"))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("runtime is being downloaded or prepared")
	}
	defer unlock()
	inventory, err := RuntimeInventory(home)
	if err != nil {
		return err
	}
	for _, entry := range inventory.Installed {
		if entry.Key == key {
			if entry.Protected {
				return fmt.Errorf("cache protected: %s", entry.Reason)
			}
			return os.RemoveAll(filepath.Join(home, "runtimes", key))
		}
	}
	return fmt.Errorf("cache is missing or not a published runtime")
}
