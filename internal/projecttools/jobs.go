package projecttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"laraserve/internal/activity"
	"laraserve/internal/env"
	"laraserve/internal/filelock"
	"laraserve/internal/frontend"
	"laraserve/internal/paths"
	"laraserve/internal/prepare"
	"laraserve/internal/process"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
	"laraserve/internal/scaffold"
	"laraserve/internal/statefile"
	"laraserve/internal/supervisor"
	"laraserve/internal/versions"
)

type TaskRequest struct {
	ProjectID     string            `json:"project_id,omitempty"`
	Kind          string            `json:"kind"`
	Args          []string          `json:"args,omitempty"`
	ConfirmSchema bool              `json:"confirm_schema,omitempty"`
	Create        *scaffold.Request `json:"create,omitempty"`
}
type Task struct {
	Label       string            `json:"label"`
	ProjectName string            `json:"project_name,omitempty"`
	Progress    activity.Progress `json:"progress"`
	ID          string            `json:"id"`
	ProjectID   string            `json:"project_id,omitempty"`
	Kind        string            `json:"kind"`
	Status      string            `json:"status"`
	Started     string            `json:"started_utc"`
	Finished    string            `json:"finished_utc,omitempty"`
	ExitCode    int               `json:"exit_code"`
	Error       string            `json:"error,omitempty"`
	Output      string            `json:"output"`
	Offset      int64             `json:"offset"`
}

func taskDir(home string) string      { return filepath.Join(home, "tasks") }
func taskFile(home, id string) string { return filepath.Join(taskDir(home), id+".json") }
func validateTask(home string, r TaskRequest) error {
	if len(r.Args) > 40 {
		return fmt.Errorf("too many arguments")
	}
	for _, arg := range r.Args {
		if len(arg) > 1024 || strings.ContainsRune(arg, 0) {
			return fmt.Errorf("invalid task argument")
		}
	}
	switch r.Kind {
	case "artisan", "composer", "npm":
		if !projects.ValidID(r.ProjectID) {
			return fmt.Errorf("select a registered project")
		}
		if p, err := projects.New(home).Load(r.ProjectID); err != nil {
			return err
		} else if p.Removed {
			return fmt.Errorf("project was removed from the workspace")
		}
		if len(r.Args) == 0 || strings.HasPrefix(r.Args[0], "-") {
			return fmt.Errorf("choose a command")
		}
		if r.Kind == "artisan" {
			schema := strings.HasPrefix(r.Args[0], "migrate") && r.Args[0] != "migrate:status" || r.Args[0] == "db:wipe" || r.Args[0] == "db:seed" || strings.HasPrefix(r.Args[0], "schema:")
			if schema && !r.ConfirmSchema {
				return fmt.Errorf("schema-changing Artisan commands require explicit confirmation")
			}
			if r.Args[0] == "serve" || r.Args[0] == "tinker" || r.Args[0] == "queue:work" || r.Args[0] == "schedule:work" {
				return fmt.Errorf("interactive or persistent workers are not supported as tasks")
			}
		} else if r.Kind == "npm" {
			command := strings.Join(r.Args, " ")
			if command != "install" && command != "ci" && command != "run build" && command != "audit" && command != "outdated" {
				return fmt.Errorf("choose install, ci, run build, audit or outdated")
			}
		} else {
			switch r.Args[0] {
			case "install", "update", "dump-autoload", "validate", "audit", "check-platform-reqs", "show":
			default:
				return fmt.Errorf("unsupported Composer action")
			}
			for _, arg := range r.Args[1:] {
				if !strings.HasPrefix(arg, "--") || strings.ContainsAny(arg, "=\r\n") {
					return fmt.Errorf("Composer task accepts simple switches only")
				}
				switch arg {
				case "--dev", "--no-dev", "--optimize-autoloader", "--strict", "--locked", "--no-check-publish":
				default:
					return fmt.Errorf("unsupported Composer switch %s", arg)
				}
			}
		}
	case "runtime":
		if len(r.Args) != 2 || (r.Args[0] != "php" && r.Args[0] != "mysql") {
			return fmt.Errorf("choose php or mysql and an exact catalog version")
		}
		catalog := runtime.PHPVersions()
		if r.Args[0] == "mysql" {
			catalog = runtime.MySQLVersions()
		}
		found := false
		for _, v := range catalog {
			if v == r.Args[1] {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("runtime version is not in the catalog")
		}
	case "create":
		if r.Create != nil && r.Create.OfficialWizard {
			return fmt.Errorf("the official interactive wizard is available through the terminal only")
		}
		if r.Create == nil {
			return fmt.Errorf("creation options required")
		}
		return r.Create.Validate()
	default:
		return fmt.Errorf("unknown task kind")
	}
	return nil
}
func StartTask(ctx context.Context, home string, r TaskRequest) (Task, error) {
	if err := validateTask(home, r); err != nil {
		return Task{}, err
	}
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	gateCtx, gateCancel := context.WithTimeout(ctx, 5*time.Second)
	defer gateCancel()
	gate, err := filelock.Acquire(gateCtx, filepath.Join(home, "startup.lock"))
	if err != nil {
		return Task{}, fmt.Errorf("project preparation is busy: %w", err)
	}
	defer gate()
	dir := taskDir(home)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Task{}, err
	}
	slot := r.ProjectID
	if slot == "" {
		slot = "global"
	}
	release, err := filelock.Acquire(ctx, filepath.Join(dir, slot+".launch.lock"))
	if err != nil {
		return Task{}, err
	}
	defer release()
	activeFile := filepath.Join(dir, slot+".active")
	if previous, e := os.ReadFile(activeFile); e == nil {
		previousTask, e := ReadTask(home, strings.TrimSpace(string(previous)))
		if e == nil && taskPending(previousTask.Status) {
			return Task{}, fmt.Errorf("a task is already pending for this project")
		}
	}
	id := supervisor.NewID()[:12]
	projectID := r.ProjectID
	if r.Kind == "create" {
		destination, e := scaffold.Destination(r.Create.Path)
		if e != nil {
			return Task{}, e
		}
		projectID = paths.ProjectID(destination)
	}
	task := Task{Label: taskLabel(r), ID: id, ProjectID: projectID, Kind: r.Kind, Status: "queued", Started: time.Now().UTC().Format(time.RFC3339Nano), ExitCode: -1}
	if r.Create != nil {
		task.ProjectName = filepath.Base(r.Create.Path)
	} else if p, e := projects.New(home).Load(r.ProjectID); e == nil {
		task.ProjectName = p.Name
	}
	request := filepath.Join(dir, id+"-request.json")
	if err := statefile.WriteJSON(request, r); err != nil {
		return task, err
	}
	if err := statefile.WriteJSON(taskFile(home, id), task); err != nil {
		return task, err
	}
	if err := os.WriteFile(activeFile, []byte(id), 0600); err != nil {
		return task, err
	}
	log, err := os.OpenFile(filepath.Join(dir, id+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return task, err
	}
	defer log.Close()
	executable, err := os.Executable()
	if err != nil {
		return task, err
	}
	cmd := exec.Command(executable, "__project-task", id)
	cmd.Env = os.Environ()
	for i := len(cmd.Env) - 1; i >= 0; i-- {
		key, _, _ := strings.Cut(cmd.Env[i], "=")
		if strings.EqualFold(key, "LARASERVE_HOME") {
			cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
		}
	}
	cmd.Env = append(cmd.Env, "LARASERVE_HOME="+home)
	cmd.Stdout, cmd.Stderr = log, log
	DetachTask(cmd)
	if err = cmd.Start(); err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		task.ExitCode = 1
		_ = statefile.WriteJSON(taskFile(home, id), task)
		return task, err
	}
	go func() { _ = cmd.Wait() }()
	return task, nil
}
func ReadTask(home, id string) (Task, error) { return readTask(home, id, true) }
func readTask(home, id string, output bool) (Task, error) {
	var task Task
	if !projects.ValidID(id) {
		return task, fmt.Errorf("invalid task ID")
	}
	b, err := projectconfig.ReadRegular(taskFile(home, id), 16<<10)
	if err != nil {
		return task, err
	}
	if err = json.Unmarshal(b, &task); err != nil {
		return task, err
	}
	if task.ID != id {
		return task, fmt.Errorf("task identity mismatch")
	}
	if task.Status == "queued" || task.Status == "running" {
		state, e := supervisor.Status(taskDir(home), id)
		started, _ := time.Parse(time.RFC3339Nano, task.Started)
		updated, _ := time.Parse(time.RFC3339Nano, state.Updated)
		vanished := state.Status == "stopped" && ((state.Generation == "" && time.Since(started) > 15*time.Second) || (state.Generation != "" && time.Since(updated) > 2*time.Second))
		if e == nil && (state.Status == "crashed" || state.Status == "failed" || vanished) {
			marker := filepath.Join(taskDir(home), id+".interrupted")
			f, markErr := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if markErr == nil {
				f.Close()
			} else if !os.IsExist(markErr) {
				return task, markErr
			}
			info, markErr := os.Stat(marker)
			if markErr != nil {
				return task, markErr
			}
			task.Status = "recovering"
			task.Error = "Task owner exited; waiting for owned child cleanup"
			if time.Since(info.ModTime()) >= 25*time.Second {
				task.Status = "interrupted"
				task.Error = "Task owner exited; inspect output before retrying"
			}
			task.ExitCode = 1
		}
	}
	if !output {
		task.Progress = activity.Read(filepath.Join(taskDir(home), id+"-progress.json"))
		return task, nil
	}
	f, err := os.Open(filepath.Join(taskDir(home), id+".log"))
	if os.IsNotExist(err) {
		task.Progress = activity.Read(filepath.Join(taskDir(home), id+"-progress.json"))
		return task, nil
	}
	if err != nil {
		return task, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return task, err
	}
	task.Offset = stat.Size() - (256 << 10)
	if task.Offset < 0 {
		task.Offset = 0
	}
	_, err = f.Seek(task.Offset, io.SeekStart)
	if err != nil {
		return task, err
	}
	out, err := io.ReadAll(io.LimitReader(f, 256<<10))
	if err != nil {
		return task, err
	}
	// Plain text only; retain byte offsets for terminal streaming.
	task.Output = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' {
			return -1
		}
		return r
	}, strings.ToValidUTF8(string(out), "�"))
	task.Progress = activity.Read(filepath.Join(taskDir(home), id+"-progress.json"))
	return task, nil
}
func CancelTask(ctx context.Context, home, id string) error {
	task, err := ReadTask(home, id)
	if err != nil {
		return err
	}
	if task.Status != "queued" && task.Status != "running" {
		return nil
	}
	if err = os.WriteFile(filepath.Join(taskDir(home), id+".cancel"), []byte("cancel\n"), 0600); err != nil {
		return err
	}
	return supervisor.Stop(ctx, taskDir(home), id)
}
func RunTaskWorker(home, id string) (result error) {
	if !projects.ValidID(id) {
		return fmt.Errorf("invalid task ID")
	}
	b, err := projectconfig.ReadRegular(filepath.Join(taskDir(home), id+"-request.json"), 64<<10)
	if err != nil {
		return err
	}
	var request TaskRequest
	if err = json.Unmarshal(b, &request); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(taskDir(home), id+"-request.json"))
	task, err := ReadTask(home, id)
	if err != nil {
		return err
	}
	defer func() {
		task.Status = "succeeded"
		task.ExitCode = 0
		if result != nil {
			task.Status = "failed"
			task.ExitCode = 1
			task.Error = activity.Failure(filepath.Join(taskDir(home), id+".log"), result.Error())
			var exit *exec.ExitError
			if errors.As(result, &exit) {
				task.ExitCode = exit.ExitCode()
			}
		}
		if _, e := os.Stat(filepath.Join(taskDir(home), id+".cancel")); e == nil {
			task.Status = "cancelled"
			task.ExitCode = 1
			task.Error = "Task cancelled; completed changes were retained"
		}
		task.Finished = time.Now().UTC().Format(time.RFC3339Nano)
		task.Output = ""
		task.Offset = 0
		_ = statefile.WriteJSON(taskFile(home, id), task)
	}()
	activity.SetFile(filepath.Join(taskDir(home), id+"-progress.json"))
	activity.Plan("Prepare runtime", "Run command")
	activity.Start("Prepare runtime")
	defer func() { activity.Finish(result) }()
	if err = validateTask(home, request); err != nil {
		return err
	}
	if err = process.InitOwnership(); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	session, err := supervisor.Begin(ctx, taskDir(home), id, "")
	if err != nil {
		return err
	}
	defer func() { session.Close(result) }()
	ctx = session.Context()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, e := os.Stat(filepath.Join(taskDir(home), id+".cancel")); e == nil {
					cancel()
					return
				}
			}
		}
	}()
	if _, err = os.Stat(filepath.Join(taskDir(home), id+".cancel")); err == nil {
		return context.Canceled
	}
	lockID := request.ProjectID
	if lockID == "" {
		lockID = "global"
	}
	release, ok, err := filelock.Try(filepath.Join(taskDir(home), lockID+".work.lock"))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("another task is already working on this project")
	}
	defer release()
	task.Status = "running"
	if err = statefile.WriteJSON(taskFile(home, id), task); err != nil {
		return err
	}
	_ = session.Ready("")
	restore := runtime.UseDownloadContext(ctx)
	defer restore()
	switch request.Kind {
	case "create":
		return scaffold.Create(ctx, home, *request.Create)
	case "runtime":
		activity.Plan("Download runtime", "Verify runtime")
		activity.Start("Download runtime")
		release, err := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
		if err != nil {
			return err
		}
		defer release()
		if request.Args[0] == "php" {
			_, err = runtime.EnsurePHP(filepath.Join(home, "runtimes"), request.Args[1])
		} else {
			_, err = runtime.EnsureMySQLVersion(filepath.Join(home, "runtimes"), request.Args[1])
		}
		if err == nil {
			activity.Start("Verify runtime")
		}
		return err
	}
	p, err := projects.New(home).Load(request.ProjectID)
	if err != nil {
		return err
	}
	if request.Kind == "npm" {
		activity.Plan("Choose project Node", "Run npm command")
		activity.Start("Choose project Node")
		manager, _, e := versions.PackageManager(p.Root)
		if e != nil {
			return e
		}
		if manager != "npm" {
			return fmt.Errorf("this project uses %s; npm will not rewrite its lockfile", manager)
		}
		gate, e := filelock.Acquire(ctx, filepath.Join(home, "startup.lock"))
		if e != nil {
			return e
		}
		node, e := frontend.Select(filepath.Join(home, "runtimes"), p.Root)
		gate()
		if e != nil {
			return e
		}
		activity.Detail("Node " + node.Version + " · selected for " + p.Name)
		activity.Start("Run npm command")
		args := append([]string{node.NPM}, request.Args...)
		cmd := exec.CommandContext(ctx, node.Node, args...)
		cmd.Dir = p.Root
		cmd.Env = frontend.Environment(node, os.Environ())
		output := &activity.Output{Destination: os.Stdout}
		cmd.Stdout, cmd.Stderr = output, output
		return output.Error(process.RunOwned(cmd))
	}
	state, err := supervisor.Status(home, p.ID)
	if err != nil {
		return err
	}
	var execution Execution
	if state.Status == "running" {
		execution, err = loadLiveExecution(home, p.ID, state.Generation)
		if err != nil {
			return err
		}
		go func() {
			ticker := time.NewTicker(300 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					now, e := supervisor.Status(home, p.ID)
					if e != nil || now.Status != "running" || now.Generation != state.Generation {
						cancel()
						return
					}
				}
			}
		}()
	} else {
		if request.Kind == "artisan" {
			return fmt.Errorf("start the project before running Artisan so its managed environment is available")
		}
		selected, e := versions.ProjectPHP(p.Root, runtime.PHPVersions(), "", p.PHP)
		if e != nil {
			return e
		}
		rt, e := runtime.EnsurePHP(filepath.Join(home, "runtimes"), selected)
		if e != nil {
			return e
		}
		st, e := prepare.EnsureState(filepath.Join(home, "projects", paths.ProjectID(p.Source)))
		if e != nil {
			return e
		}
		doc, e := projectconfig.Read(p.Root)
		if e != nil {
			return e
		}
		if e = ConfigurePHP(rt, st.Dir, doc.Config); e != nil {
			return e
		}
		execution = Execution{Root: p.Root, StateDir: st.Dir, Runtime: *rt, Env: env.Build(rt, st.Dir, false, st.AppEnv(p.Root, "http://localhost"))}
	}
	activity.Start("Run command")
	rt := &execution.Runtime
	if request.Kind == "artisan" {
		args := append([]string{"artisan"}, request.Args...)
		args = append(args, "--no-interaction")
		cmd := process.PHPContext(ctx, rt.PHP, args...)
		cmd.Dir, cmd.Env = execution.Root, execution.Env
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return process.RunOwned(cmd)
	}
	rt.SetCacheRoot(filepath.Join(home, "runtimes"))
	if err = runtime.EnsureComposer(rt); err != nil {
		return err
	}
	args := append([]string{rt.Composer}, request.Args...)
	args = append(args, "--no-interaction", "--no-plugins", "--no-scripts")
	return prepare.ComposerTask(ctx, rt.PHP, execution.Root, execution.Env, args)
}
func loadLiveExecution(home, id, generation string) (Execution, error) {
	e, err := execution(home, id)
	if err != nil {
		return e, fmt.Errorf("project needs one restart to record its task environment")
	}
	if e.Generation != generation {
		return e, fmt.Errorf("project generation changed; retry after readiness")
	}
	return e, nil
}

// RequireNoTasks protects cleanup and preparation from queued or running workers.
func RequireNoTasks(home, projectID string) error {
	entries, err := os.ReadDir(taskDir(home))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasSuffix(entry.Name(), ".json") || !projects.ValidID(id) {
			continue
		}
		task, err := readTask(home, id, false)
		if err != nil {
			return err
		}
		if (projectID == "" || task.ProjectID == projectID) && taskPending(task.Status) {
			return fmt.Errorf("wait for task %s or cancel it before preparing or removing these files", id)
		}
	}
	return nil
}

func taskPending(status string) bool {
	return status == "queued" || status == "running" || status == "recovering"
}

func taskLabel(r TaskRequest) string {
	label := "Artisan " + strings.Join(r.Args, " ")
	switch r.Kind {
	case "create":
		label = "Create Laravel app"
	case "runtime":
		label = "Install " + strings.Join(r.Args, " ")
	case "npm":
		label = "npm " + strings.Join(r.Args, " ")
	case "composer":
		label = "Composer " + strings.Join(r.Args, " ")
	}
	runes := []rune(label)
	if len(runes) > 300 {
		label = string(runes[:300]) + "…"
	}
	return label
}
