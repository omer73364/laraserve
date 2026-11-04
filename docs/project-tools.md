# Project settings and actions

Phase 4 adds the same project tools to the terminal and local dashboard. Open `laraserve dashboard`, select **Settings** on a card, then choose Settings, Environment, or Actions. New app and Runtimes have separate entry points.

## Configuration

Share `.laraserve.json` with the project:

```json
{
  "php": "8.3",
  "mysql": "8.4",
  "domain": "my-app.test",
  "https": true,
  "port": 8443,
  "database": "managed",
  "phpmyadmin": true,
  "php_settings": {
    "memory_limit": "256M",
    "upload_max_filesize": "32M",
    "post_max_size": "32M"
  },
  "php_ini": "opcache.revalidate_freq = 0"
}
```

Database modes are `auto` (detect the app), `managed` (private MySQL), `external` (honor the project's `.env` connection), and `sqlite` (private SQLite). Changing a mode or version never copies, upgrades, or deletes an existing database. MySQL versions retain separate private data directories.

Explicit flags in the current terminal request take priority over this file. The file takes priority over replayed settings; omitted fields can fall back to remembered choices and existing runtime locks. The editor includes older explicitly remembered service options, so saving a PHP limit does not silently re-enable MySQL or phpMyAdmin. Saving promotes those displayed choices into the shareable configuration.

Settings apply at the next start/restart, not halfway through a running generation. Domain/HTTPS changes may need trust or hosts setup through the terminal. Port `0` chooses a port using normal startup rules. Empty PHP/MySQL pins retain project requirement detection and compatible private version locks.

PHP changes use a derived private `php.ini`; shared downloaded runtime configuration is unchanged. Supported directives are memory/upload/POST sizes, execution/input timeouts, input variable limits, timezone, display/log errors, and common opcache toggles/revalidation settings. Both form and raw fields use the same allowlist. Duplicate directives between those fields, arbitrary sections, extensions, and code-loading hooks are rejected. Unavailable timezone data also fails validation rather than silently choosing another zone.

```sh
laraserve settings my-app
laraserve settings my-app --file ./project-options.json
```

The terminal file is a replacement configuration object, not a patch. Unknown properties are rejected. Direct edits are read at the next startup. The dashboard rejects stale revisions instead of overwriting edits made elsewhere.

## Environment editor

The Environment tab compares key names with `.env.example` and identifies missing and additional variables. A small set of operational fields is readable; credentials, tokens, keys, and unrecognized/custom variables stay masked in both current and example values. Secrets are never returned merely to populate the form.

Only changed inputs are sent. Unchanged masked fields preserve their original values; explicitly replacing one with an empty value clears it. Adding a variable is supported; removing a key or editing complex multiline dotenv syntax remains an editor operation. Literal values preserve `$`, quotes and backslashes without shell expansion. Comments and existing line endings are preserved.

```sh
laraserve env my-app
laraserve env my-app --file /private/path/changes.json
```

A changes file is a JSON map such as `{"APP_NAME":"My app"}`. Keep files containing credentials private and out of Git. API/terminal output remains masked after saving. Configuration and environment files must be bounded regular files; symlinks and unsupported syntax are refused. Before replacing an existing file, LaraServe stores a private `0600` backup under `~/.laraserve/history/<project-id>/`. Backups can contain secrets; do not publish them. Cleanup retains these backups for deliberate recovery; uninstall removes the LaraServe home.

Source settings/environment are edited for copy-mode projects, then recopied on restart. Bundles are read-only: edit their source and repack them.

## Artisan and Composer tasks

Tasks run as owned child processes with live recent output, final exit status, and cancellation. Arguments are passed directly to executables, without a shell. The dashboard accepts a command and a quoted argument list; no shell expansion or pipelines are performed.

```sh
laraserve artisan my-app -- about
laraserve artisan my-app -- route:list
laraserve artisan my-app --confirm-schema -- migrate --force
laraserve composer my-app -- validate --no-check-publish
laraserve composer my-app -- install --no-dev
laraserve task TASK_ID
laraserve task TASK_ID --wait
laraserve task TASK_ID --cancel
```

Artisan requires a running, ready project and uses that exact owner's recorded PHP, environment, private database credentials and state paths. Existing owners from an earlier client version need one restart to record this private execution context. If the app stops or changes generation, its task is cancelled. Persistent/interactive workers such as `serve`, `tinker`, `queue:work`, and `schedule:work` are excluded from this batch runner.

Known schema-changing commands (`migrate*` except `migrate:status`, `db:seed`, `db:wipe`, and `schema:*`) require a separate confirmation checkbox or `--confirm-schema`. These are deliberate user actions; start/restart never replay them. Application commands still run application code and may have their own effects.

Supported Composer commands are `install`, `update`, `dump-autoload`, `validate`, `audit`, `check-platform-reqs`, and `show`, with a limited set of switches. They run with scripts and plugins disabled. The implicit dependency install during `serve` also skips scripts/plugins, preventing lifecycle hooks from invoking migrations during startup. Projects requiring custom Composer hooks/plugins need a deliberate setup step outside this runner.

Only one task per project is accepted at a time; global creation/download tasks also serialize. Queued tasks block cleanup/preparation just like running ones. Settings/environment writes refuse to race an active project task. Runtime cleanup waits for all tasks and projects to stop. Task requests and logs are private; task APIs omit request arguments and execution credentials. The browser sees only a bounded recent output tail (256 KiB); the local task log keeps the complete output.

Tasks survive closing the dashboard. Reopening it in the same browser session restores the most recently viewed task. The terminal can reconnect using its ID. Cancelling does not undo completed package installs, file writes, or database work. Tasks have a 30-minute execution limit; interrupted owners are reported rather than treated as success. After an owner crash, a 25-second recovery hold lets owned child guards finish before another task or cleanup can touch the same files. Foreground CLI task commands return failure on unsuccessful task exit and display its actual code.

## New apps and migrations

**New project creation may run initial migrations**, as requested. This exception applies only to a new or empty destination. It does not change the existing-project migration policy, and a fresh database for an existing app still requires an explicit migration action.

```sh
laraserve new /absolute/path/my-app
laraserve new /absolute/path/my-app --starter react --database sqlite
laraserve new /absolute/path/my-app --starter livewire --database mysql
laraserve new /absolute/path/legacy-app --version '^8.0' --php-version 7.4 --migrate=false
laraserve new /absolute/path/my-app --wizard --php-version 8.5 --host-path
```

The built-in form/flags select official `laravel/laravel`, `laravel/react-starter-kit`, `laravel/vue-starter-kit`, `laravel/svelte-starter-kit`, or `laravel/livewire-starter-kit` Composer packages. Composer resolves a package compatible with the selected PHP. `--version` applies only to the plain skeleton. Creation skips package scripts, prepares `.env`, and runs initial migrations against the new private SQLite/MySQL database by default. Use `--migrate=false`, or clear the dashboard checkbox, to suppress them. External mode does not migrate an external database by default.

A destination with retained private database state is refused, even if its app folder was deleted or emptied: a new app must not apply its migrations to an old app's data. A failed creation or migration retains partial files and data for inspection. It never retries by deleting the destination. Newly created apps are registered; they are not left serving automatically. Starter-kit frontend dependencies/builds are a separate step until Phase 5.

`--wizard` downloads and caches the actual [Laravel installer](https://github.com/laravel/installer), then runs its native prompts in a real terminal with private PHP/Composer launchers. It currently supports SQLite creation, leaves Node and Boost installation off, and follows the installer's own prompts/setup for starter kits and tests. Use the built-in starter flow for managed MySQL, external connections, historical skeleton constraints, or suppressing migrations. The official installer currently requires modern PHP; an incompatible selection fails Composer's platform checks. Its upstream behavior and network requests belong to the installer.

After the native wizard finishes, the initial schema is also prepared in LaraServe's private database, so later serving uses an initialized schema. Native wizard migrations in the project-local SQLite file are separate and are not a data-import operation. The interactive installer stays in the foreground terminal process group; normal interrupt/cancellation is supported, but Linux hard-crash descendant cleanup for that interactive path is not claimed. Noninteractive dashboard/terminal tasks retain the Phase 2 pipe guards / Windows job ownership.

## Runtime library

```sh
laraserve runtimes list
laraserve runtimes install php 8.3.36
laraserve runtimes install mysql 8.4.13
laraserve runtimes remove CACHE_KEY --confirm
```

The dashboard offers catalog downloads, installed-cache sizes and deliberate removal of an unreferenced published PHP/MySQL cache. It does not embed runtime payloads or use static download checksums; the existing source/checksum verification stays in place.

Cleanup rechecks all project/task owners, recorded runtime references, and download locks. Referenced versions and shared Composer/NSS/library/platform-helper caches are protected. A project without an execution snapshot conservatively protects caches until it has been started with this version. Missing, incomplete, symlinked-root or unknown cache directories are not removal candidates. Cache removal does not touch project files, history, SQLite, or MySQL data. A removed unused runtime can be downloaded again later.

Windows source/tests cross-compile. Native Windows execution and platform-specific wizard behavior remain unverified; existing macOS process/resource limitations still apply.

## Optional development services

Node pins and the `services` object can be edited in project settings. See [development services](development-services.md) for Node/npm commands, Vite, Redis, Mailpit, queue and scheduler behavior, and the boundaries for non-npm managers.
