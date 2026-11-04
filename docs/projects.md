# Projects and background services

LaraServe can keep several projects running independently. Foreground `serve` is still available; `start` runs the same workflow in a detached owner process.

```bash
laraserve start ./my-app --http
laraserve projects
laraserve status ./my-app
laraserve restart ./my-app
laraserve stop ./my-app
```

Commands accept a registered project's path, unique folder name, or ID. `list` is an alias for `projects`. If two projects have the same folder name, use their IDs or full paths. With no target, lifecycle commands use the current directory.

## Registration and settings

`serve` and `start` register projects automatically. Registry and control files live privately under `LARASERVE_HOME` (by default `~/.laraserve`):

```text
registry/<id>.json          source, resolved root, reusable options, versions, domain and port
control/<id>.json           private owner generation and control credentials
control/<id>-startup.log    private preparation and server output
```

The project ID follows its canonical source path. Symlink aliases share an ID; names alone are not unique. Moving a project to a different path is not an automatic data migration. Shareable options now live in `.laraserve.json`; these machine-specific registration records should not be committed to an app's Git repository. See [project tools](project-tools.md).

Existing direct project footprints are imported when listing or managing projects. Their old private state IDs are retained so SQLite and MySQL data are not silently replaced. Duplicate legacy states for the same canonical folder stop startup and require deliberate resolution. Older private workspaces lack a recorded source path, so register them by serving the original source/bundle again rather than treating an internal workspace as a new project.

Domain, protocol, and assigned port are remembered. Restart reuses them; an occupied remembered port produces an error rather than silently switching the app's URL. Override it explicitly:

```bash
laraserve restart ./my-app --port 8090
```

Other explicitly supplied reusable serve flags are remembered. One-shot actions are excluded: `--migrate`, `--yes`/`-y`, and `--allow-insecure-dependencies` are never replayed by an ordinary restart. To deliberately migrate during a restart:

```bash
laraserve restart ./my-app --migrate
```

## Initial setup and readiness

Background owners have no interactive input. For first-time HTTPS approval or an administrator password prompt, run `serve` once in a terminal, or choose `--http`. After approval, normal startup retains the automatic trust recovery from Phase 1. A separate `trust` command is not required for normal browser-import retries.

`start` waits up to a minute for the exact launched generation to become ready. Longer first-use downloads continue in the background; the command prints how to inspect status and its private log. A waiting client's exit does not stop a detached project.

States include `starting`, `running`, `stopping`, `stopped`, `failed`, `crashed`, and `unresponsive`. Running means this server generation passed its own readiness check; it does not validate every application route.

Preparation is serialized to protect shared footprint/CA state and to reserve the actual listening port through readiness. Once started, projects run concurrently. Stopping a project waiting for setup also cancels its wait. Runtime downloads and Composer installation are tied to the project's lifetime.

## Ownership and shutdown

Each project holds an OS lock for its full lifetime. A lock file left after a crash is not itself evidence of a running process. Duplicate foreground and background owners are rejected.

Controls are bound to an ephemeral `127.0.0.1` port, require a private random token, validate Host, reject browser Origin requests, and never follow redirects or use HTTP proxies. Tokens are not printed by status commands. The shared registry, launcher, lifecycle controls, and service-group interfaces are intended for the later web UI as well as the CLI.

`stop` requests shutdown from the authenticated owner and waits for its lifetime lock to be released. It does not kill a PID read from a file or an unrelated process using a port. `clean` and `uninstall` refuse to remove newly managed active projects.

On Linux, owned commands run behind a pipe-based guard, including capability-enabled server binaries. When an owner dies, the guard shuts down its fresh process group and can force termination after a bounded grace period. The unreaped group leader is retained until signaling is complete to avoid PID/group reuse. Normal stop allows MySQL to flush data. Descendants of completed managed commands are also cleaned up.

Windows workers use a kill-on-close job object for crash ownership; ordinary MySQL shutdown still uses its authenticated shutdown path. This implementation cross-builds but still needs native Windows validation. macOS keeps normal context-based shutdown; hard-crash cleanup remains unverified and is not implemented with the Linux guard.

## Database recovery

New managed MySQL instances hold a separate OS owner lock and record their owned server/guard PID. Following an owner crash, startup waits for a verified previous owner to finish, then removes only its known stale lock metadata. Persistent database files and version-specific data directories are not deleted or migrated.

A live or unverifiable PID, a legacy lock without ownership metadata, unknown lock-directory contents, or an incomplete database initialization is retained for inspection. Stop older LaraServe processes before upgrading their management workflow. Do not remove an unverified lock or database directory merely to bypass a startup error.

Shared MySQL engines between apps are not part of Phase 2; they remain in Phase 6.
