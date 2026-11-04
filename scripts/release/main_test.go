package main

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeterministicLicensedArchiveAndChecksum(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "client.exe")
	os.WriteFile(binary, []byte("client fixture"), 0755)
	for _, name := range []string{"a.zip", "b.zip"} {
		if err := archiveBinary(filepath.Join(dir, name), binary, []byte("MIT license fixture")); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := os.ReadFile(filepath.Join(dir, "a.zip"))
	b, _ := os.ReadFile(filepath.Join(dir, "b.zip"))
	if string(a) != string(b) {
		t.Fatal("archives are not deterministic")
	}
	zr, err := zip.OpenReader(filepath.Join(dir, "a.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 2 || zr.File[0].Name != "laraserve.exe" || zr.File[1].Name != "LICENSE" {
		t.Fatal("missing executable or license")
	}
	if err = checksums(dir, []string{"a.zip"}); err != nil {
		t.Fatal(err)
	}
	sum, _ := os.ReadFile(filepath.Join(dir, "SHA256SUMS"))
	expected := fmt.Sprintf("%x  a.zip\n", sha256.Sum256(a))
	if string(sum) != expected {
		t.Fatal(string(sum))
	}
}
func TestReleaseDoesNotOverwriteRestorePoint(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "releases", "v1.2.3"), 0755)
	err := createRelease(dir, "1.2.3")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatal(err)
	}
	if err = createRelease(dir, "../../bad"); err == nil {
		t.Fatal("accepted unsafe version")
	}
}
