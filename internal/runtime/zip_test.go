package runtime

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func zipFixture(t *testing.T, names []string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "runtime.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for _, name := range names {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte("fixture")); err != nil {
			t.Fatal(err)
		}
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestZipDistributionLayouts(t *testing.T) {
	for _, strip := range []bool{false, true} {
		name := "php.exe"
		if strip {
			name = "mysql-8/bin/mysqld.exe"
		}
		root := t.TempDir()
		if err := extractZip(zipFixture(t, []string{name}), root, strip); err != nil {
			t.Fatal(err)
		}
		want := "php.exe"
		if strip {
			want = filepath.Join("bin", "mysqld.exe")
		}
		if _, err := os.Stat(filepath.Join(root, want)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestZipRejectsWindowsEscapesAndCollisions(t *testing.T) {
	for _, names := range [][]string{{"../escape"}, {"C:/escape"}, {"/escape"}, {`dir\escape`}, {"file:stream"}, {"NUL.txt"}, {"dir/COM1"}, {"bad. /file"}, {"PHP.exe", "php.exe"}, {"a/file", "b/file"}} {
		strip := names[0] == "a/file"
		if err := extractZip(zipFixture(t, names), t.TempDir(), strip); err == nil {
			t.Errorf("accepted %v", names)
		}
	}
}
