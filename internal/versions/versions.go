// Package versions resolves project runtime requirements without executing PHP.
package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"laraserve/internal/detect"
	"laraserve/internal/projectconfig"
)

type Pins struct {
	Node  string `json:"node,omitempty"`
	PHP   string `json:"php,omitempty"`
	MySQL string `json:"mysql,omitempty"`
}

func ReadLock(path string) (Pins, error) {
	var p Pins
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	err = json.Unmarshal(b, &p)
	return p, err
}
func WriteLock(path string, p Pins) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".versions-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

var numeric = regexp.MustCompile(`^v?\d+(?:\.\d+){0,2}$`)
var token = regexp.MustCompile(`(~|\^|>=|<=|!=|>|<|=)?\s*(\d+(?:\.\d+){0,2})(?:\.[*xX])?(?:@(?:dev|stable|beta|alpha|RC))?`)

// Composer differs from npm semver for ~8.2 and bare partial versions.
func ComposerConstraint(raw string) (*semver.Constraints, error) {
	raw = strings.ReplaceAll(raw, "|", "||")
	for strings.Contains(raw, "||||") {
		raw = strings.ReplaceAll(raw, "||||", "||")
	}
	// Semver's hyphen expansion already matches Composer's partial upper bounds.
	if !strings.Contains(raw, " - ") {
		raw = token.ReplaceAllStringFunc(raw, func(t string) string {
			m := token.FindStringSubmatch(t)
			if strings.ContainsAny(t, "*xX") {
				return t
			}
			parts := strings.Split(m[2], ".")
			if m[1] == "~" && len(parts) == 2 {
				v, _ := semver.NewVersion(m[2])
				return fmt.Sprintf(">=%s <%d.0.0", v.String(), v.Major()+1)
			}
			v, err := semver.NewVersion(m[2])
			if err != nil {
				return t
			}
			return m[1] + v.String()
		})
	}
	c, err := semver.NewConstraint(raw)
	if err != nil {
		return nil, fmt.Errorf("unsupported PHP constraint %q: %w", raw, err)
	}
	return c, nil
}

func pinMatch(version, pin string) bool {
	pin = strings.TrimPrefix(strings.TrimSpace(pin), "v")
	if pin == "" || pin == "latest" {
		return true
	}
	if numeric.MatchString(pin) {
		return version == pin || strings.HasPrefix(version, pin+".")
	}
	c, err := semver.NewConstraint(pin)
	v, verr := semver.NewVersion(version)
	return err == nil && verr == nil && c.Check(v)
}

func choose(available []string, pin, locked string, checks []*semver.Constraints, conflicts []*semver.Constraints, prefer string) (string, error) {
	accept := func(s string) bool {
		if !pinMatch(s, pin) {
			return false
		}
		v, err := semver.NewVersion(s)
		if err != nil {
			return false
		}
		for _, c := range checks {
			if !c.Check(v) {
				return false
			}
		}
		for _, c := range conflicts {
			if c.Check(v) {
				return false
			}
		}
		return true
	}
	for _, s := range available {
		if s == locked && accept(s) {
			return s, nil
		}
	}
	if pin == "" && prefer != "" {
		for _, s := range available {
			if s == prefer && accept(s) {
				return s, nil
			}
		}
	}
	list := append([]string{}, available...)
	sort.Slice(list, func(i, j int) bool {
		a, _ := semver.NewVersion(list[i])
		b, _ := semver.NewVersion(list[j])
		if a.Major() != b.Major() {
			return a.Major() < b.Major()
		}
		if a.Minor() != b.Minor() {
			return a.Minor() < b.Minor()
		}
		return a.Patch() > b.Patch()
	})
	for _, s := range list {
		if accept(s) {
			return s, nil
		}
	}
	return "", fmt.Errorf("no available version matches pin %q and project requirements (available: %s)", pin, strings.Join(available, ", "))
}

func ProjectPHP(root string, available []string, override, locked string) (string, error) {
	pins, err := projectPins(root)
	if err != nil {
		return "", err
	}
	pin := override
	if pin == "" {
		pin = pins.PHP
	}
	if pin == "" {
		pin = readVersion(root, ".php-version")
	}
	var composer struct {
		Require    map[string]string
		RequireDev map[string]string `json:"require-dev"`
		Conflict   map[string]string
		Config     struct{ Platform map[string]json.RawMessage }
	}
	if b, err := os.ReadFile(filepath.Join(root, "composer.json")); err == nil {
		if err = json.Unmarshal(b, &composer); err != nil {
			return "", fmt.Errorf("composer.json: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if pin == "" {
		if raw := composer.Config.Platform["php"]; len(raw) > 0 {
			var s string
			if json.Unmarshal(raw, &s) == nil && s != "" {
				parts := strings.Split(s, ".")
				if len(parts) >= 2 {
					pin = strings.Join(parts[:2], ".")
				} else {
					pin = s
				}
			}
		}
	}
	var requirements, not []string
	for _, m := range []map[string]string{composer.Require, composer.RequireDev} {
		if v := m["php"]; v != "" {
			requirements = append(requirements, v)
		}
	}
	if v := composer.Conflict["php"]; v != "" {
		not = append(not, v)
	}
	var lock struct {
		Platform map[string]string
		Packages []struct {
			Require  map[string]string
			Conflict map[string]string
		}
		Dev []struct {
			Require  map[string]string
			Conflict map[string]string
		} `json:"packages-dev"`
	}
	if b, err := os.ReadFile(filepath.Join(root, "composer.lock")); err == nil {
		if err = json.Unmarshal(b, &lock); err != nil {
			return "", fmt.Errorf("composer.lock: %w", err)
		}
		if s := lock.Platform["php"]; s != "" {
			requirements = append(requirements, s)
		}
		for _, p := range append(lock.Packages, lock.Dev...) {
			if s := p.Require["php"]; s != "" {
				requirements = append(requirements, s)
			}
			if s := p.Conflict["php"]; s != "" {
				not = append(not, s)
			}
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	var checks, conflicts []*semver.Constraints
	for _, r := range requirements {
		c, err := ComposerConstraint(r)
		if err != nil {
			return "", err
		}
		checks = append(checks, c)
	}
	for _, r := range not {
		c, err := ComposerConstraint(r)
		if err != nil {
			return "", err
		}
		conflicts = append(conflicts, c)
	}
	prefer := ""
	if len(requirements) == 0 {
		prefer = "8.5.12"
	}
	v, err := choose(available, pin, locked, checks, conflicts, prefer)
	if err != nil {
		return "", fmt.Errorf("PHP: %w; set a compatible .php-version or .laraserve.json pin", err)
	}
	return v, nil
}

var mysqlImage = regexp.MustCompile(`(?m)^\s*image:\s*["']?(?:docker\.io/(?:library/)?)?(?:mysql/mysql-server|mysql):([^\s"'#]+)`)
var envImage = regexp.MustCompile(`^\$\{([A-Z_]+)(?::?-([^}]+))?\}$`)

func ProjectMySQL(root string, available []string, override, locked string) (string, error) {
	pins, err := projectPins(root)
	if err != nil {
		return "", err
	}
	pin := override
	if pin == "" {
		pin = pins.MySQL
	}
	if pin == "" {
		pin = readVersion(root, ".mysql-version")
	}
	env := detect.DotEnv(filepath.Join(root, ".env"))
	if pin == "" {
		pin = env["MYSQL_VERSION"]
	}
	if pin == "" {
		pin = env["DB_VERSION"]
	}
	if pin == "" {
		seen := map[string]bool{}
		for _, file := range []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"} {
			b, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return "", err
			}
			for _, m := range mysqlImage.FindAllStringSubmatch(string(b), -1) {
				s := m[1]
				if sub := envImage.FindStringSubmatch(s); sub != nil {
					s = env[sub[1]]
					if s == "" {
						s = sub[2]
					}
				}
				if s != "" && s != "latest" {
					seen[s] = true
				}
			}
		}
		if len(seen) > 1 {
			return "", fmt.Errorf("multiple MySQL image versions found; pin mysql in .laraserve.json")
		}
		for s := range seen {
			pin = s
		}
	}
	v, err := choose(available, pin, locked, nil, nil, "8.4.13")
	if err != nil {
		return "", fmt.Errorf("MySQL: %w", err)
	}
	return v, nil
}
func projectPins(root string) (Pins, error) {
	doc, err := projectconfig.Read(root)
	return Pins{PHP: doc.Config.PHP, MySQL: doc.Config.MySQL}, err
}
func readVersion(root, name string) string {
	b, _ := os.ReadFile(filepath.Join(root, name))
	return strings.TrimSpace(string(b))
}
