package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
	"time"

	"laraserve/internal/appsettings"
	"laraserve/internal/filelock"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
	"path/filepath"
)

type uninstallState struct {
	Pending  bool   `json:"pending"`
	Complete bool   `json:"complete"`
	Error    string `json:"error,omitempty"`
	Output   string `json:"output,omitempty"`
}
type cappedOutput struct{ text string }

func (w *cappedOutput) Write(b []byte) (int, error) {
	n := len(b)
	if len(w.text) < 65536 {
		limit := 65536 - len(w.text)
		if len(b) > limit {
			b = b[:limit]
		}
		w.text += string(b)
	}
	return n, nil
}
func (s *Server) Done() <-chan struct{} { return s.finished }
func (s *Server) applicationTools(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/application":
		exe, err := os.Executable()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return true
		}
		if r.Method == "GET" {
			doc, err := appsettings.Read(s.home)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return true
			}
			login, loginErr := appsettings.LoginPath(s.home)
			respond(w, map[string]any{"settings": doc, "version": runtime.Version, "platform": goruntime.GOOS + "/" + goruntime.GOARCH, "home": s.home, "executable": exe, "login_path": login, "login_supported": loginErr == nil, "updates": appsettings.ReadUpdate(s.home, runtime.Version)})
		} else if r.Method == "POST" {
			var input appsettings.Document
			if err := decode(w, r, &input); err != nil {
				http.Error(w, "invalid application settings", 400)
				return true
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			gate, e := filelock.Acquire(ctx, filepath.Join(s.home, "startup.lock"))
			if e != nil {
				http.Error(w, e.Error(), 409)
				return true
			}
			defer gate()
			doc, err := appsettings.Save(ctx, s.home, exe, input.Revision, input.Config)
			if err != nil {
				http.Error(w, err.Error(), 409)
			} else {
				respond(w, doc)
			}
		} else {
			http.Error(w, "method not allowed", 405)
		}
		return true
	case "/api/application/updates":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct{}
		if decode(w, r, &input) != nil {
			http.Error(w, "expected an empty request", 400)
			return true
		}
		result, err := appsettings.Check(r.Context(), s.home, runtime.Version, true)
		if err != nil {
			http.Error(w, err.Error(), 502)
		} else {
			respond(w, result)
		}
		return true
	case "/api/application/uninstall":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Confirm string `json:"confirm"`
			Data    bool   `json:"delete_managed_data"`
		}
		if decode(w, r, &input) != nil || input.Confirm != "UNINSTALL" || !input.Data {
			http.Error(w, "confirm removal of all managed data by typing UNINSTALL", 400)
			return true
		}
		s.mu.Lock()
		if s.maintenance {
			s.mu.Unlock()
			http.Error(w, "uninstall is already in progress", 409)
			return true
		}
		s.maintenance = true
		s.uninstall = uninstallState{Pending: true}
		s.mu.Unlock()
		go s.runUninstall()
		w.WriteHeader(202)
		return true
	case "/api/application/uninstall-status":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		s.mu.Lock()
		state := s.uninstall
		s.mu.Unlock()
		respond(w, state)
		return true
	}
	return false
}
func (s *Server) runUninstall() {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
	defer cancel()
	var output cappedOutput
	exe, err := os.Executable()
	if err == nil {
		cmd := exec.CommandContext(ctx, exe, "uninstall", "--yes")
		env := []string{}
		for _, v := range os.Environ() {
			if !strings.HasPrefix(v, "LARASERVE_HOME=") {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "LARASERVE_HOME="+s.home, "NO_COLOR=1")
		cmd.Stdout = &output
		cmd.Stderr = &output
		err = cmd.Run()
	}
	result := uninstallState{Complete: err == nil, Output: output.text}
	if err != nil {
		result.Error = fmt.Sprintf("Uninstall did not complete: %v. %s", err, strings.TrimSpace(output.text))
	}
	s.mu.Lock()
	s.uninstall = result
	if err != nil {
		s.maintenance = false
	}
	s.mu.Unlock()
	if err == nil {
		time.AfterFunc(3*time.Second, func() { close(s.finished) })
	}
}

// These two operations apply only to a registered project, never to its source files.
func (s *Server) projectStartup(ctx context.Context, id string, enabled bool) error {
	gate, err := filelock.Acquire(ctx, filepath.Join(s.home, "startup.lock"))
	if err != nil {
		return err
	}
	defer gate()
	p, e := projects.New(s.home).Load(id)
	if e != nil {
		return e
	}
	if p.Removed {
		return fmt.Errorf("project was removed from the workspace")
	}
	_, err = appsettings.Change(ctx, s.home, func(c *appsettings.Config) error {
		if enabled {
			c.Projects[id] = true
		} else {
			delete(c.Projects, id)
		}
		return nil
	})
	return err
}
