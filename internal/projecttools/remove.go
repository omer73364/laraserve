package projecttools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"laraserve/internal/appsettings"
	"laraserve/internal/filelock"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
)

func RemoveProject(ctx context.Context, home, id string) error {
	if !projects.ValidID(id) {
		return fmt.Errorf("invalid project identifier")
	}
	auto, err := filelock.Acquire(ctx, filepath.Join(home, "auto-launch.lock"))
	if err != nil {
		return err
	}
	defer auto()
	if err = os.MkdirAll(taskDir(home), 0700); err != nil {
		return err
	}
	task, ok, err := filelock.Try(filepath.Join(taskDir(home), id+".work.lock"))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("wait for this project's command to finish before removing it")
	}
	defer task()
	gate, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return err
	}
	defer gate()
	if err = RequireNoTasks(home, id); err != nil {
		return err
	}
	return supervisor.WithStopped(ctx, home, id, func() error {
		if _, err := appsettings.Change(ctx, home, func(c *appsettings.Config) error { delete(c.Projects, id); return nil }); err != nil {
			return err
		}
		return projects.New(home).Archive(id)
	})
}
