package versions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	"laraserve/internal/projectconfig"
)

// ProjectNode checks both an explicit version selection and the package engine.
// Prefer the newest compatible LTS supplied by the caller; keep a compatible lock.
func ProjectNode(root string, available []string, override, locked string, preferred ...string) (string, error) {
	doc, err := projectconfig.Read(root)
	if err != nil {
		return "", err
	}
	pin := override
	if pin == "" {
		pin = doc.Config.Node
	}
	if pin == "" {
		for _, name := range []string{".node-version", ".nvmrc"} {
			b, e := projectconfig.ReadRegular(filepath.Join(root, name), 1024)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return "", e
			}
			pin = strings.TrimSpace(string(b))
			if pin != "" {
				break
			}
		}
	}
	if pin == "node" || pin == "lts/*" {
		pin = "latest"
	}
	var pkg struct {
		Volta struct {
			Node string `json:"node"`
		} `json:"volta"`
		Engines map[string]string `json:"engines"`
	}
	b, err := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if err == nil {
		err = json.Unmarshal(b, &pkg)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("package.json: %w", err)
	}
	if pin == "" && pkg.Volta.Node != "" {
		pin = pkg.Volta.Node
	}
	var engine *semver.Constraints
	if raw := pkg.Engines["node"]; raw != "" {
		engine, err = semver.NewConstraint(raw)
		if err != nil {
			return "", fmt.Errorf("unsupported Node engine %q: %w", raw, err)
		}
	}
	toolEngines, err := buildToolEngines(root)
	if err != nil {
		return "", err
	}
	list := append([]string(nil), available...)
	lts := map[string]bool{}
	for _, v := range preferred {
		lts[v] = true
	}
	sort.Slice(list, func(i, j int) bool {
		if pin == "" && lts[list[i]] != lts[list[j]] {
			return lts[list[i]]
		}
		a, _ := semver.NewVersion(list[i])
		b, _ := semver.NewVersion(list[j])
		return a.GreaterThan(b)
	})
	if locked != "" {
		list = append([]string{locked}, list...)
	}
	for _, v := range list {
		present := false
		for _, candidate := range available {
			if candidate == v {
				present = true
				break
			}
		}
		if !present || !pinMatch(v, pin) {
			continue
		}
		parsed, e := semver.NewVersion(v)
		if e == nil && (engine == nil || engine.Check(parsed)) {
			compatible := true
			for _, requirement := range toolEngines {
				if !requirement.Check(parsed) {
					compatible = false
					break
				}
			}
			if compatible {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("no Node version matches pin %q and project/build-tool engines; update the pin or choose compatible dependencies", pin)
}

// PackageManager refuses ambiguous lockfiles instead of installing with the wrong
// manager. It does not execute package.json scripts or contact a registry.
func PackageManager(root string) (string, string, error) {
	var pkg struct {
		PackageManager string `json:"packageManager"`
	}
	b, err := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if err != nil {
		return "", "", err
	}
	if err = json.Unmarshal(b, &pkg); err != nil {
		return "", "", err
	}
	managers := map[string]bool{}
	for file, manager := range map[string]string{"package-lock.json": "npm", "npm-shrinkwrap.json": "npm", "yarn.lock": "yarn", "pnpm-lock.yaml": "pnpm", "bun.lock": "bun", "bun.lockb": "bun"} {
		_, e := os.Lstat(filepath.Join(root, file))
		if e == nil {
			managers[manager] = true
		} else if !os.IsNotExist(e) {
			return "", "", e
		}
	}
	name, version, _ := strings.Cut(pkg.PackageManager, "@")
	if name != "" {
		switch name {
		case "npm", "pnpm", "yarn", "bun":
		default:
			return "", "", fmt.Errorf("unsupported package manager %q", name)
		}
		if len(managers) > 1 || (len(managers) == 1 && !managers[name]) {
			return "", "", fmt.Errorf("packageManager conflicts with project lockfiles")
		}
		return name, version, nil
	}
	if len(managers) > 1 {
		return "", "", fmt.Errorf("multiple package-manager lockfiles; keep the intended lockfile")
	}
	for name := range managers {
		return name, "", nil
	}
	return "npm", "", nil
}

// Use the project's locked/installed build-tool engines, not a global Node pin.
func buildToolEngines(root string) ([]*semver.Constraints, error) {
	names := []string{"vite", "webpack", "laravel-vite-plugin", "laravel-mix", "next"}
	raw := map[string]string{}
	b, err := projectconfig.ReadRegular(filepath.Join(root, "package-lock.json"), 32<<20)
	if err == nil {
		var lock struct {
			Packages map[string]json.RawMessage `json:"packages"`
		}
		if err = json.Unmarshal(b, &lock); err != nil {
			return nil, fmt.Errorf("package-lock.json: %w", err)
		}
		for _, name := range names {
			entry := lock.Packages["node_modules/"+name]
			if len(entry) == 0 {
				continue
			}
			var pkg struct {
				Engines struct {
					Node string `json:"node"`
				} `json:"engines"`
			}
			// Decode only tools that influence runtime selection. Older npm
			// packages can contain array-valued engines and other legacy metadata.
			if err := json.Unmarshal(entry, &pkg); err != nil {
				return nil, fmt.Errorf("package-lock.json: %s: %w", name, err)
			}
			raw[name] = pkg.Engines.Node
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, name := range names {
		if raw[name] != "" {
			continue
		}
		b, e := projectconfig.ReadRegular(filepath.Join(root, "node_modules", name, "package.json"), 1<<20)
		if e == nil {
			var p struct {
				Engines map[string]string `json:"engines"`
			}
			if json.Unmarshal(b, &p) == nil {
				raw[name] = p.Engines["node"]
			}
		}
	}
	result := []*semver.Constraints{}
	for name, value := range raw {
		if value == "" {
			continue
		}
		constraint, e := semver.NewConstraint(value)
		if e != nil {
			return nil, fmt.Errorf("%s Node requirement %q: %w", name, value, e)
		}
		result = append(result, constraint)
	}
	return result, nil
}
