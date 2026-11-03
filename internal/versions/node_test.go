package versions

import (
	"os"
	"path/filepath"
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
