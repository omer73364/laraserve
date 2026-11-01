package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadedMySQL(t *testing.T) {
	if os.Getenv("LARASERVE_TEST_DOWNLOADED_MYSQL") != "1" {
		t.Skip("set LARASERVE_TEST_DOWNLOADED_MYSQL=1 to verify the downloaded distribution")
	}
	t.Setenv("LARASERVE_MYSQL_DIR", "")
	runtimes := t.TempDir()
	dir, err := EnsureMySQL(runtimes)
	if err != nil {
		t.Fatal(err)
	}
	for _, binary := range []string{"mysqld", "mysql"} {
		out, err := exec.Command(filepath.Join(dir, "bin", binary), "--version").CombinedOutput()
		if err != nil {
			t.Fatalf("unpacked %s failed: %v %s", binary, err, out)
		}
		t.Logf("%s", out)
	}
	if again, err := EnsureMySQL(runtimes); err != nil || again != dir {
		t.Fatalf("runtime cache not reused: %s %v", again, err)
	}
}

func TestMySQLArchiveExtraction(t *testing.T) {
	for _, test := range []struct {
		name string
		kind byte
		bad  bool
	}{
		{"bin/mysqld", tar.TypeReg, false},
		{"../escape", tar.TypeReg, true},
		{"/absolute", tar.TypeReg, true},
		{"link", tar.TypeSymlink, true},
		{"hardlink", tar.TypeLink, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			tw := tar.NewWriter(gz)
			name := "distribution/" + test.name
			if strings.HasPrefix(test.name, "/") {
				name = test.name
			}
			h := &tar.Header{Name: name, Mode: 0o755, Typeflag: test.kind}
			if test.kind == tar.TypeReg {
				h.Size = 4
			}
			if err := tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			if h.Size > 0 {
				_, _ = tw.Write([]byte("test"))
			}
			_ = tw.Close()
			_ = gz.Close()
			dir := t.TempDir()
			archive := filepath.Join(t.TempDir(), "runtime.tar.gz")
			if err := os.WriteFile(archive, buf.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			err := extractDistribution(archive, dir)
			if (err != nil) != test.bad {
				t.Fatalf("unexpected extraction result: %v", err)
			}
			if !test.bad {
				st, err := os.Stat(filepath.Join(dir, test.name))
				if err != nil || st.Mode()&0o111 == 0 {
					t.Fatal("executable permissions lost")
				}
			}
		})
	}
}

func TestDistributionLinkChain(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range []*tar.Header{
		{Name: "dist/lib/libexample.so", Typeflag: tar.TypeSymlink, Linkname: "libexample.so.1", Mode: 0755},
		{Name: "dist/lib/libexample.so.1", Typeflag: tar.TypeSymlink, Linkname: "libexample.so.1.0", Mode: 0755},
		{Name: "dist/lib/libexample.so.1.0", Typeflag: tar.TypeReg, Size: 4, Mode: 0644},
	} {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte("test"))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	archive := filepath.Join(t.TempDir(), "runtime.tar.gz")
	if err := os.WriteFile(archive, buf.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := extractDistribution(archive, dir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "lib", "libexample.so"))
	if err != nil || string(b) != "test" {
		t.Fatal("forward link chain was not restored", err)
	}
}

// A runtime cached before libaio was installed must be repairable without a
// fresh download. Modern Ubuntu exposes the same library under a t64 name.
func TestMySQLLibraryRepair(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux shared libraries")
	}
	dir, system := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(system, "libaio.so.1t64.0.2"), []byte("aio fixture"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := mysqlLibrariesFrom(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "private", "libaio.so.1")); !os.IsNotExist(err) {
		t.Fatal("unexpected library before installation")
	}
	for i := 0; i < 2; i++ {
		if err := mysqlLibrariesFrom(dir, []string{system}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, "lib", "private", "libaio.so.1"))
		if err != nil || string(b) != "aio fixture" {
			t.Fatalf("library repair: %q %v", b, err)
		}
	}
}

func TestMySQLPrivateLibraryEnvironment(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux loader")
	}
	dir := t.TempDir()
	env := MySQLEnv(dir, []string{"HOME=/private", "LD_LIBRARY_PATH=/host/libraries", "PATH=/bin"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "/host/libraries") || !strings.Contains(joined, "HOME=/private") || !strings.Contains(joined, "LD_LIBRARY_PATH="+filepath.Join(dir, "lib", "private")+":"+filepath.Join(dir, "lib")) {
		t.Fatal(joined)
	}
}

func TestMySQLMissingLibraryHelp(t *testing.T) {
	for _, lib := range []string{"libaio.so.1", "libnuma.so.1", "libstdc++.so.6", "libother.so.2"} {
		message := mysqlLibraryHelp("mysqld: error while loading shared libraries: " + lib + ": cannot open shared object file: No such file or directory")
		if !strings.Contains(message, lib) || !strings.Contains(message, "rerun LaraServe") {
			t.Fatal(message)
		}
	}
	if mysqlLibraryHelp("unknown option") != "" {
		t.Fatal("unrelated error misdiagnosed")
	}
}

func TestVerifyMySQLUsesPrivateLibraries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux loader")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$LD_LIBRARY_PATH" in
 */lib/private:*/lib) echo 'mysqld Ver 8.4.13 for Linux on x86_64';;
 *) echo 'mysqld: error while loading shared libraries: libaio.so.1: cannot open shared object file' >&2; exit 127;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "bin", "mysqld"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LD_LIBRARY_PATH", "/unrelated/host/library")
	if err := verifyMySQL(dir, "8.4.13"); err != nil {
		t.Fatal(err)
	}
}
