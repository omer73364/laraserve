package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
	"laraserve/internal/versions"
)

func fixture(t *testing.T) (*Server, *projects.Project) {
	t.Helper()
	home := t.TempDir()
	root := t.TempDir()
	p := &projects.Project{ID: "abcdef123456", Name: "Demo <app>", Source: root, Root: root, PHP: "8.5", Database: "SQLite", Args: []string{root, "--http=true"}}
	if err := projects.New(home).Save(p); err != nil {
		t.Fatal(err)
	}
	s, err := New(context.Background(), home, "127.0.0.1:45678")
	if err != nil {
		t.Fatal(err)
	}
	return s, p
}
func call(s *Server, method, path, body string, auth, csrf bool, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if auth {
		r.AddCookie(&http.Cookie{Name: s.cookieName(), Value: s.cookie})
	}
	if csrf {
		r.Header.Set("X-CSRF-Token", s.csrf)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}
func TestAccessBoundaries(t *testing.T) {
	s, _ := fixture(t)
	tests := []struct {
		method, path, body, origin string
		auth, csrf                 bool
		want                       int
	}{
		{"GET", "/", "", "", false, false, 200},
		{"GET", "/api/projects", "", "", false, false, 401},
		{"GET", "/api/projects", "", "http://evil.test", true, false, 403},
		{"POST", "/session", fmt.Sprintf(`{"token":%q}`, s.bootstrap), "", false, false, 403},
		{"POST", "/session", `{"token":"wrong"}`, s.origin, false, false, 403},
		{"POST", "/session", fmt.Sprintf(`{"token":%q}`, s.bootstrap), s.origin, false, false, 204},
		{"GET", "/api/session", "", "", true, false, 200},
		{"POST", "/api/projects/abcdef123456/stop", "{}", s.origin, true, false, 403},
		{"POST", "/api/projects/abcdef123456/stop", "{}", "", true, true, 403},
		{"POST", "/api/projects/abcdef123456/stop", `{"migrate":true}`, s.origin, true, true, 400},
		{"POST", "/api/projects/abcdef123456/stop", "{} {}", s.origin, true, true, 400},
		{"GET", "/api/projects/../../manifest.json/logs", "", "", true, false, 404},
	}
	for _, test := range tests {
		w := call(s, test.method, test.path, test.body, test.auth, test.csrf, test.origin)
		if w.Code != test.want {
			t.Errorf("%s %s got %d want %d: %s", test.method, test.path, w.Code, test.want, w.Body.String())
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("CORS enabled")
		}
	}
	r := httptest.NewRequest("GET", s.origin+"/", nil)
	r.Host = "evil.test"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("Host not checked")
	}
	w = call(s, "POST", "/session", fmt.Sprintf(`{"token":%q}`, s.bootstrap), false, false, s.origin)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal(cookies)
	}
	if strings.Contains(w.Body.String(), s.bootstrap) {
		t.Fatal("bootstrap leaked")
	}
	w = call(s, "GET", "/api/projects", "", true, false, "")
	for _, secret := range []string{"serve_args", "control_token", "control_address", s.cookie, s.csrf, s.bootstrap} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("private control data exposed", secret)
		}
	}
}

func TestProjectViewUsesRecordedNodeSelection(t *testing.T) {
	s, p := fixture(t)
	selection := &versions.NodeSelection{Version: "22.23.0", Source: ".nvmrc", Pin: "22", Engine: ">=20"}
	path := filepath.Join(s.home, "runtimes", "node-project-"+paths.ProjectID(p.Source)+".json")
	if err := versions.WriteLock(path, versions.Pins{Node: selection.Version, NodeSelection: selection}); err != nil {
		t.Fatal(err)
	}
	response := call(s, "GET", "/api/projects", "", true, false, "")
	var views []View
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Node != selection.Version || views[0].NodeReason != selection.Explanation() {
		t.Fatalf("%+v", views)
	}
	// A settings edit must not pretend that a new Node version is already in use.
	if err := os.WriteFile(filepath.Join(p.Source, ".nvmrc"), []byte("24"), 0600); err != nil {
		t.Fatal(err)
	}
	response = call(s, "GET", "/api/projects", "", true, false, "")
	if err := json.Unmarshal(response.Body.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if views[0].Node != "22.23.0" {
		t.Fatal("reported an unprepared version as selected")
	}
}
func TestDashboardStopUsesOwnerControl(t *testing.T) {
	s, p := fixture(t)
	session, err := supervisor.Begin(context.Background(), s.home, p.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close(nil)
	if err = session.Ready("http://localhost:8080"); err != nil {
		t.Fatal(err)
	}
	w := call(s, "GET", "/api/projects", "", true, false, "")
	var views []View
	if err = json.Unmarshal(w.Body.Bytes(), &views); err != nil || len(views) != 1 || views[0].Status != "running" {
		t.Fatal(w.Body.String(), err)
	}
	w = call(s, "POST", "/api/projects/"+p.ID+"/stop", "{}", true, true, s.origin)
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-session.Context().Done():
		session.Close(nil)
	case <-time.After(3 * time.Second):
		t.Fatal("owner did not receive stop")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		op := s.operations[p.ID]
		s.mu.Unlock()
		if !op.Pending {
			if op.Error != "" {
				t.Fatal(op.Error)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stop did not finish")
}
func TestLogSelectionBoundariesAndSearch(t *testing.T) {
	s, p := fixture(t)
	dir := filepath.Join(p.Root, "storage", "logs")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "laravel.log"), []byte("alpha\nERROR <script>alert(1)</script>\nlast\n"), 0600)
	output, err := s.logs(p, "laravel", "error")
	if err != nil || !strings.Contains(output.Text, "<script>") || strings.Contains(output.Text, "alpha") {
		t.Fatal(output, err)
	}
	// Arbitrary sources and path queries never select files.
	if _, err = s.logs(p, "../../.env", ""); err == nil {
		t.Fatal("arbitrary log accepted")
	}
	outside := filepath.Join(t.TempDir(), "secret.log")
	os.WriteFile(outside, []byte("secret"), 0600)
	if err = os.Symlink(outside, filepath.Join(dir, "outside.log")); err == nil {
		if _, err = s.logs(p, "laravel", ""); err == nil {
			t.Fatal("symlink escaped project")
		}
		os.Remove(filepath.Join(dir, "outside.log"))
	}
	os.WriteFile(filepath.Join(dir, "laravel.log"), []byte(strings.Repeat("line\n", logLimit)), 0600)
	output, err = s.logs(p, "laravel", "")
	if err != nil || !output.Truncated || len(output.Text) > logLimit {
		t.Fatal(len(output.Text), output.Truncated, err)
	}
	if _, err = s.logs(p, "laravel", strings.Repeat("q", 257)); err == nil {
		t.Fatal("unbounded query")
	}
}
func TestOneShotOptionsNeverReplayed(t *testing.T) {
	p := &projects.Project{Source: "/demo", Args: []string{"/demo", "--http=true", "--migrate=true", "--yes", "--allow-insecure-dependencies=true", "--mysql=false"}}
	args := strings.Join(reusableArgs(p), " ")
	if strings.Contains(args, "migrate") || strings.Contains(args, "yes") || strings.Contains(args, "insecure") {
		t.Fatal(args)
	}
	if !strings.Contains(args, "--mysql=false") {
		t.Fatal(args)
	}
}

func TestLegacyServerAndVersionedDatabaseLogSelection(t *testing.T) {
	s, p := fixture(t)
	const legacy = "123456abcdef"
	footprint := `{"projects":{"` + legacy + `":{"path":` + fmt.Sprintf("%q", p.Root) + `}}}`
	if err := os.WriteFile(filepath.Join(s.home, "manifest.json"), []byte(footprint), 0600); err != nil {
		t.Fatal(err)
	}
	p.MySQL = "8.4.13"
	streams := map[string]string{
		"server":  filepath.Join(s.home, "projects", legacy, "logs", "server.log"),
		"mysql":   filepath.Join(s.home, "databases", legacy, "mysql-8.4.13", "server.log"),
		"startup": filepath.Join(s.home, "control", p.ID+"-startup.log"),
	}
	for source, file := range streams {
		os.MkdirAll(filepath.Dir(file), 0700)
		os.WriteFile(file, []byte(source+" marker"), 0600)
		output, err := s.logs(p, source, "")
		if err != nil || output.Text != source+" marker" {
			t.Fatal(source, output, err)
		}
	}
	oldDir := filepath.Join(s.home, "databases", legacy)
	os.MkdirAll(filepath.Join(oldDir, "data"), 0700)
	os.WriteFile(filepath.Join(oldDir, "server.log"), []byte("old database log"), 0600)
	output, err := s.logs(p, "mysql", "")
	if err != nil || output.Text != "old database log" {
		t.Fatal(output, err)
	}
}
