# Local dashboard

Run `laraserve dashboard` and open the complete private link printed in the terminal. The browser receives a session, then removes the login fragment from its address bar. Keep the original link private. `--port 9779` selects a fixed port; the default chooses an available port on `127.0.0.1`.

The dashboard is included in the client. It needs no Node installation, frontend build, network assets, account, hosted service, or separate PHP runtime. Closing it leaves independently owned projects running. Run the command again to obtain a fresh dashboard link and session.

## Projects and controls

Existing registered projects appear with their state, local URL, PHP version, database selection, source folder, and owner health. Direct legacy project registrations are imported using the same logic as the terminal. Old workspace records without a known source cannot safely be imported.

- **Start** uses saved reusable options and launches the existing project in the background.
- **Stop** contacts that project's authenticated owner and waits for its resources to stop.
- **Restart** stops the current owner and launches a new generation with its saved settings.
- **Open app** opens a running app in a separate browser tab.
- **Logs** selects the project in the log viewer.

Actions run asynchronously; project cards show progress and failures. A second dashboard action for that project is rejected while its first action is pending. The shared supervisor also prevents duplicate owners across the terminal and other dashboard sessions.

Migrations, blanket yes-to-all permission, and one-shot insecure dependency consent are never replayed. The dashboard accepts no shell commands, extra launch flags, migration requests, or arbitrary filesystem paths. Use the terminal for a first run that requires interactive trust or dependency consent, and for changing options until Phase 4.

The page refreshes project status every three seconds while visible. Operation progress belongs to this dashboard process; actual project ownership remains independently queryable after the dashboard exits. A preparation timeout ends the dashboard's wait, not a detached project's lifetime. Check status and startup logs before starting it again.

## Logs

Choose Laravel, Web server + PHP, MySQL, or Startup, then enter a case-insensitive literal search. Refresh logs explicitly when needed. PHP and web server output share one stream. Search covers the recent tail, not the full historical file.

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
