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

Migrations, blanket yes-to-all permission, and one-shot insecure dependency consent are never replayed. Phase 4 adds validated settings, masked environment editing, deliberate Artisan/Composer tasks, and new-app creation. Known schema-changing actions require explicit confirmation for existing apps; initial migrations are allowed when creating a new app. No shell commands or arbitrary-file editing endpoints are exposed. Use the terminal for initial setup that needs interactive trust or dependency consent. See [project tools](project-tools.md).

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

Commands includes shortcuts that only fill the form. The explicit Run action button starts the task. Activity displays the latest task and supports hiding/reopening output and cancelling an active task. It is not a full historical task browser. App creation and runtime management have dedicated workspace pages.

Light is the default appearance, with Dark and System options in the header. The browser remembers the selection locally; a non-sensitive appearance cookie carries it across dashboard ports on the same host. Both palettes use the README's LaraServe logo and lime accent. All scripts, styles, and images remain bundled with no external fonts or assets. Layouts adapt to narrow screens and include visible keyboard focus.

## Live monitoring

Workspace pulse cards show running apps, measured CPU, process-tree resident memory, and active development services. They refresh with project status every three seconds while the page is visible. CPU/memory sparklines keep up to 30 actual refresh samples in this browser page; they are not persistent history. Unavailable counters and the first CPU sample are displayed explicitly. Sample counts identify partial measurement coverage. CPU can exceed 100% across cores, and summed RSS can count shared pages more than once. See the resource measurement limits above.

The directory includes Open app and Restart alongside Start/Stop. Open app is enabled only for a running app with a valid web URL; Restart is unavailable during preparation/stopping. Laravel metadata comes from the `laravel/framework` package in `composer.lock`, or falls back to the declared `composer.json` constraint. The column labels these sources as Locked or Constraint and does not claim a dependency is installed.

## Add an existing app and browse folders

Use **Add app** to choose a Laravel source folder containing regular `artisan` and `composer.json` files. Registration records the project privately without executing PHP, installing dependencies, modifying its files, starting services, or provisioning a database. Existing registrations, including older identities and saved launch options, are retained. Invalid project settings are reported before a new registration is created.

Both Add app and Create app include **Browse**. The local modal lists directories, supports Home/Parent, an editable absolute path, a filter, and hidden folders. Creating an app lets you choose a parent folder and new app folder name; choosing the destination creates nothing. An empty folder name selects the current folder, which must still satisfy the new-app empty-folder checks when creation runs.

Folder listings and registration are authenticated, same-origin, CSRF-protected POST requests. Listings return bounded directory names and paths, not file contents; symlink entries are omitted, while a deliberately entered symlink path is resolved to its canonical folder. Hidden folders are off by default. No native picker program or shell command is launched.

Dropdowns use a bundled custom control while retaining the form's original values. Arrow keys, Home/End, type-ahead, Enter/Space, Escape, Tab, and pointer selection are supported. Focus styling surrounds search fields rather than drawing a second outline around the inner input.
