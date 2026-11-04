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

// NodeSelection explains the resolved runtime without executing project code.
type NodeSelection struct {
	Version    string   `json:"version"`
	Source     string   `json:"source"`
	Pin        string   `json:"pin,omitempty"`
	Engine     string   `json:"engine,omitempty"`
	Reused     bool     `json:"reused,omitempty"`
	BuildTools []string `json:"build_tools,omitempty"`
}

func (s NodeSelection) Explanation() string {
	text := s.Source
	if s.Pin != "" {
		text += " · " + s.Pin
	}
	if s.Engine != "" {
		text += " · app requires " + s.Engine
	}
	if s.Reused {
		text += " · keeping compatible selection"
	}
	if len(s.BuildTools) > 0 {
		text += " · build tools require " + strings.Join(s.BuildTools, "; ")
	}
	return text
}

func ProjectNode(root string, available []string, override, locked string, preferred ...string) (string, error) {
	selection, err := ResolveProjectNode(root, available, override, locked, preferred...)
	return selection.Version, err
}

func ResolveProjectNode(root string, available []string, override, locked string, preferred ...string) (NodeSelection, error) {
	selection := NodeSelection{Source: "Automatic compatibility selection"}
	doc, err := projectconfig.Read(root)
	if err != nil {
		return NodeSelection{}, err
	}
	pin := override
	if pin != "" {
		selection.Source = "Explicit override"
	}
	if pin == "" {
		pin = doc.Config.Node
		if pin != "" {
			selection.Source = ".laraserve.json"
		}
	}
	if pin == "" {
		for _, name := range []string{".node-version", ".nvmrc"} {
			b, e := projectconfig.ReadRegular(filepath.Join(root, name), 1024)
			if os.IsNotExist(e) {
				continue
			}
			if e != nil {
				return NodeSelection{}, e
			}
			pin = strings.TrimSpace(string(b))
			if pin != "" {
				selection.Source = name
				break
			}
		}
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
		return NodeSelection{}, fmt.Errorf("package.json: %w", err)
	}
	if pin == "" && pkg.Volta.Node != "" {
		pin = pkg.Volta.Node
		selection.Source = "package.json → volta.node"
	}
	selection.Pin = pin
	// LTS aliases follow compatibility selection, not the newest major.
	if pin == "lts/*" {
		pin = ""
	} else if pin == "node" {
		pin = "latest"
	}
	var engine *semver.Constraints
	if raw := pkg.Engines["node"]; raw != "" {
		selection.Engine = raw
		engine, err = semver.NewConstraint(raw)
		if err != nil {
			return NodeSelection{}, fmt.Errorf("unsupported Node engine %q: %w", raw, err)
		}
	}
	toolEngines, err := buildToolEngines(root)
	if err != nil {
		return NodeSelection{}, err
	}
	for _, requirement := range toolEngines {
		selection.BuildTools = append(selection.BuildTools, requirement.String())
	}
	sort.Strings(selection.BuildTools)
	list := append([]string(nil), available...)
	lts := map[string]bool{}
	for _, v := range preferred {
		lts[v] = true
	}
	// Broad engines such as >=14 do not prove that an older frontend works
	// with the newest major. Use a conservative baseline unless the app pins
	// another version or its requirements exclude these established lines.
	rank := func(v string) int {
		if !lts[v] {
			return 3
		}
		parsed, err := semver.NewVersion(v)
		if err == nil {
			switch parsed.Major() {
			case 22:
				return 0
			case 24:
				return 1
			}
		}
		return 2
	}
	sort.Slice(list, func(i, j int) bool {
		if pin == "" && rank(list[i]) != rank(list[j]) {
			return rank(list[i]) < rank(list[j])
		}
		a, _ := semver.NewVersion(list[i])
		b, _ := semver.NewVersion(list[j])
		return a.GreaterThan(b)
	})
	if locked != "" && (pin != "" || len(preferred) == 0 || rank(locked) < 2) {
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
				selection.Version = v
				if pin == "" && v == locked {
					selection.Reused = true
				}
				return selection, nil
			}
		}
	}
	return NodeSelection{}, fmt.Errorf("no compatible Node version: %s pin %q, package.json engines.node %q, build-tool requirements [%s]. Choose a compatible project pin or update conflicting dependencies", selection.Source, selection.Pin, selection.Engine, strings.Join(selection.BuildTools, "; "))
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
