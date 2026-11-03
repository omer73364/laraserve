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
func ProjectNode(root string, available []string, override, locked string) (string, error) {
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
		Engines map[string]string `json:"engines"`
	}
	b, err := projectconfig.ReadRegular(filepath.Join(root, "package.json"), 1<<20)
	if err == nil {
		err = json.Unmarshal(b, &pkg)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("package.json: %w", err)
	}
	var engine *semver.Constraints
	if raw := pkg.Engines["node"]; raw != "" {
		engine, err = semver.NewConstraint(raw)
		if err != nil {
			return "", fmt.Errorf("unsupported Node engine %q: %w", raw, err)
		}
	}
	list := append([]string(nil), available...)
	sort.Slice(list, func(i, j int) bool {
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
			return v, nil
		}
	}
	return "", fmt.Errorf("no Node version matches pin %q and package.json engines; choose a compatible Node pin", pin)
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
