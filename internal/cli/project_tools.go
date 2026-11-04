package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"laraserve/internal/paths"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/projecttools"
	"laraserve/internal/ui"
)

func resolveToolProject(target string) (*paths.Home, *projects.Project, error) {
	home, err := paths.New()
	if err != nil {
		return nil, nil, err
	}
	store, err := projectStore(home)
	if err != nil {
		return nil, nil, err
	}
	p, err := store.Find(target)
	return home, p, err
}
func cmdSettings(args []string) error {
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	file := fs.String("file", "", "validated replacement JSON configuration")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve settings [project] [--file config.json]")
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	home, p, err := resolveToolProject(target)
	if err != nil {
		return err
	}
	_, err = projecttools.SettingsRoot(p)
	if err != nil {
		return err
	}
	doc, err := projecttools.ReadSettings(p)
	if err != nil {
		return err
	}
	if *file != "" {
		b, err := projectconfig.ReadRegular(*file, 64<<10)
		if err != nil {
			return err
		}
		var c projectconfig.Config
		decoder := json.NewDecoder(strings.NewReader(string(b)))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&c); err != nil {
			return err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return fmt.Errorf("unexpected trailing JSON")
		}
		doc, err = projecttools.SaveSettings(context.Background(), home.Root, p, doc.Revision, c)
		if err != nil {
			return err
		}
		ui.Info("Saved; restart to apply settings")
	}
	return json.NewEncoder(os.Stdout).Encode(doc)
}
func cmdEnvironment(args []string) error {
	fs := flag.NewFlagSet("env", flag.ContinueOnError)
	file := fs.String("file", "", "private JSON map of variable changes; omitted keys stay unchanged")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve env [project] [--file changes.json]")
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	home, p, err := resolveToolProject(target)
	if err != nil {
		return err
	}
	root, err := projecttools.SettingsRoot(p)
	if err != nil {
		return err
	}
	doc, err := projectconfig.ReadEnvironment(root)
	if err != nil {
		return err
	}
	if *file != "" {
		b, e := projectconfig.ReadRegular(*file, 128<<10)
		if e != nil {
			return e
		}
		var changes map[string]string
		if err = json.Unmarshal(b, &changes); err != nil {
			return err
		}
		doc, err = projecttools.SaveEnvironment(context.Background(), home.Root, p, doc.Revision, changes)
		if err != nil {
			return err
		}
		ui.Info("Saved; restart to apply environment changes")
	}
	return json.NewEncoder(os.Stdout).Encode(doc)
}
func cmdProjectTask(args []string, kind string) error {
	split := -1
	for i, arg := range args {
		if arg == "--" {
			split = i
			break
		}
	}
	if split < 0 {
		return fmt.Errorf("usage: laraserve %s [project] [--confirm-schema] -- command [arguments]", kind)
	}
	fs := flag.NewFlagSet(kind, flag.ContinueOnError)
	confirm := fs.Bool("confirm-schema", false, "explicitly allow a schema-changing Artisan command")
	if err := parse(fs, args[:split]); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("choose one project")
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	home, p, err := resolveToolProject(target)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	task, err := projecttools.StartTask(ctx, home.Root, projecttools.TaskRequest{ProjectID: p.ID, Kind: kind, Args: args[split+1:], ConfirmSchema: *confirm})
	if err != nil {
		return err
	}
	return waitTask(ctx, home.Root, task.ID)
}
func waitTask(ctx context.Context, home, id string) error {
	var offset int64
	for {
		task, err := projecttools.ReadTask(home, id)
		if err != nil {
			return err
		}
		if f, e := os.Open(filepath.Join(home, "tasks", id+".log")); e == nil {
			_, _ = f.Seek(offset, io.SeekStart)
			n, _ := io.Copy(os.Stdout, f)
			offset += n
			f.Close()
		}
		if task.Status != "queued" && task.Status != "running" && task.Status != "recovering" {
			if task.ExitCode != 0 {
				return fmt.Errorf("task %s %s (exit %d): %s", id, task.Status, task.ExitCode, task.Error)
			}
			ui.OK("Task completed (exit 0)")
			return nil
		}
		select {
		case <-ctx.Done():
			cancel, stop := context.WithTimeout(context.Background(), 5*time.Second)
			_ = projecttools.CancelTask(cancel, home, id)
			stop()
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
}
func cmdRuntimes(args []string) error {
	home, err := paths.New()
	if err != nil {
		return err
	}
	if len(args) == 0 || len(args) == 1 && args[0] == "list" {
		inventory, err := projecttools.RuntimeInventory(home.Root)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(inventory)
	}
	if len(args) == 3 && args[0] == "install" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		task, err := projecttools.StartTask(ctx, home.Root, projecttools.TaskRequest{Kind: "runtime", Args: args[1:]})
		if err != nil {
			return err
		}
		return waitTask(ctx, home.Root, task.ID)
	}
	if len(args) == 3 && args[0] == "remove" && args[2] == "--confirm" {
		return projecttools.RemoveUnusedCache(context.Background(), home.Root, args[1])
	}
	return fmt.Errorf("usage: laraserve runtimes [list | install php|mysql VERSION | remove CACHE_KEY --confirm]")
}

func cmdTask(args []string) error {
	fs := flag.NewFlagSet("task", flag.ContinueOnError)
	cancelF := fs.Bool("cancel", false, "cancel the owned task")
	waitF := fs.Bool("wait", false, "stream output until exit")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: laraserve task ID [--wait | --cancel]")
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	id := fs.Arg(0)
	if *cancelF {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return projecttools.CancelTask(ctx, home.Root, id)
	}
	if *waitF {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return waitTask(ctx, home.Root, id)
	}
	task, err := projecttools.ReadTask(home.Root, id)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(task)
}
