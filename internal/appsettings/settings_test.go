package appsettings

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreferencesRevisionAndProjectIsolation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux integration path; serializers tested separately")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	exe := filepath.Join(t.TempDir(), "client with spaces")
	initial, err := Read(home)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := Change(context.Background(), home, func(c *Config) error { c.Projects["abcdef123456"] = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	next := updated.Config
	next.LaunchAtLogin = true
	if _, err = Save(context.Background(), home, exe, initial.Revision, next); err == nil {
		t.Fatal("stale preferences accepted")
	}
	path, _ := LoginPath(home)
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale save wrote integration")
	}
	saved, err := Save(context.Background(), home, exe, updated.Revision, next)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.Config.Projects["abcdef123456"] {
		t.Fatal("application save dropped project preference")
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(b), "__autostart --home") {
		t.Fatal(string(b), err)
	}
	next = saved.Config
	next.LaunchAtLogin = false
	if _, err = Save(context.Background(), home, exe, saved.Revision, next); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("login entry was not removed")
	}
}
func TestLoginSerializationAndUnsafePaths(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "client \"&$% binary")
	home := t.TempDir()
	linux, err := LoginContent("linux", exe, home)
	if err != nil || !strings.Contains(linux, "%%") || !strings.Contains(linux, "Terminal=false") {
		t.Fatal(linux, err)
	}
	plist, err := LoginContent("darwin", exe, home)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(strings.NewReader(plist))
	for {
		_, err = decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid plist", err)
		}
	}

	for _, bad := range []string{"relative", "/bad\ncommand"} {
		if _, err = LoginContent("linux", bad, home); err == nil {
			t.Fatal("unsafe executable accepted")
		}
	}
}
func TestUpdateCheckUsesPublishedVersion(t *testing.T) {
	for _, tc := range []struct {
		code          int
		body, current string
		available     bool
		bad           bool
	}{{200, `{"tag_name":"v0.2.0"}`, "0.1.0", true, false}, {200, `{"tag_name":"v0.1.0"}`, "0.2.0", false, false}, {404, ``, "0.1.0", false, false}, {429, ``, "0.1.0", false, true}, {200, `{"tag_name":"not-a-version"}`, "0.1.0", false, true}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); io.WriteString(w, tc.body) }))
		result, err := fetchRelease(context.Background(), server.Client(), server.URL, tc.current)
		server.Close()
		if (err != nil) != tc.bad || result.Available != tc.available || result.URL != ReleasesURL {
			t.Fatalf("%+v %v", result, err)
		}
	}
}
