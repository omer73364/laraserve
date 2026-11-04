package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"time"

	"laraserve/internal/appsettings"
	"laraserve/internal/filelock"
	"laraserve/internal/projects"
	"laraserve/internal/statefile"
	"laraserve/internal/supervisor"
	"laraserve/internal/ui"
)

func cmdAutostart(args []string) error {
	fs := flag.NewFlagSet("__autostart", flag.ContinueOnError)
	home := fs.String("home", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !filepath.IsAbs(*home) || fs.NArg() != 0 {
		return fmt.Errorf("invalid startup data path")
	}
	if err := os.Setenv("LARASERVE_HOME", *home); err != nil {
		return err
	}
	doc, err := appsettings.Read(*home)
	if err != nil {
		return err
	}
	if !doc.Config.LaunchAtLogin {
		return nil
	}
	if doc.Config.OpenDashboard {
		return cmdDashboard([]string{"--open"})
	}
	runStartupProjects(context.Background(), *home)
	return nil
}
func runStartupProjects(ctx context.Context, home string) {
	// Multiple open dashboards must not race each other to launch selected apps.
	release, ok, err := filelock.Try(filepath.Join(home, "auto-launch.lock"))
	if err != nil || !ok {
		return
	}
	defer release()
	doc, err := appsettings.Read(home)
	if err != nil {
		ui.Warn("Startup preferences: %v", err)
		return
	}
	list, err := projects.New(home).List()
	if err != nil {
		return
	}
	type item struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"state"`
		Error string `json:"error,omitempty"`
	}
	result := []item{}
	for _, p := range list {
		if !doc.Config.Projects[p.ID] {
			continue
		}
		state, err := supervisor.Status(home, p.ID)
		if err != nil {
			result = append(result, item{ID: p.ID, Name: p.Name, State: "failed", Error: err.Error()})
			continue
		}
		if state.Status == "running" || state.Status == "starting" || state.Status == "stopping" || state.Status == "unresponsive" {
			result = append(result, item{ID: p.ID, Name: p.Name, State: state.Status})
			continue
		}
		launch, done := context.WithTimeout(ctx, 2*time.Minute)
		_, err = supervisor.Launch(launch, supervisor.LaunchRequest{Home: home, Project: *p, Args: append(projects.ReusableArgs(p), "--setup-local")})
		done()
		entry := item{ID: p.ID, Name: p.Name, State: "running"}
		if err != nil {
			entry.State = "failed"
			entry.Error = err.Error()
			ui.Warn("Startup %s: %v", p.Name, err)
		}
		result = append(result, entry)
	}
	_ = statefile.WriteJSON(filepath.Join(home, "startup-report.json"), map[string]any{"checked": time.Now().UTC().Format(time.RFC3339), "projects": result})
}
func openDashboardURL(url string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	default:
		return fmt.Errorf("open the printed dashboard link in your browser")
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
