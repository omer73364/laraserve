# Development services

Optional services share the project's owner: one start launches them, one stop waits for their cleanup, and an unexpected exit cancels the other selected processes. Services are off by default. Enable them in dashboard settings or in `.laraserve.json`:

```json
{
  "node": "22",
  "services": {
    "vite": true,
    "redis": true,
    "mailpit": true,
    "queue": true,
    "scheduler": true
  }
}
```

Start or restart the project after saving. Queue workers consume real queued jobs; scheduled commands can change application data. Enable these only when wanted. Set up a working queue connection and its schema explicitly before enabling a worker. Starting an existing application still never runs migrations automatically.

## Node and frontend dependencies

```sh
laraserve node /path/to/app -- --version
laraserve npm /path/to/app -- install
laraserve npm /path/to/app -- run build
```

Node downloads only for an explicit Node/npm command or enabled Vite. Selection checks `node` in `.laraserve.json`, then `.node-version`, then `.nvmrc`, and always checks `package.json`'s `engines.node`. `node`, `latest` and `lts/*` choose a compatible release from the stable even-numbered Node 14+ catalog. Compatible previous selections stay locked privately; exact older patch pins are supported when listed by Node's publisher for the platform. Unsupported aliases fail with a version-selection error. Old Node releases remain available for old apps; they do not become supported or receive updates from LaraServe.

The release index is cached for 24 hours and retained for offline restarts. Published archives use Node's current `SHASUMS256.txt`; checksums are not embedded in the client. Linux/macOS amd64/arm64 and Windows amd64 use official distributions. Windows arm64 is also recognized by the downloader, but is not a LaraServe release target.

`packageManager` and npm, pnpm, Yarn and Bun lockfiles identify the intended manager. Conflicting managers fail instead of silently rewriting a lockfile. The bundled npm CLI is available through `laraserve npm` for npm projects. pnpm/Yarn/Bun dependency installation remains a separate step using that manager; LaraServe does not yet install those managers. Vite can use their already-installed dependencies. `laraserve npm` is an explicit normal npm invocation, so package lifecycle scripts run normally unless `--ignore-scripts` is supplied.

Vite requires installed `node_modules/vite`. LaraServe starts Vite's API with the project's existing Vite configuration, a reserved private loopback port, strict port binding, the app's origin and allowed host, and the app's HTTPS certificate when HTTPS is enabled. Custom `package.json` development scripts are not substituted for Vite. Node is added to the selected development processes' PATH. Vite remains loopback-only even when the PHP app is exposed to the LAN.

## Redis

An explicit `LARASERVE_REDIS` executable takes precedence, followed by an installed `redis-server`. Linux amd64 can otherwise download and privately extract Ubuntu 22.04's maintained Redis packages and support libraries. It requires glibc 2.35+. Package URLs and SHA256 values come from current publisher package indexes, with cached metadata for offline restarts. No system package installation, root permission, package scripts, or host Redis service is used. Other platforms require an installed/explicit Redis executable, or an external connection with private Redis disabled.

Each project gets its own loopback port, random password and append-only data in its private project state. Connection variables are injected into PHP and workers; they are not written into the source `.env` or returned by dashboard service status. Turning off private Redis restores the project's normal external Redis settings. Explicit project cleanup removes this private Redis state.

## Mailpit

Mailpit downloads from its official GitHub release and verifies the publisher digest on first use. Its cached release selection allows offline restarts. Each project has private loopback SMTP and inbox ports plus a persistent message database in private project state. Laravel mail variables target that inbox while the service is enabled. The terminal prints the inbox URL. Explicit cleanup removes captured messages.

## Workers, status and logs

Queue uses `artisan queue:work --sleep=1 --tries=1 --timeout=60`; scheduler uses `artisan schedule:work`. Both share the selected PHP runtime, project INI, private application environment, and database. They wait for managed Redis/Mailpit readiness. Laravel 8+ is the intended scheduler baseline. Custom worker commands, Horizon, multiple workers and automatic retry policies are deferred.

Dashboard cards show each service's status. Project logs offer Vite, queue, scheduler, Redis and Mailpit streams alongside existing sources. Logs are recent bounded reads of private files; logs may contain application secrets. Worker readiness means the process survived startup, not that every scheduled task or queued job will succeed. A later process exit fails the service group rather than silently claiming it is healthy.

Commands and downloads respond to cancellation. Linux ownership guards also clean children after owner death; Windows uses the existing job mechanism. Native Windows/macOS execution of these additions still needs platform testing. Worker shutdown uses the existing bounded termination grace; unfinished jobs follow the application's normal retry semantics.

Frontend dependency installation and custom service commands are not exposed as new dashboard tasks in this phase. Runtime-library removal conservatively protects Node, Mailpit and Redis caches as shared dependencies.
