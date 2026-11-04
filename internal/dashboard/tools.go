package dashboard

import (
	"context"
	"net/http"
	"strings"
	"time"

	"laraserve/internal/activity"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/projecttools"
	"laraserve/internal/runtime"
	"laraserve/internal/scaffold"
	"laraserve/internal/supervisor"
	"path/filepath"
)

func (s *Server) globalTools(w http.ResponseWriter, r *http.Request) bool {
	if s.applicationTools(w, r) {
		return true
	}
	if s.workspaceTools(w, r) {
		return true
	}
	switch r.URL.Path {
	case "/api/task-history":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		tasks, err := projecttools.ListTasks(s.home, "")
		if err != nil {
			http.Error(w, err.Error(), 500)
		} else {
			respond(w, tasks)
		}
		return true
	case "/api/catalog":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		respond(w, map[string]any{"php": runtime.PHPVersions(), "mysql": runtime.MySQLVersions()})
		return true
	case "/api/runtimes":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		inventory, err := projecttools.RuntimeInventory(s.home)
		if err != nil {
			http.Error(w, err.Error(), 400)
		} else {
			respond(w, inventory)
		}
		return true
	case "/api/runtimes/install":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Kind    string `json:"kind"`
			Version string `json:"version"`
		}
		if err := decode(w, r, &input); err != nil {
			http.Error(w, "invalid runtime request", 400)
			return true
		}
		task, err := projecttools.StartTask(r.Context(), s.home, projecttools.TaskRequest{Kind: "runtime", Args: []string{input.Kind, input.Version}})
		if err != nil {
			http.Error(w, err.Error(), 400)
		} else {
			respond(w, task)
		}
		return true
	case "/api/runtimes/remove":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Key     string `json:"key"`
			Confirm bool   `json:"confirm"`
		}
		if err := decode(w, r, &input); err != nil || !input.Confirm {
			http.Error(w, "explicit cache removal confirmation required", 400)
			return true
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if err := projecttools.RemoveUnusedCache(ctx, s.home, input.Key); err != nil {
			http.Error(w, err.Error(), 409)
		} else {
			w.WriteHeader(204)
		}
		return true
	case "/api/create":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input scaffold.Request
		if err := decode(w, r, &input); err != nil {
			http.Error(w, "invalid creation options", 400)
			return true
		}
		task, err := projecttools.StartTask(r.Context(), s.home, projecttools.TaskRequest{Kind: "create", Create: &input})
		if err != nil {
			http.Error(w, err.Error(), 400)
		} else {
			respond(w, task)
		}
		return true
	}
	if strings.HasPrefix(r.URL.Path, "/api/tasks/") {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/tasks/"), "/")
		if len(parts) > 2 || !projects.ValidID(parts[0]) {
			http.NotFound(w, r)
			return true
		}
		if len(parts) == 1 && r.Method == "GET" {
			task, err := projecttools.ReadTask(s.home, parts[0])
			if err != nil {
				http.Error(w, "task not found", 404)
			} else {
				respond(w, task)
			}
			return true
		}
		if len(parts) == 2 && parts[1] == "cancel" && r.Method == "POST" {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if err := projecttools.CancelTask(ctx, s.home, parts[0]); err != nil {
				http.Error(w, err.Error(), 409)
			} else {
				w.WriteHeader(204)
			}
			return true
		}
		http.NotFound(w, r)
		return true
	}
	return false
}
func (s *Server) projectTools(w http.ResponseWriter, r *http.Request, p *projects.Project, tool string) bool {
	switch tool {
	case "progress":
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		s.mu.Lock()
		op := s.operations[p.ID]
		s.mu.Unlock()
		state, _ := supervisor.Status(s.home, p.ID)
		output, err := s.logs(p, "startup", "")
		if err != nil {
			output.Text = ""
		}
		progress := activity.Read(filepath.Join(s.home, "control", p.ID+"-progress.json"))
		if op.Action == "stop" {
			status := "completed"
			if op.Pending {
				status = "running"
			}
			if op.Error != "" {
				status = "failed"
			}
			progress = activity.Progress{Steps: []activity.Step{{Name: "Stop app and services", Status: status}}, Error: op.Error}
		}
		respond(w, map[string]any{"operation": op, "progress": progress, "output": output.Text, "status": state.Status, "name": p.Name})
		return true
	case "startup":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Enabled bool `json:"enabled"`
		}
		if decode(w, r, &input) != nil {
			http.Error(w, "invalid startup preference", 400)
			return true
		}
		if err := s.projectStartup(r.Context(), p.ID, input.Enabled); err != nil {
			http.Error(w, err.Error(), 409)
		} else {
			w.WriteHeader(204)
		}
		return true
	case "settings":
		_, err := projecttools.SettingsRoot(p)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return true
		}
		if r.Method == "GET" {
			doc, err := projecttools.ReadSettings(p)
			if err != nil {
				http.Error(w, err.Error(), 400)
			} else {
				respond(w, doc)
			}
			return true
		}
		if r.Method == "POST" {
			var input projectconfig.Document
			if err = decode(w, r, &input); err != nil {
				http.Error(w, "invalid settings document", 400)
				return true
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			doc, err := projecttools.SaveSettings(ctx, s.home, p, input.Revision, input.Config)
			if err != nil {
				http.Error(w, err.Error(), 409)
			} else {
				respond(w, doc)
			}
			return true
		}
	case "environment":
		root, err := projecttools.SettingsRoot(p)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return true
		}
		if r.Method == "GET" {
			doc, err := projectconfig.ReadEnvironment(root)
			if err != nil {
				http.Error(w, err.Error(), 400)
			} else {
				respond(w, doc)
			}
			return true
		}
		if r.Method == "POST" {
			var input struct {
				Revision string            `json:"revision"`
				Changes  map[string]string `json:"changes"`
			}
			if err = decode(w, r, &input); err != nil {
				http.Error(w, "invalid environment changes", 400)
				return true
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			doc, err := projecttools.SaveEnvironment(ctx, s.home, p, input.Revision, input.Changes)
			if err != nil {
				http.Error(w, err.Error(), 409)
			} else {
				respond(w, doc)
			}
			return true
		}
	case "tasks":
		if r.Method == "GET" {
			tasks, err := projecttools.ListTasks(s.home, p.ID)
			if err != nil {
				http.Error(w, err.Error(), 500)
			} else {
				respond(w, tasks)
			}
			return true
		}
		if r.Method == "POST" {
			var input projecttools.TaskRequest
			if err := decode(w, r, &input); err != nil || input.Kind != "artisan" && input.Kind != "composer" && input.Kind != "npm" {
				http.Error(w, "choose Artisan, Composer or npm", 400)
				return true
			}
			input.ProjectID = p.ID
			task, err := projecttools.StartTask(r.Context(), s.home, input)
			if err != nil {
				http.Error(w, err.Error(), 400)
			} else {
				respond(w, task)
			}
			return true
		}
	default:
		return false
	}
	http.Error(w, "method not allowed", 405)
	return true
}
