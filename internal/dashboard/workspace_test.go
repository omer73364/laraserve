package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceEndpointsAreProtected(t *testing.T) {
	s, _ := fixture(t)
	for _, path := range []string{"/api/folders", "/api/projects/add"} {
		for _, tc := range []struct {
			auth, csrf bool
			origin     string
			want       int
		}{{false, false, s.origin, 401}, {true, false, s.origin, 403}, {true, true, "http://evil.test", 403}} {
			w := call(s, "POST", path, `{}`, tc.auth, tc.csrf, tc.origin)
			if w.Code != tc.want {
				t.Fatalf("%s: got %d want %d", path, w.Code, tc.want)
			}
		}
		if w := call(s, "GET", path, "", true, false, ""); w.Code != 405 {
			t.Fatalf("%s allowed GET: %d", path, w.Code)
		}
	}
}
func TestFolderBrowserAndRegistration(t *testing.T) {
	s, _ := fixture(t)
	root := t.TempDir()
	for _, name := range []string{"z-app", "a-app", ".hidden"} {
		os.Mkdir(filepath.Join(root, name), 0700)
	}
	os.WriteFile(filepath.Join(root, "private.env"), []byte("never-list-this-secret"), 0600)
	body, _ := json.Marshal(map[string]any{"path": root})
	w := call(s, "POST", "/api/folders", string(body), true, true, s.origin)
	var listing folderListing
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &listing) != nil {
		t.Fatal(w.Code, w.Body.String())
	}
	if len(listing.Folders) != 2 || listing.Folders[0].Name != "a-app" || strings.Contains(w.Body.String(), "private.env") || strings.Contains(w.Body.String(), "never-list") {
		t.Fatal("incorrect folder listing", w.Body.String())
	}
	if _, err := browseFolders("relative", false); err == nil {
		t.Fatal("relative path accepted")
	}
	if _, err := browseFolders(filepath.Join(root, "private.env"), false); err == nil {
		t.Fatal("file accepted as folder")
	}
	hidden, err := browseFolders(root, true)
	if err != nil || len(hidden.Folders) != 3 {
		t.Fatal("hidden folder toggle failed", err)
	}
	app := filepath.Join(root, "a-app")
	os.WriteFile(filepath.Join(app, "artisan"), []byte("<?php"), 0600)
	os.WriteFile(filepath.Join(app, "composer.json"), []byte(`{"require":{"laravel/framework":"^12.0"}}`), 0600)
	body, _ = json.Marshal(map[string]string{"path": app})
	w = call(s, "POST", "/api/projects/add", string(body), true, true, s.origin)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "a-app") {
		t.Fatal(w.Code, w.Body.String())
	}
}
func TestLaravelVersionReportsItsSource(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"laravel/framework":"^12.0"}}`), 0600)
	if v, source := laravelVersion(root); v != "^12.0" || source != "composer.json constraint" {
		t.Fatal(v, source)
	}
	os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{"packages":[{"name":"laravel/framework","version":"v12.42.0"}]}`), 0600)
	if v, source := laravelVersion(root); v != "v12.42.0" || source != "composer.lock" {
		t.Fatal(v, source)
	}
	os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`invalid`), 0600)
	if v, _ := laravelVersion(root); v != "^12.0" {
		t.Fatal("invalid lock did not fall back", v)
	}
	os.WriteFile(filepath.Join(root, "composer.json"), []byte(`invalid`), 0600)
	if v, _ := laravelVersion(root); v != "" {
		t.Fatal("fabricated a version", v)
	}
}
func TestFolderListingIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 305; i++ {
		os.Mkdir(filepath.Join(root, strings.Repeat("x", i/100+1)+string(rune('a'+i%26))+string(rune('A'+i/26))), 0700)
	}
	listing, err := browseFolders(root, false)
	if err != nil || !listing.Truncated || len(listing.Folders) != 300 {
		t.Fatal("unbounded directory response", len(listing.Folders), listing.Truncated, err)
	}
}
