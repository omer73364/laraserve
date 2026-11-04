# Local dashboard

Run `laraserve dashboard` and open the complete private link printed in the terminal. The browser receives a session, then removes the login fragment from its address bar. Keep the original link private. `--port 9779` selects a fixed port; the default chooses an available port on `127.0.0.1`.

The dashboard is included in the client. It needs no Node installation, frontend build, network assets, account, hosted service, or separate PHP runtime. Closing it leaves independently owned projects running. Run the command again to obtain a fresh dashboard link and session.

## Projects and controls

Existing registered projects appear with their state, local URL, PHP version, Laravel metadata, database selection, source folder, and owner health. Direct legacy project registrations are imported using the same logic as the terminal. Old workspace records without a known source cannot safely be imported.

- **Start** uses saved reusable options and launches the existing project in the background.
- **Stop** contacts that project's authenticated owner and waits for its resources to stop.
- **Restart** stops the current owner and launches a new generation with its saved settings.
- **Open app** opens a running app in a separate browser tab.
- **Logs** opens the selected project’s log viewer.

Actions run asynchronously; project rows and the project header show progress and failures. A second dashboard action for that project is rejected while its first action is pending. The shared supervisor also prevents duplicate owners across the terminal and other dashboard sessions.

Migrations, blanket yes-to-all permission, and one-shot insecure dependency consent are never replayed. Phase 4 adds validated settings, masked environment editing, deliberate Artisan/Composer tasks, and new-app creation. Known schema-changing actions require explicit confirmation for existing apps; initial migrations are allowed when creating a new app. No shell commands or arbitrary-file editing endpoints are exposed. Local hosts/trust setup uses OS authorization dialogs when available; dependency and preparation failures are shown directly in the progress view. See [project tools](project-tools.md).

The page refreshes project status every three seconds while visible. Operation progress belongs to this dashboard process; actual project ownership remains independently queryable after the dashboard exits. A preparation timeout ends the dashboard's wait, not a detached project's lifetime. Check status and startup logs before starting it again.

## Logs

Open a project and select its **Logs** tab. Choose Laravel, Web server + PHP, MySQL, a development service, or Startup, then enter a case-insensitive literal search. Refresh logs explicitly when needed. PHP and web server output share one stream. Search covers the recent tail, not the full historical file.

Reads are capped at 256 KiB per request. Laravel includes up to eight recently modified `.log` files directly under `storage/logs`, sharing that budget; the viewer indicates truncation. Managed MySQL reads the project's selected version's log and preserves the old 8.4.13 directory layout where present. External database logs are not collected. Empty or missing streams show no output.

Log paths are selected on the backend, confined to the project or private LaraServe tree, and checked as regular files. Symlinks escaping those trees are rejected. Text is displayed without interpreting HTML or terminal escape sequences. Logs may still contain application secrets: the dashboard is a local operator tool, not a sanitized support report. Do not share its private link or forward its port.

## Health and measured resources

Health distinguishes preparation, readiness with a reachable owner, stopping, failure/crash, and an unreachable owner. “Server ready” reflects the generation's successful startup probe and live supervisor; it is not a continuous application HTTP or SQL query health check.

On Linux, counters include the owner and its current descendants, including guards, PHP/web servers, and managed MySQL. Memory is the sum of resident set sizes, so shared pages may be counted more than once. CPU is sampled from process and system counters between refreshes: 100% represents about one logical CPU and multicore use may exceed 100%. The first sample, a new owner generation, or process counter changes can show “sampling” until another usable sample arrives. Short-lived exited children are not retained in historical totals. These are measured snapshots, not estimates or billing metrics.

Windows and macOS currently show lifecycle health and logs, with resource counters unavailable. Windows source/tests cross-compile; native execution is still pending. Existing macOS hard-crash cleanup limitations remain unchanged.

## Access model

The dashboard binds only to IPv4 loopback, requires an exact Host, rejects foreign Origins, sets no CORS permissions, and frames cannot embed it. Its bootstrap secret is sent in a URL fragment, exchanged through a same-origin JSON request for an HttpOnly, SameSite=Strict, browser-session cookie. Cookie names include the dashboard port so concurrent instances do not overwrite one another. Every instance generates fresh secrets kept in memory.

Project data requires authentication. Actions additionally require an exact same-origin request and a session CSRF header. The browser never receives per-project control tokens, control addresses, or saved launch arguments. The content security policy allows only bundled local scripts/styles and same-origin connections. No telemetry is sent.

Loopback HTTP does not install another CA. This protects against unsolicited web origins, not another program running as your OS user, a privileged local process, or someone who obtains the private bootstrap link. There is deliberately no LAN binding or remote dashboard mode.

## Workspace layout and appearance

The Projects directory lists apps in rows with their stack, recent service state, status, and lifecycle controls. Search by name/folder and filter by status; totals cover the entire workspace. Open a project for its own Overview, Configuration, Environment, Commands, and Logs sections. A project switcher and browser back/forward navigation keep the workspace easy to navigate.

Configuration is divided into Connection, Runtimes & database, PHP configuration, and Development services. The form saves all categories together. Drafts survive switching categories or sections within the same project. Leaving that project with unsaved configuration or environment changes asks before discarding them; browser reload/close also warns. Save controls wait for the current revision to load and prevent repeated submissions while saving. A save does not restart the app.

Commands includes shortcuts that only fill the form. The explicit Run action button starts the task. Activity lists the latest 50 named task records. Commands also lists that project’s own tasks. Selecting a record opens only its log and recorded setup steps, and active command tasks can be cancelled. App creation and runtime management have dedicated workspace pages.

Light is the default appearance, with Dark and System options in the header. The browser remembers the selection locally; a non-sensitive appearance cookie carries it across dashboard ports on the same host. Both palettes use the README's LaraServe logo and lime accent. All scripts, styles, and images remain bundled with no external fonts or assets. Layouts adapt to narrow screens and include visible keyboard focus.

## Live monitoring

Workspace pulse cards show running apps, measured CPU, process-tree resident memory, and active development services. They refresh with project status every three seconds while the page is visible. CPU/memory sparklines keep up to 30 actual refresh samples in this browser page; they are not persistent history. Unavailable counters and the first CPU sample are displayed explicitly. Sample counts identify partial measurement coverage. CPU can exceed 100% across cores, and summed RSS can count shared pages more than once. See the resource measurement limits above.

The directory includes Open app and Restart alongside Start/Stop. Open app is enabled only for a running app with a valid web URL; Restart is unavailable during preparation/stopping. Laravel metadata comes from the `laravel/framework` package in `composer.lock`, or falls back to the declared `composer.json` constraint. The column labels these sources as Locked or Constraint and does not claim a dependency is installed.

## Add an existing app and browse folders

Use **Add app** to choose a Laravel source folder containing regular `artisan` and `composer.json` files. Registration records the project privately without executing PHP, installing dependencies, modifying its files, starting services, or provisioning a database. Existing registrations, including older identities and saved launch options, are retained. Invalid project settings are reported before a new registration is created.

Both Add app and Create app include **Browse**. The local modal lists directories, supports Home/Parent, an editable absolute path, a filter, and hidden folders. Creating an app lets you choose a parent folder and new app folder name; choosing the destination creates nothing. An empty folder name selects the current folder, which must still satisfy the new-app empty-folder checks when creation runs.

Folder listings and registration are authenticated, same-origin, CSRF-protected POST requests. Listings return bounded directory names and paths, not file contents; symlink entries are omitted, while a deliberately entered symlink path is resolved to its canonical folder. Hidden folders are off by default. No native picker program or shell command is launched.

Dropdowns use a bundled custom control while retaining the form's original values. Arrow keys, Home/End, type-ahead, Enter/Space, Escape, Tab, and pointer selection are supported. Focus styling surrounds search fields rather than drawing a second outline around the inner input.

## Client preferences and project lifecycle

Settings contains Launch LaraServe at login, Open dashboard at login, opt-in daily release checks, a manual update check, and uninstall. Preferences live in private `preferences.json`, with revision checks to prevent overwriting concurrent edits. Release checks contact only the official repository's latest-release API and never download or install an update.

Run on startup belongs to each project's Overview. Selected apps start when the dashboard opens, or through the login helper when client login startup is enabled. Reusable launch options exclude migrations and one-shot consent flags. Login integration uses a per-home XDG autostart entry on Linux, a user LaunchAgent on macOS, and a current-user Run registry value on Windows. Keep the portable executable in a stable location. Login changes apply at the next sign-in.

Remove project requires a confirmation dialog, stops the owned app and services, clears its startup preference, and hides the registration. It preserves source files, private databases, and captured mail. Adding the same source folder restores its registration and options; legacy manifest imports do not resurrect removed apps.

Uninstall requires both acknowledgement of permanent managed-data loss and typing `UNINSTALL`. It uses the existing CLI uninstaller, refuses active projects/tasks, removes the login integration, and closes the dashboard after success. Managed databases, caches, preferences, captured mail, and recorded generated workspaces/vendor folders may be removed. The portable executable remains for manual removal. System hosts/trust cleanup can require elevated OS permissions. Back up managed data first.

## Creation, progress, and frontend commands

Create app exposes destination browsing, starter kit, database, domain, port, HTTPS, Vite, and a styled initial-migration switch. Runtime and skeleton version fields are omitted from this main form. Connection choices are saved in `.laraserve.json`. Choosing a starter kit selects Vite by default; it remains optional. Creation installs frontend dependencies with the project-selected managed Node runtime. With Vite selected it enables the live server; otherwise it builds frontend assets when a build script is available. Non-npm lockfiles are reported rather than rewritten with npm. Partial files are retained if installation fails.

Creation, runtime installation, and commands open a live output dialog. Successful creation opens the new project's Configuration; adding an existing app opens Configuration too. Errors remain available in the task output. Toasts show notifications at the viewport edge and can be dismissed; persistent failures are deduplicated. Lifecycle buttons expose pending states, and operation completion/failure produces a toast.

Commands offers npm install, ci, run build, audit, and outdated, plus install/build shortcuts. These run as cancellable owned tasks without a shell. npm uses the same Node selection as Vite and refuses a project that declares another package manager. Dependency installation can run package lifecycle scripts. Start now automatically prepares missing/outdated frontend dependencies for selected or detected frontends; explicit npm commands remain available for deliberate maintenance.

Dashboard and automatic starts pass `--setup-local`: unresolved requested domains are added to the hosts file and HTTPS attempts local CA trust without a terminal prompt. Unlike the older noninteractive fallback, a failed hosts update preserves the requested domain and fails visibly instead of silently substituting `.localhost`. Trust failures also stop preparation with an actionable message. Explicit `--no-hosts` and `--no-trust` choices remain respected. An unresolved requested domain with `--no-hosts` fails clearly instead of silently changing its name. System permission requirements still apply.

Open phpMyAdmin appears in each project's header. It is enabled only while that project's managed MySQL/phpMyAdmin instance is running, using its private per-run sign-in URL. It is disabled for SQLite, external databases, stopped apps, or disabled phpMyAdmin. Control tokens are never sent to the dashboard browser.


## Automatic preparation and visible stages

Start copies `.env.example` to `.env` only when `.env` is absent, using an exclusive private-file creation. Existing environment values are not overwritten. Missing Composer dependencies are installed using the project runtime. With automatic local-service detection enabled (the default), Vite is inferred from package dependencies. Local Redis and Mailpit are inferred from actual environment settings when needed; remote Redis/mail hosts are preserved. Queue workers, schedulers, and schema changes remain explicit choices.

Frontend installation is per project. LaraServe checks the project configuration Node pin, `.node-version`, `.nvmrc`, Volta, root package engines, and locked/installed Vite, webpack, Laravel Mix, Next, and Laravel Vite plugin engines. Unpinned projects prefer publisher-designated LTS releases. A private per-project Node lock preserves a compatible selection. Frontend input fingerprints avoid unnecessary repeated installs; missing modules or changed manifests/locks/Node versions trigger preparation again. Node changes rebuild native npm dependencies. Conflicting package managers and unsatisfiable version requirements produce explicit errors rather than modifying another manager’s lockfile.

Startup, creation, runtime downloads and command tasks report named stages, completion checks, skipped/not-run states, and the active subtask. Runtime downloads include actual byte/percentage progress when their size is known. Logs are collapsed by default. The actual error is shown inline, including relevant command output, rather than only referring to a log path. Command task logs are separate files with project names and command labels; startup logs are isolated by project and replaced for each new launch. Preparation stages are recorded separately from verbose output, so truncating a long log does not lose its checklist.

Automatic ports remain automatic on subsequent starts: the actual previously assigned port is stored separately from the requested port. An occupied automatic port is skipped; an explicitly occupied port fails with a clear explanation. Clearing Port restores automatic selection, and resetting the protocol restores the HTTPS default. Requested domains are preserved.

Linux administrator actions use a desktop polkit prompt or a configured `SUDO_ASKPASS`; an interactive terminal can still use sudo. macOS hosts changes use the system administrator prompt. Windows hosts changes request UAC. Passwords never pass through the browser, preferences, process arguments, or logs. Cancelling/denying authorization stops preparation. Headless systems without an authentication agent or cached authorization show the exact setup requirement. Native Windows/macOS authorization still requires platform verification.

phpMyAdmin accepts both loopback host aliases, and its sign-on cookie allows the dashboard’s navigation while retaining token validation and HttpOnly/Secure protections. Its project button opens a fresh per-run sign-in link; it is not a static link to the phpMyAdmin root.

### App workspace layout

The dashboard uses a persistent desktop sidebar with workspace navigation and project shortcuts showing current status. A compact toolbar displays the current section or project. Appearance and notification history remain available in the sidebar footer. On narrow screens, the sidebar becomes a keyboard-accessible drawer: closed navigation is inert, Tab stays inside the open drawer, and Escape closes it and restores focus to the navigation button. Existing project controls and settings remain available.
