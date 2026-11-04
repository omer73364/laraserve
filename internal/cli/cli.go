// Package cli implements the laraserve commands.
package cli

import (
	"context"
	"flag"
	"fmt"
	"laraserve/internal/filelock"
	"os"
	"path/filepath"
	"strings"

	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/projecttools"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
)

const usage = `laraserve %s - serve any Laravel app over HTTPS, with no PHP/Composer installed

Usage:
  laraserve new <path> [--wizard | --starter KIT] [--database TYPE] [--migrate=false]
  laraserve serve [path|bundle.laraserve] [flags]
  laraserve settings [project] [--file config.json]
  laraserve env [project] [--file changes.json]
  laraserve artisan|composer [project] [--confirm-schema] -- command [args]
  laraserve task ID [--wait | --cancel]
  laraserve node|npm [path] -- command [args]   use project-selected Node
  laraserve runtimes [list | install php|mysql VERSION | remove KEY --confirm]
  laraserve dashboard [--port N]  open the protected local management UI
  laraserve projects           list registered projects and their status
  laraserve start [path|name|ID] [serve flags]   start in the background
  laraserve stop [path|name|ID]                 stop its current owner
  laraserve restart [path|name|ID] [serve flags]
  laraserve status [path|name|ID]
  laraserve pack  [path] [-o app.laraserve] [--include-env]
  laraserve trust | untrust      install / remove the local CA in the OS trust store
  laraserve clean [path]         remove vendor/ (if laraserve made it), state, hosts entries
  laraserve uninstall            remove everything laraserve ever created
  laraserve versions [path]      show selected and available runtime versions
  laraserve check [path] [--json] inspect project setup without changing files
  laraserve doctor [--report FILE] | version

serve flags:
  --domain NAME      serve on NAME (e.g. testapp.test); default localhost
  --http             plain HTTP (no CA, no trust prompt) on :8080
  --port N           listen port (default 8443 https / 8080 http; 443 is tried with --domain)
  --host ADDR        bind address (default 127.0.0.1; 0.0.0.0 exposes to the LAN)
  --allow-domain S   let the local CA also sign names under suffix S (comma separated)
  --no-trust         do not install the CA in the OS trust store
  --no-hosts         do not edit the hosts file (falls back to NAME.localhost)
  --copy             run on a copy of the project; the original is never touched
  --migrate          run pending migrations for SQLite or private MySQL
  --php-version V    select a project-compatible PHP version (e.g. 8.3)
  --mysql-version V  select private MySQL version (e.g. 8.0 or 8.4)
  --mysql            force private MySQL (auto-detected from project settings)
  --mysql=false      use the project's external MySQL connection
  --phpmyadmin=false disable the local phpMyAdmin page for private MySQL
  --host-path        keep the host PATH (default: only laraserve's own binaries)
  --allow-insecure-dependencies
                     allow advisory-affected Composer packages for this installation
  --verbose          stream server logs
  -y, --yes          answer yes to prompts
`

func Run(args []string) int {
	if len(args) == 0 {
		printHelp()
		return 0
	}
	if action := map[string]string{"serve": "Local development, ready to run.", "new": "Create your next Laravel app.", "versions": "Project requirements and available runtimes.", "doctor": "Your local environment at a glance."}[args[0]]; action != "" {
		ui.Header(runtime.Version, action)
	}
	var err error
	switch args[0] {
	case "trust", "untrust", "clean", "uninstall":
		home, e := paths.New()
		if e != nil {
			ui.Err("%v", e)
			return 1
		}
		if args[0] == "uninstall" {
			auto, e := filelock.Acquire(context.Background(), filepath.Join(home.Root, "auto-launch.lock"))
			if e != nil {
				ui.Err("%v", e)
				return 1
			}
			defer auto()
		}
		release, e := filelock.Acquire(context.Background(), filepath.Join(home.Root, "startup.lock"))
		if e != nil {
			ui.Err("%v", e)
			return 1
		}
		defer release()
	}
	switch args[0] {
	case "__autostart":
		err = cmdAutostart(args[1:])
	case "__project-task":
		if len(args) != 2 {
			err = fmt.Errorf("invalid task invocation")
		} else {
			home, e := paths.New()
			if e != nil {
				err = e
			} else {
				err = projecttools.RunTaskWorker(home.Root, args[1])
			}
		}
	case "node", "npm":
		err = cmdNode(args[1:], args[0] == "npm")
	case "task":
		err = cmdTask(args[1:])
	case "settings":
		err = cmdSettings(args[1:])
	case "env":
		err = cmdEnvironment(args[1:])
	case "artisan", "composer":
		err = cmdProjectTask(args[1:], args[0])
	case "runtimes":
		err = cmdRuntimes(args[1:])
	case "dashboard":
		err = cmdDashboard(args[1:])
	case "new":
		err = cmdNew(args[1:])
	case "versions":
		err = cmdVersions(args[1:])
	case "serve", "__background-serve":
		err = cmdServe(args[1:])
	case "projects", "list":
		err = cmdProjects(args[1:])
	case "start":
		err = cmdStart(args[1:], false)
	case "restart":
		err = cmdStart(args[1:], true)
	case "stop":
		err = cmdStop(args[1:])
	case "status":
		err = cmdStatus(args[1:])
	case "pack":
		err = cmdPack(args[1:])
	case "trust":
		err = cmdTrust(args[1:])
	case "untrust":
		err = cmdUntrust(args[1:])
	case "clean":
		err = cmdClean(args[1:])
	case "uninstall":
		err = cmdUninstall(args[1:])
	case "doctor":
		err = cmdDoctor(args[1:]...)
	case "check":
		err = cmdCheck(args[1:])
	case "version", "--version", "-v":
		fmt.Println("laraserve", runtime.Version)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", args[0])
		fmt.Fprintf(os.Stderr, usage, runtime.Version)
		return 2
	}
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		ui.Err("%v", err)
		return 1
	}
	return 0
}

func printHelp() {
	ui.Header(runtime.Version, "Serve Laravel locally with project-aware PHP and HTTPS.")
	text := fmt.Sprintf(usage, runtime.Version)
	if _, body, ok := strings.Cut(text, "\n\n"); ok {
		text = body
	}
	fmt.Print(text)
}

// parse allows flags after positional arguments (laraserve serve ./app --http).
func parse(fs *flag.FlagSet, args []string) error {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			flags = append(flags, a)
			name := strings.TrimLeft(a, "-")
			if strings.Contains(name, "=") {
				continue
			}
			if f := fs.Lookup(name); f != nil {
				bf, ok := f.Value.(interface{ IsBoolFlag() bool })
				if (!ok || !bf.IsBoolFlag()) && i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
			continue
		}
		pos = append(pos, a)
	}
	return fs.Parse(append(flags, pos...))
}

func yesFlags(fs *flag.FlagSet) *bool {
	y := fs.Bool("yes", false, "")
	fs.BoolVar(y, "y", false, "")
	return y
}

type ctx struct {
	autoSetup bool
	home      *paths.Home
	man       *manifest.Manifest
}

func load() (*ctx, error) {
	h, err := paths.New()
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(h.Manifest())
	if err != nil {
		return nil, err
	}
	return &ctx{home: h, man: m}, nil
}
