package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"laraserve/internal/supervisor"
	"laraserve/internal/ui"
)

func cmdProjects(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("usage: laraserve projects")
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	store, err := projectStore(home)
	if err != nil {
		return err
	}
	list, err := store.List()
	if err != nil {
		return err
	}
	if len(list) == 0 {
		ui.Info("No projects registered; use serve or start with a project path")
		return nil
	}
	for _, p := range list {
		status, err := supervisor.Status(home.Root, p.ID)
		if err != nil {
			return err
		}
		ui.Fields(p.Name, ui.Field{Label: "ID", Value: p.ID}, ui.Field{Label: "State", Value: status.Status}, ui.Field{Label: "Local", Value: status.URL}, ui.Field{Label: "PHP", Value: orDash(p.PHP)}, ui.Field{Label: "Database", Value: orDash(p.Database)}, ui.Field{Label: "Project", Value: p.Source})
	}
	return nil
}
func cmdStatus(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: laraserve status [path|name|ID]")
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	store, err := projectStore(home)
	if err != nil {
		return err
	}
	p, err := store.Find(target)
	if err != nil {
		return err
	}
	state, err := supervisor.Status(home.Root, p.ID)
	if err != nil {
		return err
	}
	ui.Fields(p.Name, ui.Field{Label: "ID", Value: p.ID}, ui.Field{Label: "State", Value: state.Status}, ui.Field{Label: "Local", Value: state.URL}, ui.Field{Label: "PHP", Value: orDash(p.PHP)}, ui.Field{Label: "Database", Value: orDash(p.Database)}, ui.Field{Label: "Project", Value: p.Source})
	return nil
}
func cmdStop(args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: laraserve stop [path|name|ID]")
	}
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	store, err := projectStore(home)
	if err != nil {
		return err
	}
	p, err := store.Find(target)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err = supervisor.Stop(ctx, home.Root, p.ID); err != nil {
		return err
	}
	ui.OK("%s stopped", p.Name)
	return nil
}
func cmdStart(args []string, restart bool) error {
	options := newServeOptions()
	if err := parse(options.fs, args); err != nil {
		return err
	}
	if options.fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve start [path|name|ID] [serve flags]")
	}
	target := "."
	if options.fs.NArg() == 1 {
		target = options.fs.Arg(0)
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	store, err := projectStore(home)
	if err != nil {
		return err
	}
	previous, findErr := store.Find(target)
	source := ""
	var launchArgs []string
	if findErr == nil {
		source = previous.Source
		launchArgs = append([]string{}, previous.Args...)
		// Explicit flags override remembered settings. Positional paths are not added twice.
		overrides := rememberStartArgs(options)
		launchArgs = append(launchArgs, overrides...)
	} else {
		if restart {
			return findErr
		}
		source, err = projects.Canonical(target)
		if err != nil {
			return findErr
		}
		launchArgs = append([]string{source}, rememberStartArgs(options)...)
	}
	planned := projects.Project{Source: source, Root: source, Args: rememberServeArgs(options.fs, source), HTTPS: !*options.httpF, Domain: *options.domainF, Port: *options.portF}
	if previous != nil {
		planned = *previous
	}
	ui.Info("Starting %s in the background", filepath.Base(source))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := supervisor.Launch(ctx, supervisor.LaunchRequest{Home: home.Root, Project: planned, Args: launchArgs, Restart: restart})
	if errors.Is(err, context.DeadlineExceeded) && result.State.Status == "starting" {
		ui.Info("Still preparing; check `laraserve status %s` and %s", result.State.ID, result.LogPath)
		return nil
	}
	if err != nil {
		return err
	}
	ui.OK("%s started at %s", filepath.Base(source), result.State.URL)
	return nil
}
