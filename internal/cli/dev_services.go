package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"laraserve/internal/devservice"
	"laraserve/internal/projectconfig"
	"laraserve/internal/runtime"
	"laraserve/internal/serve"
	"laraserve/internal/statefile"
	"laraserve/internal/supervisor"
	"laraserve/internal/ui"
	"laraserve/internal/versions"
)

type developmentServices struct {
	mu           sync.Mutex
	status       devservice.Snapshot
	statusFile   string
	env          []string
	node         *runtime.NodeRuntime
	dependencies []chan struct{}
	services     []supervisor.Service
	reservations []net.Listener
	fields       []ui.Field
}

func (d *developmentServices) Close() {
	for _, l := range d.reservations {
		_ = l.Close()
	}
}
func (d *developmentServices) address() (string, error) {
	l, e := devservice.Reserve()
	if e != nil {
		return "", e
	}
	d.reservations = append(d.reservations, l)
	return l.Addr().String(), nil
}
func (d *developmentServices) add(command devservice.Command, addresses ...string) {
	d.status.Services = append(d.status.Services, devservice.Status{Name: command.Label, State: "starting"})
	var dependency chan struct{}
	if command.Label == "redis" || command.Label == "mailpit" {
		dependency = make(chan struct{})
		d.dependencies = append(d.dependencies, dependency)
	}
	d.services = append(d.services, supervisor.ServiceFunc{Label: command.Label, Execute: func(ctx context.Context, ready func()) error {
		if command.Label == "queue" || command.Label == "scheduler" {
			for _, wait := range d.dependencies {
				select {
				case <-wait:
				case <-ctx.Done():
					return nil
				}
			}
		}
		command.Env = d.env
		if d.node != nil {
			command.Env = nodeEnvironment(d.node, command.Env)
		}
		for _, address := range addresses {
			for _, l := range d.reservations {
				if l.Addr().String() == address {
					_ = l.Close()
				}
			}
		}
		err := command.Run(ctx, func() {
			d.record(command.Label, "running")
			if dependency != nil {
				close(dependency)
			}
			ready()
		})
		state := "stopped"
		if err != nil {
			state = "failed"
		}
		d.record(command.Label, state)
		return err
	}})
}

func (d *developmentServices) record(name, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.status.Services {
		if d.status.Services[i].Name == name {
			d.status.Services[i].State = state
		}
	}
	if d.statusFile != "" {
		_ = statefile.WriteJSON(d.statusFile, d.status)
	}
}
func prepareDevelopment(cache string, c projectconfig.Config, cfg *serve.Config, rt *runtime.Runtime) (*developmentServices, error) {
	d := &developmentServices{}
	success := false
	defer func() {
		if !success {
			d.Close()
		}
	}()
	command := func(name, executable string, args ...string) devservice.Command {
		return devservice.Command{Label: name, Executable: executable, Args: args, Root: cfg.Root, Log: filepath.Join(cfg.StateDir, "logs", name+".log")}
	}
	if c.Services.Redis {
		binary, err := runtime.EnsureRedis(cfg.Context, cache)
		if err != nil {
			return nil, err
		}
		address, err := d.address()
		if err != nil {
			return nil, err
		}
		_, port, _ := net.SplitHostPort(address)
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return nil, err
		}
		password := hex.EncodeToString(secret)
		dir := filepath.Join(cfg.StateDir, "redis")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		// JSON quoting is accepted by Redis's configuration parser and escapes paths.
		quoted, _ := json.Marshal(filepath.ToSlash(dir))
		config := fmt.Sprintf("bind 127.0.0.1\nport %s\nprotected-mode yes\nrequirepass %s\ndir %s\nappendonly yes\nsave \"\"\ndaemonize no\n", port, password, quoted)
		file := filepath.Join(dir, "redis.conf")
		if err = statefile.WriteBytes(file, []byte(config)); err != nil {
			return nil, err
		}
		cmd := command("redis", binary, file)
		cmd.Address = address
		d.add(cmd, address)
		cfg.AppEnv["REDIS_HOST"] = "127.0.0.1"
		cfg.AppEnv["REDIS_PORT"] = port
		cfg.AppEnv["REDIS_PASSWORD"] = password
		cfg.AppEnv["REDIS_URL"] = "redis://:" + password + "@" + address
		d.fields = append(d.fields, ui.Field{Label: "Redis", Value: address})
	}
	if c.Services.Mailpit {
		binary, err := runtime.EnsureMailpit(cache)
		if err != nil {
			return nil, err
		}
		smtp, err := d.address()
		if err != nil {
			return nil, err
		}
		web, err := d.address()
		if err != nil {
			return nil, err
		}
		_, port, _ := net.SplitHostPort(smtp)
		dir := filepath.Join(cfg.StateDir, "mailpit")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		cmd := command("mailpit", binary, "--smtp", smtp, "--listen", web, "--database", filepath.Join(dir, "messages.db"))
		cmd.Address = web
		d.add(cmd, smtp, web)
		cfg.AppEnv["MAIL_MAILER"] = "smtp"
		cfg.AppEnv["MAIL_DRIVER"] = "smtp"
		cfg.AppEnv["MAIL_HOST"] = "127.0.0.1"
		cfg.AppEnv["MAIL_PORT"] = port
		cfg.AppEnv["MAIL_USERNAME"] = ""
		cfg.AppEnv["MAIL_PASSWORD"] = ""
		cfg.AppEnv["MAIL_ENCRYPTION"] = "null"
		cfg.AppEnv["MAIL_SCHEME"] = "smtp"
		cfg.AppEnv["MAIL_URL"] = "smtp://" + smtp
		d.fields = append(d.fields, ui.Field{Label: "Mailpit", Value: "http://" + web})
	}
	if c.Services.Vite {
		if _, _, err := versions.PackageManager(cfg.Root); err != nil {
			return nil, err
		}
		entry := filepath.Join(cfg.Root, "node_modules", "vite", "dist", "node", "index.js")
		if info, err := os.Stat(entry); err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Vite is enabled but not installed; install frontend dependencies with the project's package manager first")
		}
		node, err := projectNode(cache, cfg.Root)
		if err != nil {
			return nil, err
		}
		address, err := d.address()
		if err != nil {
			return nil, err
		}
		_, port, _ := net.SplitHostPort(address)
		n, _ := strconv.Atoi(port)
		server := map[string]any{"host": "127.0.0.1", "port": n, "strictPort": true, "cors": map[string]any{"origin": cfg.URL()}, "hmr": map[string]any{"host": cfg.Domain, "port": n}}
		server["allowedHosts"] = []string{cfg.Domain}
		scheme := "http"
		if cfg.HTTPS {
			scheme = "https"
			server["https"] = map[string]string{"cert": cfg.Cert, "key": cfg.Key}
		}
		server["origin"] = scheme + "://" + net.JoinHostPort(cfg.Domain, port)
		options, _ := json.Marshal(map[string]any{"root": cfg.Root, "server": server})
		launcher := []byte("import {pathToFileURL} from 'node:url'; import {readFileSync} from 'node:fs';\nconst {createServer}=await import(pathToFileURL(process.argv[2]).href);\nconst options=" + string(options) + ";\nif(options.server.https){for(const k of ['cert','key']) options.server.https[k]=readFileSync(options.server.https[k]);}\nconst server=await createServer(options); await server.listen(); server.printUrls();\nfor(const signal of ['SIGINT','SIGTERM']) process.on(signal,async()=>{await server.close();process.exit(0);});\n")
		file := filepath.Join(cfg.StateDir, "vite.mjs")
		if err = statefile.WriteBytes(file, launcher); err != nil {
			return nil, err
		}
		cmd := command("vite", node.Node, file, entry)
		cmd.Address = address
		d.add(cmd, address)
		d.fields = append(d.fields, ui.Field{Label: "Node", Value: node.Version}, ui.Field{Label: "Vite", Value: scheme + "://" + net.JoinHostPort(cfg.Domain, port)})
		// Pass the managed Node directory to all selected processes after env.Build.
		d.node = node
	}
	if c.Services.Queue {
		cmd := command("queue", rt.PHP, "artisan", "queue:work", "--sleep=1", "--tries=1", "--timeout=60")
		cmd.PHP = true
		d.add(cmd)
		d.fields = append(d.fields, ui.Field{Label: "Queue", Value: "Worker enabled"})
	}
	if c.Services.Scheduler {
		cmd := command("scheduler", rt.PHP, "artisan", "schedule:work")
		cmd.PHP = true
		d.add(cmd)
		d.fields = append(d.fields, ui.Field{Label: "Scheduler", Value: "Worker enabled"})
	}
	success = true
	return d, nil
}
