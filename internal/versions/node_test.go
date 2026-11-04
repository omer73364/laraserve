package versions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodeRequirements(t *testing.T) {
	root := t.TempDir()
	put := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("package.json", `{"engines":{"node":"^20.19 || >=22.12"}}`)
	available := []string{"18.20.8", "20.20.0", "22.22.0", "24.10.0"}
	for _, tc := range []struct{ pin, lock, want string }{{"", "", "24.10.0"}, {"", "22.22.0", "22.22.0"}, {"20", "", "20.20.0"}, {"18", "", ""}, {"", "99.0.0", "24.10.0"}} {
		got, err := ProjectNode(root, available, tc.pin, tc.lock)
		if tc.want == "" {
			if err == nil {
				t.Fatal("accepted incompatible Node")
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%+v: %s %v", tc, got, err)
		}
	}
	put(".nvmrc", "20\n")
	got, err := ProjectNode(root, available, "", "")
	if err != nil || got != "20.20.0" {
		t.Fatal(got, err)
	}
	put(".laraserve.json", `{"node":"22"}`)
	got, err = ProjectNode(root, available, "", "")
	if err != nil || got != "22.22.0" {
		t.Fatal(got, err)
	}
	put(".laraserve.json", `{"node":"lts/*"}`)
	got, err = ProjectNode(root, available, "", "")
	if err != nil || got != "24.10.0" {
		t.Fatal(got, err)
	}
}

func TestNodeSelectionExplainsPinsAndPreservesExactVersions(t *testing.T) {
	for _, tc := range []struct{ file, body, source, want string }{
		{".nvmrc", "22.22.0", ".nvmrc", "22.22.0"},
		{".node-version", "24", ".node-version", "24.10.0"},
		{".laraserve.json", `{"node":"22"}`, ".laraserve.json", "22.23.0"},
		{"package.json", `{"volta":{"node":"24.10.0"}}`, "package.json → volta.node", "24.10.0"},
		{".nvmrc", "lts/*", ".nvmrc", "22.23.0"},
	} {
		t.Run(tc.file+tc.want, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, tc.file)
			if err := os.WriteFile(path, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ResolveProjectNode(root, []string{"22.22.0", "22.23.0", "24.10.0", "26.1.0"}, "", "", "22.23.0", "24.10.0", "26.1.0")
			if err != nil || got.Version != tc.want || got.Source != tc.source || !strings.Contains(got.Explanation(), tc.source) {
				t.Fatalf("%+v, %v", got, err)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.body {
				t.Fatal("selection changed the project pin")
			}
		})
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".nvmrc"), []byte("22.22.1"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProjectNode(root, []string{"22.23.0"}, "", ""); err == nil || !strings.Contains(err.Error(), ".nvmrc pin \"22.22.1\"") {
		t.Fatalf("missing exact pin explanation: %v", err)
	}
}
func TestPackageManagerConflicts(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"packageManager":"pnpm@10.0.0"}`), 0600)
	name, version, err := PackageManager(root)
	if err != nil || name != "pnpm" || version != "10.0.0" {
		t.Fatal(name, version, err)
	}
	os.WriteFile(filepath.Join(root, "package-lock.json"), nil, 0600)
	if _, _, err = PackageManager(root); err == nil {
		t.Fatal("accepted conflicting lockfile")
	}
}

func TestProjectsChooseDifferentCompatibleBuildToolNodeVersions(t *testing.T) {
	available := []string{"18.20.8", "20.20.0", "22.22.0", "24.10.0"}
	for _, tc := range []struct{ engine, want string }{{"^18 || ^20", "20.20.0"}, {"^22.12 || ^24", "24.10.0"}} {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vite":"*"}}`), 0600)
		os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"packages":{"node_modules/vite":{"engines":{"node":"`+tc.engine+`"}}}}`), 0600)
		got, e := ProjectNode(root, available, "", "24.10.0")
		if e != nil || got != tc.want {
			t.Fatal(tc, got, e)
		}
	}
}

func TestDefaultNodePrefersLTSButHonorsProjectPin(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"engines":{"node":">=20"}}`), 0600)
	available := []string{"22.22.0", "24.10.0", "26.1.0"}
	got, e := ProjectNode(root, available, "", "", "22.22.0", "24.10.0")
	if e != nil || got != "22.22.0" {
		t.Fatal(got, e)
	}
	got, e = ProjectNode(root, available, "26", "", "24.10.0")
	if e != nil || got != "26.1.0" {
		t.Fatal(got, e)
	}
}

func TestLockfileLegacyDependencyEngines(t *testing.T) {
	root := t.TempDir()
	lock := `{"packages":{
		"node_modules/legacy-package":{"engines":["node >=0.4"]},
		"node_modules/another-package":{"engines":[]},
		"node_modules/webpack":{"engines":{"node":"^18 || ^20","npm":["legacy"]}},
		"node_modules/laravel-mix":{"engines":{"node":">=18"}}
	}}`
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(lock), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ProjectNode(root, []string{"18.20.8", "20.20.0", "24.10.0"}, "", "24.10.0")
	if err != nil || got != "20.20.0" {
		t.Fatalf("legacy metadata must preserve build-tool constraints: got %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{"packages":`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectNode(root, []string{"20.20.0"}, "", ""); err == nil {
		t.Fatal("accepted malformed lockfile")
	}
}

func TestAutomaticNodeAvoidsNewMajorLock(t *testing.T) {
	for _, tc := range []struct{ engine, pin, locked, want string }{
		{">=14", "", "26.1.0", "22.22.0"},
		{">=24", "", "26.1.0", "24.10.0"},
		{">=26", "", "", "26.1.0"},
		{">=14", "26", "", "26.1.0"},
		{">=14", "", "24.10.0", "24.10.0"},
	} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"engines":{"node":"`+tc.engine+`"}}`), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := ProjectNode(root, []string{"22.22.0", "24.10.0", "26.1.0"}, tc.pin, tc.locked, "22.22.0", "24.10.0", "26.1.0")
		if err != nil || got != tc.want {
			t.Fatalf("%+v: got %q, %v", tc, got, err)
		}
	}
}
