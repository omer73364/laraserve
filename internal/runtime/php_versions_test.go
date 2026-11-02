package runtime

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/klauspost/compress/zstd"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
)

func TestPHPCatalogMatchesSelectableVersions(t *testing.T) {
	var catalog map[string]downloadSource
	if err := json.Unmarshal(phpCatalogJSON, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"7.4.33", "8.0.30", "8.1.34", "8.2.35", "8.3.36", "8.4.27"} {
		for _, arch := range []string{"amd64", "arm64"} {
			if version == "7.4.33" && arch == "arm64" {
				continue
			}
			source, ok := catalog[version+"/"+arch]
			if !ok {
				t.Fatalf("missing %s/%s", version, arch)
			}
			if _, err := sourceOverride("TEST_CATALOG", source); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestLegacyLauncherSetsPrivatePaths(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Unix executable permissions")
	}
	path := filepath.Join(t.TempDir(), "php-legacy")
	script := "#!/bin/sh\nexit 0\n"
	for i := 0; i < 2; i++ {
		if err := writeLegacyPHPLauncher(path, script); err != nil {
			t.Fatal(err)
		}
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode()&0111 == 0 {
		t.Fatalf("launcher permissions: %v", err)
	}
}

func TestLegacyDependencyCatalog(t *testing.T) {
	var packages []legacyPackage
	if err := json.Unmarshal(legacyPackagesJSON, &packages); err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, p := range packages {
		if !strings.HasPrefix(p.Source.URL, "https://archive.ubuntu.com/") {
			t.Fatal(p.Source.URL)
		}
		if _, err := sourceOverride("TEST_CATALOG", p.Source); err != nil {
			t.Fatal(err)
		}
		for _, name := range p.Libraries {
			found[name] = true
		}
	}
	for _, name := range []string{"libicuuc.so.70", "libedit.so.2", "libpcre2-8.so.0", "libonig.so.5", "libxslt.so.1"} {
		if !found[name] {
			t.Fatalf("missing fallback for %s", name)
		}
	}
}

func TestPHP74INIEnablesComposerAndDatabaseExtensions(t *testing.T) {
	dir := t.TempDir()
	extensions := filepath.Join(dir, "libexec", "extensions")
	if err := os.MkdirAll(extensions, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pdo", "mysqlnd", "json", "phar", "ctype", "mbstring", "pdo_mysql", "mysqli", "pdo_sqlite", "dom", "zip"} {
		if err := os.WriteFile(filepath.Join(extensions, name+".so"), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := configurePHP74(dir); err != nil {
		t.Fatal(err)
	}
	ini, err := os.ReadFile(filepath.Join(dir, "php.ini"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"phar", "json", "pdo_mysql", "pdo_sqlite"} {
		if !strings.Contains(string(ini), "extension="+name+".so") {
			t.Fatalf("missing %s", name)
		}
	}
	if strings.Index(string(ini), "extension=pdo.so") > strings.Index(string(ini), "extension=pdo_mysql.so") {
		t.Fatal("PDO consumer loads before PDO")
	}
	if strings.Contains(string(ini), dir) {
		t.Fatal("staging directory leaked into ini")
	}
}

func TestDebZstdDataAndCrossPackageDocumentation(t *testing.T) {
	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	if err := tw.WriteHeader(&tar.Header{Name: "./usr/lib/libtest.so.1", Mode: 0644, Size: 4}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "./usr/share/doc/php-extra", Typeflag: tar.TypeSymlink, Linkname: "php-common", Mode: 0777}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encoder.Write(tarData.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	fmt.Fprintf(&deb, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", "data.tar.zst/", "0", "0", "0", "100644", compressed.Len())
	deb.Write(compressed.Bytes())
	dir := t.TempDir()
	archive := filepath.Join(dir, "package.deb")
	if err := os.WriteFile(archive, deb.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := extractDebData(archive, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "usr/lib/libtest.so.1"))
	if err != nil || string(data) != "test" {
		t.Fatalf("library extraction: %q %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "usr/share/doc/php-extra")); !os.IsNotExist(err) {
		t.Fatal("cross-package doc link retained")
	}
}
