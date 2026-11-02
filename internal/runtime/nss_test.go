package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"laraserve/internal/certs"
)

func TestNSSPackagesArePinnedAndMatching(t *testing.T) {
	var packages []legacyPackage
	if err := json.Unmarshal(nssPackagesJSON, &packages); err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{}
	for _, p := range packages {
		if !strings.HasPrefix(p.Source.URL, "https://archive.ubuntu.com/") {
			t.Fatal(p.Source.URL)
		}
		if _, err := sourceOverride("NSS_TEST_CATALOG", p.Source); err != nil {
			t.Fatal(err)
		}
		versions[p.Name] = p.Version
	}
	if versions["libnss3"] == "" || versions["libnss3"] != versions["libnss3-tools"] {
		t.Fatal("NSS tools and libraries do not match")
	}
}

func TestRealPrivateNSSBrowserTrust(t *testing.T) {
	if os.Getenv("LARASERVE_TEST_NSS") != "1" {
		t.Skip("set LARASERVE_TEST_NSS=1 for real NSS download and temporary browser DB test")
	}
	root := t.TempDir()
	t.Setenv("PATH", "/nonexistent")
	tool, err := EnsureNSS(filepath.Join(root, "runtimes"))
	if err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(root, "browser-profile")
	if err := os.MkdirAll(db, 0700); err != nil {
		t.Fatal(err)
	}
	ca, _, err := certs.EnsureCA(filepath.Join(root, "ca"), certs.DefaultPermitted)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(tool, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("certutil %v: %v %s", args, err, out)
		}
		return string(out)
	}
	run("-N", "-d", "sql:"+db, "--empty-password")
	run("-A", "-d", "sql:"+db, "-n", certs.CAName, "-t", "C,,", "-i", ca.PEMPath)
	if list := run("-L", "-d", "sql:"+db); !strings.Contains(list, certs.CAName) || !strings.Contains(list, "C,,") {
		t.Fatalf("CA trust not imported: %s", list)
	}
	run("-D", "-d", "sql:"+db, "-n", certs.CAName)
	if list := run("-L", "-d", "sql:"+db); strings.Contains(list, certs.CAName) {
		t.Fatal("CA was not removed")
	}
	old := downloadClient
	t.Cleanup(func() { downloadClient = old })
	downloadClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("warm NSS cache attempted download")
		return nil, fmt.Errorf("offline")
	})}
	if again, err := EnsureNSS(filepath.Join(root, "runtimes")); err != nil || again != tool {
		t.Fatalf("offline reuse: %s %v", again, err)
	}
	t.Log("Private certutil initialized an NSS database, imported/listed/removed a CA, and reused its cache offline without host tools")
}
