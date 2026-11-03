// Package dashboard exposes existing project controls through a loopback-only UI.
package dashboard

import (
	"context"
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"laraserve/internal/metrics"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
)

//go:embed assets/*
var assets embed.FS

type Operation struct {
	Action  string `json:"action"`
	Pending bool   `json:"pending"`
	Error   string `json:"error,omitempty"`
}
type View struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Source    string         `json:"source"`
	PHP       string         `json:"php"`
	Database  string         `json:"database"`
	URL       string         `json:"url"`
	Status    string         `json:"status"`
	Health    string         `json:"health"`
	Error     string         `json:"error,omitempty"`
	Operation *Operation     `json:"operation,omitempty"`
	Resources metrics.Sample `json:"resources"`
}
type Server struct {
	home, host, origin, bootstrap, cookie, csrf string
	ctx                                         context.Context
	mu                                          sync.Mutex
	operations                                  map[string]Operation
	sampler                                     *metrics.Sampler
}

// New requires the already-bound IPv4 loopback listener's address. No public
// address, forwarded Host header, or user-provided callback URL is accepted.
func New(ctx context.Context, home, address string) (*Server, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil || host != "127.0.0.1" {
		return nil, fmt.Errorf("dashboard requires IPv4 loopback")
	}
	return &Server{home: home, host: address, origin: "http://" + address, bootstrap: supervisor.NewID() + supervisor.NewID(), cookie: supervisor.NewID() + supervisor.NewID(), csrf: supervisor.NewID(), ctx: ctx, operations: map[string]Operation{}, sampler: metrics.New()}, nil
}
func (s *Server) URL() string { return s.origin + "/#" + s.bootstrap }
func equal(a, b string) bool  { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	if r.Host != s.host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.origin) {
		http.Error(w, "forbidden", 403)
		return
	}
	if r.URL.Path == "/session" {
		if r.Method != "POST" || r.Header.Get("Origin") != s.origin || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "forbidden", 403)
			return
		}
		var input struct {
			Token string `json:"token"`
		}
		if err := decode(w, r, &input); err != nil || !equal(input.Token, s.bootstrap) {
			http.Error(w, "forbidden", 403)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		w.WriteHeader(204)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		cookie, err := r.Cookie(s.cookieName())
		if err != nil || !equal(cookie.Value, s.cookie) {
			http.Error(w, "open the dashboard link from your terminal", 401)
			return
		}
		if r.Method != "GET" && (r.Header.Get("Origin") != s.origin || !equal(r.Header.Get("X-CSRF-Token"), s.csrf)) {
			http.Error(w, "forbidden", 403)
			return
		}
		s.api(w, r)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	switch r.URL.Path {
	case "/", "/app.js", "/style.css":
	default:
		http.NotFound(w, r)
		return
	}
	files, _ := fs.Sub(assets, "assets")
	http.FileServer(http.FS(files)).ServeHTTP(w, r)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("invalid body")
	}
	return nil
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	if s.globalTools(w, r) {
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/session" {
		respond(w, map[string]string{"csrf": s.csrf})
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/projects" {
		list, err := projects.New(s.home).List()
		if err != nil {
			http.Error(w, "cannot read project registry", 500)
			return
		}
		views := make([]View, 0, len(list))
		for _, p := range list {
			state, err := supervisor.Status(s.home, p.ID)
			v := View{ID: p.ID, Name: p.Name, Source: p.Source, PHP: p.PHP, Database: p.Database, Status: state.Status, Health: "Stopped", URL: safeURL(state.URL)}
			if err != nil {
				v.Status = "unknown"
				v.Error = "Cannot read project owner"
				v.Health = "Unknown"
			}
			switch v.Status {
			case "running":
				v.Health = "Owner reachable · server ready"
				v.Resources = s.sampler.Measure(p.ID, state.Generation, state.PID)
			case "starting":
				v.Health = "Preparing"
			case "stopping":
				v.Health = "Stopping"
			case "failed", "crashed":
				v.Health = "Needs attention"
			case "unresponsive":
				v.Health = "Owner unreachable"
			}
			s.mu.Lock()
			if op, ok := s.operations[p.ID]; ok {
				copy := op
				v.Operation = &copy
			}
			s.mu.Unlock()
			views = append(views, v)
		}
		respond(w, views)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/api/projects/") || len(parts) != 2 || !projects.ValidID(parts[0]) {
		http.NotFound(w, r)
		return
	}
	p, err := projects.New(s.home).Load(parts[0])
	if err != nil {
		http.Error(w, "project not found", 404)
		return
	}
	if s.projectTools(w, r, p, parts[1]) {
		return
	}
	if r.Method == "GET" && parts[1] == "logs" {
		output, err := s.logs(p, r.URL.Query().Get("source"), r.URL.Query().Get("q"))
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		respond(w, output)
		return
	}
	action := parts[1]
	if r.Method != "POST" || (action != "start" && action != "stop" && action != "restart") {
		http.NotFound(w, r)
		return
	}
	// Actions deliberately accept no flags, commands, migrations, or filesystem paths.
	var input struct{}
	if err := decode(w, r, &input); err != nil {
		http.Error(w, "expected an empty JSON object", 400)
		return
	}
	s.mu.Lock()
	if s.operations[p.ID].Pending {
		s.mu.Unlock()
		http.Error(w, "project action already in progress", 409)
		return
	}
	s.operations[p.ID] = Operation{Action: action, Pending: true}
	s.mu.Unlock()
	go s.perform(p, action)
	w.WriteHeader(202)
}
func (s *Server) perform(p *projects.Project, action string) {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	var err error
	if action == "stop" {
		err = supervisor.Stop(ctx, s.home, p.ID)
	} else {
		args := reusableArgs(p)
		_, err = supervisor.Launch(ctx, supervisor.LaunchRequest{Home: s.home, Project: *p, Args: args, Restart: action == "restart"})
	}
	op := Operation{Action: action}
	if err != nil {
		op.Error = err.Error()
	}
	s.mu.Lock()
	s.operations[p.ID] = op
	s.mu.Unlock()
}
func reusableArgs(p *projects.Project) []string {
	// Registry records are local, but old or hand-edited records must not cause
	// one-shot actions to be replayed by the dashboard.
	args := []string{p.Source}
	for i := 1; i < len(p.Args); i++ {
		arg := p.Args[i]
		key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch key {
		case "migrate", "yes", "y", "allow-insecure-dependencies":
			if !strings.Contains(arg, "=") && i+1 < len(p.Args) && (p.Args[i+1] == "true" || p.Args[i+1] == "false") {
				i++
			}
			continue
		}
		args = append(args, arg)
	}
	return args
}
func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return ""
	}
	return u.String()
}

func (s *Server) cookieName() string {
	return "laraserve_dashboard_" + strings.ReplaceAll(s.host, ":", "_")
}
