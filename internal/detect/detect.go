// Package detect inspects a project directory.
package detect

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Project struct {
	Root          string
	HasVendor     bool
	PHPConstraint string
	Laravel       string
	Extensions    []string
}

var defaultConnection = regexp.MustCompile(`(?m)['"]default['"]\s*=>\s*(?:env\(\s*['"]DB_CONNECTION['"]\s*,\s*)?['"]([a-zA-Z0-9_]+)['"]`)

// DatabaseConnection reads standard Laravel database settings without booting
// the application (which may need a running database to boot).
func DatabaseConnection(root string) string {
	dot := DotEnv(filepath.Join(root, ".env"))
	if raw := dot["DB_URL"]; raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" {
			return strings.ToLower(u.Scheme)
		}
	}
	if conn := strings.TrimSpace(dot["DB_CONNECTION"]); conn != "" {
		return strings.ToLower(conn)
	}
	if b, err := os.ReadFile(filepath.Join(root, "config", "database.php")); err == nil {
		if m := defaultConnection.FindSubmatch(b); len(m) == 2 {
			return strings.ToLower(string(m[1]))
		}
	}
	return "sqlite"
}

// Inspect verifies dir is a Laravel app and reads what it needs from composer.json.
func Inspect(dir string) (*Project, error) {
	if _, err := os.Stat(filepath.Join(dir, "artisan")); err != nil {
		return nil, fmt.Errorf("%s is not a Laravel app (no artisan file)", dir)
	}
	b, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		return nil, errors.New("composer.json not found")
	}
	var c struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid composer.json: %w", err)
	}
	lv, ok := c.Require["laravel/framework"]
	if !ok {
		return nil, errors.New("composer.json does not require laravel/framework")
	}
	p := &Project{Root: dir, PHPConstraint: c.Require["php"], Laravel: lv}
	for k := range c.Require {
		if strings.HasPrefix(k, "ext-") {
			p.Extensions = append(p.Extensions, strings.TrimPrefix(k, "ext-"))
		}
	}
	sort.Strings(p.Extensions)
	if _, err := os.Stat(filepath.Join(dir, "vendor", "autoload.php")); err == nil {
		p.HasVendor = true
	}
	return p, nil
}

// DotEnv reads KEY=VALUE pairs from a .env file (best effort, no interpolation).
func DotEnv(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		} else if i := strings.Index(v, " #"); i >= 0 {
			v = strings.TrimSpace(v[:i])
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}
