package mysql

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validDatabaseName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// DatabaseName derives a stable SQL identifier from the original app folder or
// bundle name. Domains may change independently of a project's database.
func DatabaseName(app string) string {
	app = strings.TrimSuffix(app, ".laraserve")
	var b strings.Builder
	separator := false
	for _, c := range strings.ToLower(app) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' {
			if separator && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(c)
			separator = false
		} else {
			separator = true
		}
	}
	name := b.String()
	if name == "" {
		name = "app"
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "app_" + name
	}
	switch name {
	case "mysql", "sys", "information_schema", "performance_schema":
		name = "app_" + name
	}
	if len(name) > 64 {
		sum := sha256.Sum256([]byte(name))
		name = fmt.Sprintf("%s_%x", name[:55], sum[:4])
	}
	return name
}

// Called under the state's exclusive lock. Preserve legacy databases instead of
// silently redirecting existing applications to a newly created empty database.
func projectDatabaseName(state, candidate string) (string, error) {
	file := filepath.Join(state, "database-name")
	if b, err := os.ReadFile(file); err == nil {
		name := strings.TrimSpace(string(b))
		if !validDatabaseName.MatchString(name) {
			return "", fmt.Errorf("invalid managed database name in %s", file)
		}
		return name, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	name := DatabaseName(candidate)
	if _, err := os.Stat(filepath.Join(state, "initialized")); err == nil {
		name = "laraserve"
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.WriteFile(file, []byte(name+"\n"), 0600); err != nil {
		return "", err
	}
	return name, nil
}
