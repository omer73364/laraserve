package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"laraserve/internal/process"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
	"syscall"
	"time"

	"laraserve/internal/certs"
	"laraserve/internal/detect"
	"laraserve/internal/env"
	"laraserve/internal/filelock"
	"laraserve/internal/hosts"
	"laraserve/internal/mysql"
	"laraserve/internal/pack"
	"laraserve/internal/paths"
	"laraserve/internal/phpmyadmin"
	"laraserve/internal/prepare"
	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/projecttools"
	"laraserve/internal/runtime"
	"laraserve/internal/serve"
	"laraserve/internal/statefile"
	"laraserve/internal/supervisor"
	"laraserve/internal/trust"
	"laraserve/internal/ui"
	"laraserve/internal/versions"
)

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)

func cmdServe(args []string) error {
	options := newServeOptions()
	fs := options.fs
	yes := options.yes
	if err := parse(fs, args); err != nil {
		return err
	}
	ui.Yes = *yes
	if *options.sandbox {
		return errors.New("--sandbox is not implemented yet (planned: Landlock on Linux, sandbox-exec on macOS)")
	}

	if fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve serve [path] [flags]")
	}

	// ---- 1. locate the project (directory, bundle, or copy) ----
	target := "."
	if fs.NArg() > 0 {
		target = fs.Arg(0)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	canonical, err := projects.Canonical(abs)
	if err != nil {
		return err
	}
	abs = canonical
	h, err := paths.New()
	if err != nil {
		return err
	}
	registryID := paths.ProjectID(canonical)
	lifetime, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	if err := process.InitOwnership(); err != nil {
		return fmt.Errorf("cannot establish child-process ownership: %w", err)
	}
	session, err := supervisor.Begin(lifetime, h.Root, registryID, os.Getenv("LARASERVE_START_ID"))
	if err != nil {
		return err
	}
	var runErr error
	defer func() { session.Close(runErr) }()
	store, err := projectStore(h)
	if err != nil {
		runErr = err
		return err
	}
	planned := &projects.Project{ID: registryID, Name: strings.TrimSuffix(filepath.Base(canonical), ".laraserve"), Source: canonical, Root: canonical, Args: rememberServeArgs(fs, canonical), HTTPS: !*options.httpF, Domain: *options.domainF, Port: *options.portF}
	if err = store.Import(planned); err != nil {
		runErr = err
		return err
	}

	return runRegisteredServe(options, h, session, canonical, &runErr)
}

func runRegisteredServe(options *serveOptions, h *paths.Home, session *supervisor.Session, abs string, runErr *error) (result error) {
	defer func() {
		if session.Context().Err() != nil {
			result = nil
		}
		*runErr = result
	}()
	fs := options.fs
	domainF := options.domainF
	portF := options.portF
	host := options.host
	httpF := options.httpF
	allow := options.allow
	noTrust := options.noTrust
	noHosts := options.noHosts
	copyF := options.copyF
	migrate := options.migrate
	mysqlF := options.mysqlF
	allowInsecure := options.allowInsecure
	phpVersionF := options.phpVersionF
	mysqlVersionF := options.mysqlVersionF
	adminF := options.adminF
	hostPath := options.hostPath
	verbose := options.verbose

	https := !*httpF
	registryID := paths.ProjectID(abs)
	restoreDownloads := runtime.UseDownloadContext(session.Context())
	defer restoreDownloads()
	// Serialize footprint/CA preparation, but release before serving. Running
	// projects remain independent and cache installation also has its own locks.
	preparation, err := filelock.Acquire(session.Context(), filepath.Join(h.Root, "startup.lock"))
	if err != nil {
		return err
	}
	defer func() {
		if preparation != nil {
			preparation()
		}
	}()
	if err := projecttools.RequireNoTasks(h.Root, registryID); err != nil {
		return err
	}
	c, err := load()
	if err != nil {
		return err
	}
	registry := projects.New(h.Root)
	previous, err := registry.Load(registryID)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if previous != nil && previous.Source != abs {
		return fmt.Errorf("project ID collision; preserve the existing registry record")
	}
	explicit := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
	if previous != nil {
		if !explicit["domain"] {
			*domainF = previous.Domain
		}
		if !explicit["port"] {
			*portF = previous.Port
		}
		if !explicit["http"] {
			*httpF = !previous.HTTPS
			https = previous.HTTPS
		}
	}
	root, isWorkspace, err := resolveRoot(c, abs, *copyF)
	if err != nil {
		return err
	}

	configDoc, err := projectconfig.Read(root)
	if err != nil {
		return err
	}
	actual := explicit
	if os.Getenv("LARASERVE_SETTINGS_REPLAY") == "1" {
		actual = map[string]bool{}
		for _, name := range strings.Split(os.Getenv("LARASERVE_EXPLICIT_FLAGS"), ",") {
			if name != "" {
				actual[name] = true
			}
		}
	}
	defaults := configDoc.Config
	if !actual["domain"] && defaults.Domain != "" {
		*domainF = defaults.Domain
	}
	if !actual["http"] && defaults.HTTPS != nil {
		*httpF = !*defaults.HTTPS
	}
	if !actual["port"] && defaults.Port != nil {
		*portF = *defaults.Port
	}
	if !actual["php-version"] && defaults.PHP != "" {
		*phpVersionF = defaults.PHP
	}
	if !actual["mysql-version"] && defaults.MySQL != "" {
		*mysqlVersionF = defaults.MySQL
	}
	if !actual["phpmyadmin"] && defaults.PHPMyAdmin != nil {
		*adminF = *defaults.PHPMyAdmin
	}
	https = !*httpF
	// ---- 2. detect ----
	proj, err := detect.Inspect(root)
	if err != nil {
		return err
	}
	record := &projects.Project{ID: registryID, Name: strings.TrimSuffix(filepath.Base(abs), ".laraserve"), Source: abs, Root: root, HTTPS: https}
	if previous != nil {
		record.Domain = previous.Domain
		record.Port = previous.Port
	}
	record.Args = rememberServeArgs(fs, abs)
	if err := registry.Save(record); err != nil {
		return err
	}
	ui.Section("Preparing application")
	ui.OK("Detected Laravel %s (PHP %s)", orDash(proj.Laravel), orDash(proj.PHPConstraint))
	connection := detect.DatabaseConnection(root)
	useMySQL := managedMySQL(fs, *mysqlF, connection)
	if !actual["mysql"] {
		switch defaults.Database {
		case "managed":
			useMySQL = true
		case "external", "sqlite":
			useMySQL = false
		case "auto":
			useMySQL = connection == "mysql" || connection == "mariadb"
		}
	}

	id, err := projectStateID(c.man, root, isWorkspace)
	if err != nil {
		return err
	}
	stateDir := filepath.Join(c.home.Projects(), id)
	versionPath := filepath.Join(stateDir, "runtime-versions.json")
	locked, err := versions.ReadLock(versionPath)
	if err != nil {
		return err
	}
	selectedPHP, err := versions.ProjectPHP(root, runtime.PHPVersions(), *phpVersionF, locked.PHP)
	if err != nil {
		return err
	}
	selectedMySQL := ""
	if useMySQL {
		selectedMySQL, err = versions.ProjectMySQL(root, runtime.MySQLVersions(), *mysqlVersionF, locked.MySQL)
		if err != nil {
			return err
		}
	}
	// ---- 3. select and download the project's PHP runtime ----
	rt, err := runtime.EnsurePHP(c.home.Runtimes(), selectedPHP)
	if err != nil {
		return err
	}
	if err = projecttools.ConfigurePHP(rt, stateDir, defaults); err != nil {
		return err
	}
	ui.OK("PHP %s ready", selectedPHP)
	// ---- 4. per-project state ----
	mp := c.man.Project(id, root)
	mp.Workspace = isWorkspace
	mp.Path = root
	st, err := prepare.EnsureState(stateDir)
	if err != nil {
		return err
	}

	// ---- 5. dependencies ----
	if !proj.HasVendor {
		if err := runtime.EnsureComposer(rt); err != nil {
			return err
		}
		if *allowInsecure {
			ui.Warn("Allowing Composer dependencies with known security advisories for this installation")
		}
		ui.Info("vendor/ is missing: running composer install (needs network or a Composer cache)")
		baseEnv := env.Build(rt, st.Dir, *hostPath, nil)
		if err := prepare.ComposerInstallContext(session.Context(), rt.PHP, rt.Composer, root, baseEnv, *allowInsecure); err != nil {
			return fmt.Errorf("composer install failed: %w\n  Tip: run `laraserve pack` on an online machine and serve the bundle", err)
		}
		mp.Vendor = true
	}
	ui.OK("Dependencies ready")

	// ---- 6. domain ----
	domain := strings.ToLower(strings.TrimSpace(*domainF))
	if domain == "" {
		domain = "localhost"
	}
	if !domainRe.MatchString(domain) {
		return fmt.Errorf("invalid domain %q", domain)
	}
	permitted := splitCSV(*allow)
	if https && !certs.Covered(domain, append(append([]string{}, certs.DefaultPermitted...), permitted...)) {
		return fmt.Errorf("%s is outside the local CA's name constraints (%s).\n"+
			"  Re-run with --allow-domain %s to extend them, or use a .test / .localhost name",
			domain, strings.Join(certs.DefaultPermitted, ", "), domain)
	}
	if !hosts.IsLoopbackName(domain) && !hosts.Resolves(domain) {
		domain = c.ensureHosts(domain, *noHosts)
	}
	c.man.AddDomain(mp, domain)

	// ---- 7. port ----
	serverRuntime := *rt
	if rt.ServerPHP != "" {
		serverRuntime.PHP = rt.ServerPHP
	}
	port, err := choosePort(&serverRuntime, *host, *portF, https, *domainF != "")
	if err != nil {
		return err
	}

	// ---- 8. TLS ----
	lan := *host == "0.0.0.0"
	var certPath, keyPath string
	if https {
		ca, err := c.ensureTrust(permitted, *noTrust, false)
		if err != nil {
			return err
		}
		var lanIPs []net.IP
		if lan {
			lanIPs = certs.LANIPs()
		}
		certPath, keyPath, err = certs.IssueLeaf(ca, c.home.CA(), domain, lanIPs)
		if err != nil {
			return err
		}
		ui.OK("Certificate ready for %s", domain)
	}

	cfg := serve.Config{
		PHP: serverRuntime.PHP, NativePHP: rt.NativeCLI, Root: root, StateDir: st.Dir, Domain: domain, Host: *host,
		Port: port, HTTPS: https, Cert: certPath, Key: keyPath, LAN: lan, Verbose: *verbose, Context: session.Context(),
	}

	// ---- 9. app environment (nothing written into the project) ----
	cfg.AppEnv = st.AppEnv(root, cfg.URL())
	if !actual["mysql"] && defaults.Database == "external" {
		delete(cfg.AppEnv, "DB_CONNECTION")
		delete(cfg.AppEnv, "DB_DATABASE")
	}
	if !actual["mysql"] && defaults.Database == "sqlite" {
		cfg.AppEnv["DB_CONNECTION"] = "sqlite"
		cfg.AppEnv["DB_DATABASE"] = st.SQLite
	}
	var db *mysql.Server
	var dbLifetime context.Context
	if useMySQL {
		ui.Section("Preparing database")
		ui.Info("Using a private MySQL database (use --mysql=false to keep the project's external connection)")
		base, err := runtime.EnsureMySQLVersion(c.home.Runtimes(), selectedMySQL)
		if err != nil {
			return err
		}
		baseEnv := env.Build(rt, st.Dir, *hostPath, nil)
		check := process.PHP(rt.PHP, "-r", "exit(extension_loaded('pdo_mysql') ? 0 : 1);")
		check.Env = baseEnv
		if err := check.Run(); err != nil {
			return fmt.Errorf("private MySQL requires pdo_mysql in the PHP runtime: %w", err)
		}
		ctx, stop := signal.NotifyContext(session.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		dbLifetime = ctx
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		db, err = mysql.Start(ctx, base, mysqlDataDir(filepath.Join(c.home.Root, "databases", id), selectedMySQL), baseEnv, filepath.Base(abs))
		if err != nil {
			return err
		}
		defer db.Close()
		cfg.Context = ctx
		go func() {
			select {
			case <-db.Done():
				cancel()
			case <-ctx.Done():
			}
		}()
		for k, v := range db.Env {
			cfg.AppEnv[k] = v
		}
		ui.OK("MySQL %s ready (database: %s)", selectedMySQL, db.Env["DB_DATABASE"])
	}
	development, err := prepareDevelopment(c.home.Runtimes(), configDoc.Config, &cfg, rt)
	if err != nil {
		return err
	}
	defer development.Close()
	cfg.Env = env.Build(rt, st.Dir, *hostPath, cfg.AppEnv)
	development.env = cfg.Env
	ownerState, snapshotErr := supervisor.Status(c.home.Root, registryID)
	if snapshotErr != nil {
		return snapshotErr
	}
	development.status.Generation = ownerState.Generation
	development.statusFile = filepath.Join(st.Dir, "services.json")
	if err := statefile.WriteJSON(development.statusFile, development.status); err != nil {
		return err
	}
	if snapshotErr = projecttools.SaveExecution(c.home.Root, registryID, projecttools.Execution{Generation: ownerState.Generation, Root: root, StateDir: st.Dir, Runtime: *rt, Env: cfg.Env}); snapshotErr != nil {
		return snapshotErr
	}
	if db != nil && *migrate {
		mysqlState := &prepare.State{Dir: db.Dir}
		ui.Info("Applying MySQL migrations")
		if err := mysqlState.EnsureMigrationsContext(cfg.Context, rt.PHP, root, cfg.Env, *migrate); err != nil {
			return err
		}
	} else if cfg.AppEnv["DB_CONNECTION"] == "sqlite" && *migrate {
		ui.Info("Applying SQLite migrations")
		if err := st.EnsureMigrations(rt.PHP, root, cfg.Env, *migrate); err != nil {
			return err
		}
		ui.OK("SQLite ready")
	} else if db == nil {
		if cfg.AppEnv["DB_CONNECTION"] == "sqlite" {
			ui.OK("SQLite ready (migrations skipped)")
		} else {
			ui.OK("Using project's database connection (%s)", connection)
		}
	}
	var admin *phpmyadmin.Workspace
	if db != nil && *adminF {
		check := process.PHP(rt.PHP, "-r", "exit(extension_loaded('mysqli') && extension_loaded('mbstring') ? 0 : 1);")
		check.Env = cfg.Env
		if err := check.Run(); err != nil {
			return fmt.Errorf("phpMyAdmin needs mysqli and mbstring in the PHP runtime; use --phpmyadmin=false to serve without it: %w", err)
		}
		source, err := runtime.EnsurePHPMyAdmin(c.home.Runtimes())
		if err != nil {
			return err
		}
		start := 8081
		if https {
			start = 8444
		}
		adminPort, err := serve.FreePort("127.0.0.1", start, false)
		if err != nil {
			return err
		}
		if adminPort == cfg.Port {
			adminPort, err = serve.FreePort("127.0.0.1", adminPort+1, false)
			if err != nil {
				return err
			}
		}
		adminURL := (serve.Config{Domain: "localhost", Port: adminPort, HTTPS: https}).URL()
		admin, err = phpmyadmin.Prepare(source, st.Dir, adminURL, db.Env)
		if err != nil {
			return err
		}
		defer admin.Close()
		cfg.Admin = &serve.AdminConfig{Root: admin.Root, Port: adminPort}
	}
	if selectedMySQL == "" {
		selectedMySQL = locked.MySQL
	}
	if err := versions.WriteLock(versionPath, versions.Pins{PHP: selectedPHP, MySQL: selectedMySQL}); err != nil {
		return err
	}
	if err := c.man.Save(); err != nil {
		return err
	}

	record.Domain = domain
	record.Port = port
	record.HTTPS = https
	record.PHP = selectedPHP
	if db != nil {
		record.MySQL = selectedMySQL
		record.Database = "Managed MySQL " + selectedMySQL
	} else {
		record.MySQL = ""
		record.Database = "External " + connection
		if cfg.AppEnv["DB_CONNECTION"] == "sqlite" {
			record.Database = "SQLite"
		}
	}
	if err := registry.Save(record); err != nil {
		return err
	}
	if err := session.Context().Err(); err != nil {
		return err
	}
	// ---- 10. serve ----
	services := append(development.services, supervisor.ServiceFunc{Label: "application", Execute: func(ctx context.Context, ready func()) error {
		application := cfg
		lifetime, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(cfg.Context, cancel)
		defer stop()
		application.Context = lifetime
		return serve.Run(application, ready)
	}})
	err = supervisor.RunServices(cfg.Context, services, func() {
		if err := session.Ready(cfg.URL()); err != nil {
			ui.Warn("Could not record project readiness: %v", err)
		}
		preparation()
		preparation = nil
		database := "External (" + connection + ")"
		data := ""
		if db != nil {
			database = "MySQL " + selectedMySQL + " · private socket"
			if goruntime.GOOS == "windows" {
				database = "MySQL " + selectedMySQL + " · private loopback"
			}
			data = db.Dir
		} else if connection == "sqlite" || connection == "" {
			database = "SQLite"
			data = st.SQLite
		}
		fields := []ui.Field{
			{Label: "Local", Value: cfg.URL()},
			{Label: "Project", Value: root},
			{Label: "Framework", Value: "Laravel " + orDash(proj.Laravel)},
			{Label: "PHP", Value: selectedPHP},
			{Label: "Database", Value: database},
		}
		fields = append(fields, development.fields...)
		if data != "" {
			fields = append(fields, ui.Field{Label: "Database data", Value: data})
		}
		if admin != nil {
			fields = append(fields, ui.Field{Label: "phpMyAdmin", Value: admin.LoginURL})
		}
		if lan {
			for _, ip := range certs.LANIPs() {
				fields = append(fields, ui.Field{Label: "LAN", Value: fmt.Sprintf("%s://%s:%d", map[bool]string{true: "https", false: "http"}[https], ip, port)})
			}
		}
		fields = append(fields, ui.Field{Label: "Logs", Value: filepath.Join(st.Dir, "logs", "server.log")})
		ui.Ready(fields...)
		if lan && https {
			ui.Info("LAN devices must trust %s", filepath.Join(c.home.CA(), "ca.pem"))
		}
	})
	if db != nil && dbLifetime.Err() == nil {
		select {
		case <-db.Done():
			return fmt.Errorf("private MySQL stopped: %v; see %s", db.Err(), filepath.Join(db.Dir, "server.log"))
		default:
		}
	}
	if err == nil {
		ui.OK("Application stopped")
	}
	return err
}

func managedMySQL(fs *flag.FlagSet, value bool, connection string) bool {
	explicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "mysql" {
			explicit = true
		}
	})
	if explicit {
		return value
	}
	return connection == "mysql" || connection == "mariadb"
}

func resolveRoot(c *ctx, abs string, copyMode bool) (root string, workspace bool, err error) {
	st, err := os.Stat(abs)
	if err != nil {
		return "", false, err
	}
	switch {
	case !st.IsDir() && strings.HasSuffix(abs, ".laraserve"):
		ws := filepath.Join(c.home.Workspaces(), paths.ProjectID(abs))
		if _, err := os.Stat(filepath.Join(ws, "artisan")); err != nil {
			ui.Info("Unpacking bundle")
			if err := pack.Extract(abs, ws); err != nil {
				return "", false, err
			}
		}
		return ws, true, nil
	case !st.IsDir():
		return "", false, fmt.Errorf("%s is not a directory or .laraserve bundle", abs)
	case copyMode:
		ws := filepath.Join(c.home.Workspaces(), paths.ProjectID(abs))
		ui.Info("Copying project to %s", ws)
		if err := os.MkdirAll(ws, 0o755); err != nil {
			return "", false, err
		}
		if err := pack.Copy(abs, ws); err != nil {
			return "", false, err
		}
		return ws, true, nil
	}
	return abs, false, nil
}

// ensureHosts makes domain resolvable, or falls back to NAME.localhost.
func (c *ctx) ensureHosts(domain string, noHosts bool) string {
	fallback := strings.SplitN(domain, ".", 2)[0] + ".localhost"
	if noHosts {
		ui.Warn("%s does not resolve; using %s (or add `%s` to %s)", domain, fallback, hosts.Line(domain), hosts.Path())
		return fallback
	}
	if !ui.Confirm(fmt.Sprintf("Add %s to the hosts file (%s)?", domain, hosts.Path())) {
		ui.Warn("Using %s instead (or add `%s` to %s yourself)", fallback, hosts.Line(domain), hosts.Path())
		return fallback
	}
	if err := hosts.Add(domain); err != nil {
		ui.Warn("could not edit hosts file: %v", err)
		ui.Info("Add this line yourself: %s", hosts.Line(domain))
		return fallback
	}
	c.man.AddHost(domain)
	_ = c.man.Save()
	ui.OK("Added %s to the hosts file", domain)
	return domain
}

// ensureTrust creates the CA and installs it in the OS store (once).
func (c *ctx) ensureTrust(allow []string, noTrust, reinstall bool) (*certs.CA, error) {
	ca, regen, err := certs.EnsureCA(c.home.CA(), allow)
	if err != nil {
		return nil, err
	}
	if regen {
		ui.Info("Local CA regenerated to include: %s", strings.Join(ca.Permitted, ", "))
	}
	if noTrust {
		ui.Warn("CA trust setup skipped (--no-trust): browsers may show a certificate warning")
		return ca, c.man.Save()
	}
	if c.man.CA != nil && c.man.CA.SHA256 != ca.SHA256 {
		if err := trust.Remove(c.man.CA); err != nil {
			return nil, fmt.Errorf("cannot replace the previous CA trust entry: %w; retry with access to the trust store, or use --http", err)
		}
		c.man.CA = nil
	}
	wasApproved := c.man.CA != nil
	if c.man.CA == nil || reinstall {
		switch {
		case ui.Confirm("Install laraserve's local CA in the system trust store? (name-constrained to " +
			strings.Join(ca.Permitted, ", ") + "; removable with `laraserve untrust`)"):
			entry, err := trust.Install(ca)
			if err != nil {
				ui.Warn("CA install failed: %v (browsers will show a warning)", err)
			} else {
				// Retain previously modified profiles so untrust can reverse them
				// even when they were unavailable during this installation.
				if c.man.CA != nil {
					seen := map[string]bool{}
					for _, db := range entry.NSSDBs {
						seen[db] = true
					}
					for _, db := range c.man.CA.NSSDBs {
						if !seen[db] {
							entry.NSSDBs = append(entry.NSSDBs, db)
						}
					}
				}
				c.man.CA = entry
				ui.OK("Local CA installed")
				ui.Info("Fully quit and reopen Firefox/Chrome to reload certificate trust")
			}
		default:
			ui.Warn("CA not installed: browsers will show a certificate warning")
		}
	}
	if wasApproved && !reinstall && c.man.CA != nil {
		trust.Repair(ca, c.man.CA)
	}
	return ca, c.man.Save()
}

func choosePort(rt *runtime.Runtime, host string, explicit int, https, hasDomain bool) (int, error) {
	if explicit != 0 {
		return serve.FreePort(host, explicit, true)
	}
	if https && hasDomain && privilegedPortOK(rt, host) {
		return 443, nil
	}
	def := 8080
	if https {
		def = 8443
	}
	return serve.FreePort(host, def, false)
}

// privilegedPortOK decides whether port 443 can be used without running as root.
func privilegedPortOK(rt *runtime.Runtime, host string) bool {
	probeHost := host
	if host == "0.0.0.0" {
		probeHost = "127.0.0.1"
	} else if host == "::" {
		probeHost = "::1"
	}
	if conn, err := net.DialTimeout("tcp", net.JoinHostPort(probeHost, "443"), 200*time.Millisecond); err == nil {
		conn.Close()
		ui.Info("Port 443 is in use; choosing an alternate HTTPS port")
		return false
	}
	if goruntime.GOOS != "linux" {
		return serve.CanBind(host, 443)
	}
	if !linuxLowPortAllowed(rt.PHP) {
		if !ui.Confirm("Use port 443? The cached server needs permission to bind it (sudo setcap, once)") {
			return false
		}
		cmd := exec.Command("sudo", "setcap", "cap_net_bind_service=+ep", rt.PHP)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			ui.Warn("setcap failed: %v; using 8443", err)
			return false
		}
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, "443"))
	if err == nil {
		l.Close()
		return true
	}
	// The probe runs as the current user, so EACCES is expected after setcap; only
	// permission errors are the only failures the server capability can resolve.
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

func linuxLowPortAllowed(bin string) bool {
	if os.Geteuid() == 0 {
		return true
	}
	if b, err := os.ReadFile("/proc/sys/net/ipv4/ip_unprivileged_port_start"); err == nil {
		var n int
		if _, err := fmt.Sscan(strings.TrimSpace(string(b)), &n); err == nil && n <= 443 {
			return true
		}
	}
	if out, err := exec.Command("getcap", bin).Output(); err == nil && strings.Contains(string(out), "cap_net_bind_service") {
		return true
	}
	return false
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// Preserve databases made by earlier releases; new versions get distinct data.
func mysqlDataDir(base, version string) string {
	if version == "8.4.13" {
		if st, err := os.Stat(filepath.Join(base, "data")); err == nil && st.IsDir() {
			return base
		}
	}
	return filepath.Join(base, "mysql-"+version)
}
