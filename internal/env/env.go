// Package env builds the scrubbed environment every child process runs in.
// The host's PHP/Composer/XDG configuration is never inherited.
package env

import (
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"

	"laraserve/internal/runtime"
)

var passthrough = []string{
	"HOME", "USER", "USERNAME", "LOGNAME", "LANG", "LC_ALL", "TERM", "TZ",
	"TMPDIR", "TEMP", "TMP",
	"SystemRoot", "SYSTEMROOT", "USERPROFILE", "COMSPEC", "PATHEXT",
}

// Build returns a minimal environment. stateDir is the project's private
// directory under ~/.laraserve; extra holds app variables (APP_KEY, ...).
func Build(rt *runtime.Runtime, stateDir string, hostPath bool, extra map[string]string) []string {
	m := map[string]string{}
	for _, k := range passthrough {
		if v, ok := os.LookupEnv(k); ok {
			m[k] = v
		}
	}
	path := rt.Dir
	if goruntime.GOOS == "windows" {
		path += ";" + filepath.Join(os.Getenv("SystemRoot"), "System32")
	}
	if hostPath {
		path += string(os.PathListSeparator) + os.Getenv("PATH")
	}
	m["PATH"] = path

	m["PHPRC"] = rt.INI
	m["PHP_INI_SCAN_DIR"] = "" // do not scan host conf.d directories
	m["COMPOSER_HOME"] = filepath.Join(stateDir, "composer")
	m["COMPOSER_CACHE_DIR"] = filepath.Join(stateDir, "composer", "cache")
	m["COMPOSER_NO_INTERACTION"] = "1"
	m["COMPOSER_DISABLE_XDEBUG_WARN"] = "1"
	m["XDG_CONFIG_HOME"] = filepath.Join(stateDir, "xdg", "config")
	m["XDG_DATA_HOME"] = filepath.Join(stateDir, "xdg", "data")
	m["XDG_CACHE_HOME"] = filepath.Join(stateDir, "xdg", "cache")
	for k, v := range extra {
		m[k] = v
	}

	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}
