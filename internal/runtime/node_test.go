package runtime

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNodePublisherChecksum(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	calls := 0
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://nodejs.org/dist/v22.1.0/SHASUMS256.txt" {
			t.Fatalf("wrong checksum URL %s", r.URL)
		}
		body := digest([]byte("node")) + "  node-v22.1.0-linux-x64.tar.gz\n"
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	_, sum, err := publisherChecksum("https://nodejs.org/dist/v22.1.0/node-v22.1.0-linux-x64.tar.gz")
	if err != nil || sum != digest([]byte("node")) || calls != 1 {
		t.Fatal(sum, err, calls)
	}
	if _, _, err = publisherChecksum("https://nodejs.org/dist/v22.1.0/missing.tar.gz"); err == nil {
		t.Fatal("accepted missing checksum")
	}
}
func TestNodeCatalogOfflineAndExactPatches(t *testing.T) {
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	body := `[{"version":"v26.0.0-alpha.1","lts":false,"files":["linux-x64","linux-arm64","osx-x64-tar","osx-arm64-tar","win-x64-zip"]},{"version":"v24.2.0","lts":false,"files":["linux-x64","linux-arm64","osx-x64-tar","osx-arm64-tar","win-x64-zip"]},{"version":"v22.15.0","lts":"Jod","files":["linux-x64","linux-arm64","osx-x64-tar","osx-arm64-tar","win-x64-zip"]}]`
	calls := 0
	downloadClient = &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	root := filepath.Join(t.TempDir(), "new-cache")
	got, err := NodeVersions(root)
	if err != nil || len(got) != 2 || got[1] != "22.15.0" {
		t.Fatal(got, err)
	}
	if _, err = os.Stat(filepath.Join(root, "node-releases.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = NodeVersions(root); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
}
