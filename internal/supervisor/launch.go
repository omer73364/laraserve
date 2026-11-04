package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"laraserve/internal/activity"
	"laraserve/internal/filelock"
	"laraserve/internal/paths"
	"laraserve/internal/projects"
)

type LaunchRequest struct {
	Home     string
	Project  projects.Project
	Args     []string
	Restart  bool
	Explicit []string
}

type LaunchResult struct {
	State   State
	LogPath string
}

// Launch is shared by CLI and future UI clients. Its context limits the wait,
// not the detached project's lifetime; Stop is the explicit lifetime action.
func Launch(ctx context.Context, request LaunchRequest) (LaunchResult, error) {
	home, source, args, restart := request.Home, request.Project.Source, request.Args, request.Restart
	canonical, err := projects.Canonical(source)
	if err != nil {
		return LaunchResult{}, err
	}
	id := paths.ProjectID(canonical)
	if err = os.MkdirAll(directory(home), 0700); err != nil {
		return LaunchResult{}, err
	}
	release, err := filelock.Acquire(ctx, filepath.Join(directory(home), id+"-launch.lock"))
	if err != nil {
		return LaunchResult{}, err
	}
	defer release()
	state, err := Status(home, id)
	if err != nil {
		return LaunchResult{}, err
	}
	if restart {
		if err = Stop(ctx, home, id); err != nil {
			return LaunchResult{}, err
		}
	} else if state.Status == "running" || state.Status == "starting" || state.Status == "stopping" || state.Status == "unresponsive" {
		return LaunchResult{}, fmt.Errorf("project already has an owner (%s); use status, stop, or restart", state.Status)
	}
	if err := ctx.Err(); err != nil {
		return LaunchResult{}, err
	}
	pending := request.Project
	pending.ID = id
	pending.Source = canonical
	if pending.Name == "" {
		pending.Name = strings.TrimSuffix(filepath.Base(canonical), ".laraserve")
	}
	if pending.Root == "" {
		pending.Root = canonical
	}
	if err = projects.New(home).Import(&pending); err != nil {
		return LaunchResult{}, err
	}
	if err = os.MkdirAll(directory(home), 0700); err != nil {
		return LaunchResult{}, err
	}
	logPath := filepath.Join(directory(home), id+"-startup.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return LaunchResult{}, err
	}
	defer log.Close()
	executable, err := os.Executable()
	if err != nil {
		return LaunchResult{}, err
	}
	generation := NewID()
	cmd := exec.Command(executable, append([]string{"__background-serve"}, args...)...)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.Env = workerEnv(home, generation)
	cmd.Env = append(cmd.Env, "NO_COLOR=1", "LARASERVE_SETTINGS_REPLAY=1", "LARASERVE_EXPLICIT_FLAGS="+strings.Join(request.Explicit, ","))
	detach(cmd)
	if err = cmd.Start(); err != nil {
		return LaunchResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err == nil {
				return LaunchResult{}, errors.New("background startup stopped before becoming ready")
			}
			return LaunchResult{}, errors.New(activity.Failure(logPath, "Project could not start: "+err.Error()))
		case <-ctx.Done():
			return LaunchResult{State: State{ID: id, Generation: generation, Status: "starting"}, LogPath: logPath}, ctx.Err()
		case <-ticker.C:
			state, err := Status(home, id)
			if err != nil {
				continue
			}
			if state.Generation != generation {
				continue
			}
			if state.Status == "running" {
				return LaunchResult{State: state, LogPath: logPath}, nil
			}
			if state.Status == "failed" || state.Status == "crashed" || state.Status == "stopped" {
				return LaunchResult{}, errors.New(activity.Failure(logPath, "Project preparation "+state.Status))
			}
		}
	}
}
func workerEnv(home, generation string) []string {
	var result []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "LARASERVE_HOME") || strings.EqualFold(name, "LARASERVE_START_ID") || strings.EqualFold(name, "LARASERVE_SETTINGS_REPLAY") || strings.EqualFold(name, "LARASERVE_EXPLICIT_FLAGS") {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "LARASERVE_HOME="+home, "LARASERVE_START_ID="+generation)
}
