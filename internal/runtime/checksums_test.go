package runtime

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublisherChecksumVerifiesCurrentRelease(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	payload := "updated publisher artifact"
	metadataCalls := 0
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body := payload
		if r.URL.Host == "api.github.com" {
			metadataCalls++
			body = fmt.Sprintf(`{"assets":[{"name":"tool","digest":"sha256:%s"}]}`, digest([]byte(payload)))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
	})}
	dst := filepath.Join(t.TempDir(), "tool")
	if err := downloadFile(downloadSource{"https://github.com/org/repo/releases/download/v1/tool", "publisher"}, dst, "test"); err != nil {
		t.Fatal(err)
	}
	if metadataCalls != 1 {
		t.Fatal(metadataCalls)
	}
	b, err := os.ReadFile(dst)
	if err != nil || string(b) != payload {
		t.Fatal(string(b), err)
	}
}

func TestMissingPublisherDigestFailsClosed(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	calls := 0
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"assets":[]}`)), Request: r}, nil
	})}
	dst := filepath.Join(t.TempDir(), "tool")
	if err := downloadFile(downloadSource{"https://github.com/org/repo/releases/download/v1/tool", "publisher"}, dst, "test"); err == nil {
		t.Fatal("accepted missing digest")
	}
	if calls != 1 {
		t.Fatal("downloaded artifact without metadata")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("created unverified artifact")
	}
}

func TestPackageChecksumRecords(t *testing.T) {
	entries, err := parsePackageChecksums(strings.NewReader("Package: a\nFilename: pool/main/a/a.deb\nSHA256: aaa\n\nPackage: b\nFilename: pool/main/b/b.deb\nSHA256: bbb\n"))
	if err != nil || entries["pool/main/a/a.deb"] != "aaa" || entries["pool/main/b/b.deb"] != "bbb" {
		t.Fatal(entries, err)
	}
}

func TestPublisherRejectsChangedArtifact(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		body := "wrong payload"
		if r.URL.Host == "getcomposer.org" && strings.HasSuffix(r.URL.Path, ".sha256sum") {
			body = digest([]byte("expected"))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
	})}
	err := downloadFile(downloadSource{"https://getcomposer.org/download/1/composer.phar", "publisher"}, filepath.Join(t.TempDir(), "tool"), "test")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal(err)
	}
}

func TestLivePublisherMetadata(t *testing.T) {
	if os.Getenv("LARASERVE_TEST_CHECKSUMS") != "1" {
		t.Skip("opt-in publisher metadata integration test")
	}
	sources := []string{
		"https://github.com/dunglas/frankenphp/releases/download/v1.13.4/frankenphp-linux-x86_64",
		"https://github.com/heera/php-binaries/releases/download/php-8.1/php-8.1.34-cli-linux-x86_64.tar.gz",
		"https://getcomposer.org/download/2.11.1/composer.phar",
		"https://files.phpmyadmin.net/phpMyAdmin/5.2.3/phpMyAdmin-5.2.3-all-languages.tar.gz",
		"https://cdn.mysql.com/Downloads/MySQL-8.4/mysql-8.4.13-linux-glibc2.28-x86_64-minimal.tar.xz",
	}
	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			_, sum, err := publisherChecksum(source)
			if err != nil {
				t.Fatal(err)
			}
			if sum == "" {
				t.Fatal("empty checksum")
			}
		})
	}
}
