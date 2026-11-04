# Development, releases, and restore points

## Phase-by-phase work

The roadmap lives in [ROADMAP.md](../ROADMAP.md). Each phase should have acceptance criteria, focused commits, recorded verification, and a restore point. Current work is on `codex/phase-4-project-tools`. Earlier phases remain available at their own checkpoints.

The baseline before Phase 1 is the annotated tag `restore/before-phase-1-2026-11-02`. These are source-code restore points; databases and private runtime state are separate and are not stored in Git.

Inspect a checkpoint without changing the working tree:

```bash
git show restore/before-phase-1-2026-11-02
```

Create a separate checkout of the old code without discarding current work:

```bash
git worktree add --detach ../laraserve-before-phase1 restore/before-phase-1-2026-11-02
```

The end-of-phase local checkpoint is `restore/phase-1-local-ready`. A standalone Git bundle is saved at `dist/checkpoints/phase-1.bundle` after committing the phase. A bundle can restore source history in a new directory:

```bash
git clone -b codex/phase-1-foundation /absolute/path/to/phase-1.bundle laraserve-restored
```

Never reset or delete private database directories to roll back code.

## Local verification

```bash
make check
go test -race ./...
```

Integration tests are opt-in and use temporary state. Linux browser recovery can be tested without modifying the actual system or browser trust stores:

```bash
LARASERVE_TEST_TRUST_REPAIR=1 go test ./internal/trust -run TestRealBrowserTrustRecovery -v
```

See [Windows setup](windows.md) for native Windows tests. Cross-compilation checks that source builds for Windows; it does not execute Windows programs.

`docs/ci-workflow.example.yml` is an inactive template covering Ubuntu 22.04, Ubuntu 24.04, and Windows Server 2022. No automatic workflow is installed under `.github/workflows`, per the owner's request. Hosted CI runs and release publication are separate follow-up work.

## Local release artifacts

```bash
make dist
make release VERSION=0.1.0-phase1.2
```

`make dist` cross-compiles the clients without runtime payloads and generates `dist/SHA256SUMS` for the five raw executables. Linux, macOS, and Windows build outputs do not imply equal runtime validation across platforms; consult the support matrix.

`make release` additionally prepares `dist/releases/v<VERSION>/` containing ZIP archives and `SHA256SUMS`. Each archive includes its client executable, the MIT license, Go dependency license notices, and upstream license texts. Archive timestamps are fixed for reproducible packaging. Build from a clean tested commit when preparing public releases. Existing versioned release directories are never overwritten; choose another version for changed artifacts.

Verify locally:

```bash
cd dist/releases/v0.1.0-phase1.2
sha256sum -c SHA256SUMS
```

Checksums detect changed files; they are not release signatures. These commands do not upload files, publish a release, push Git changes, or run GitHub Actions.

## Phase 2 checkpoint

The Phase 2 baseline is `restore/before-phase-2-2026-11-03`; the tested local result is `restore/phase-2-local-ready`. A standalone history bundle is saved as `dist/checkpoints/phase-2.bundle`. Clone that bundle with `-b codex/phase-2-supervision` to restore it in a new directory without changing existing work.

See [Phase 2 evidence](phase-2-checkpoint.md) and [project lifecycle](projects.md). Real Linux lifecycle tests use an explicitly supplied client and existing trusted runtimes, with isolated temporary projects and databases:

```bash
LARASERVE_TEST_LIFECYCLE_BINARY=/absolute/path/to/laraserve \
LARASERVE_TEST_FRANKENPHP=/absolute/path/to/frankenphp \
LARASERVE_TEST_MYSQL_RUNTIME=/absolute/path/to/mysql-distribution \
go test ./integration -run 'TestReal(ProjectLifecycle|MySQLLifecycleRecovery)' -v -timeout 5m
```

The MySQL test creates a temporary runtime view and fresh data; the original runtime and all user databases remain untouched. Omit the MySQL variable to run only the web lifecycle test. No workflow is activated and no source or artifact is published by these commands.

## Phase 3 checkpoint

The local dashboard and its limits are documented in [Phase 3](phase-3-checkpoint.md). Restore to `restore/phase-3-local-ready`, or clone the verified standalone `dist/checkpoints/phase-3.bundle` with branch `codex/phase-3-dashboard`.

The opt-in real dashboard test uses the same temporary PHP fixture settings as the Phase 2 lifecycle tests:

```sh
LARASERVE_TEST_LIFECYCLE_BINARY=/absolute/path/to/laraserve \
LARASERVE_TEST_FRANKENPHP=/absolute/path/to/frankenphp \
go test ./integration -run TestRealDashboardLifecycle -v -timeout 3m
```

Use the built client, not a `go run` wrapper: detached workers must be able to reopen the same executable. The dashboard assets are embedded directly from `internal/dashboard/assets`; no Node build or external CDN is required. `node --check internal/dashboard/assets/app.js` optionally verifies script syntax when Node is available.

## Phase 4 checkpoint

See [Phase 4](phase-4-checkpoint.md) for the verification record and [project tools](project-tools.md) for behavior/limits. Restore source at `restore/phase-4-local-ready`, or clone `dist/checkpoints/phase-4.bundle` with branch `codex/phase-4-project-tools`.

The existing opt-in `TestRealDashboardLifecycle` now also checks settings/INI, masked environment edits, task output/exit status/cancellation, and explicit migrations. The real network creation checks use disposable destinations, a private `LARASERVE_HOME`, and a known `LARASERVE_FRANKENPHP`; run `new`, verify private SQLite schema, then serve, add a pending migration and restart to verify it remains unapplied. The native `--wizard` test needs actual terminal stdin (not a heredoc) and Composer access to official packages.

Task workers reopen the built executable through `__project-task`; terminal and dashboard both use this core. `execution/<project-id>.json`, task requests/logs and history backups are private and may contain credentials or application secrets. Never include them in a source backup or support report.

## Phase 5 checkpoint

See [Phase 5](phase-5-checkpoint.md) and [development services](development-services.md). Restore source at `restore/phase-5-local-ready`, or clone `dist/checkpoints/phase-5.bundle` with branch `codex/phase-5-development-services`.

The opt-in `TestRealDevelopmentWorkerLifecycle` uses the same client/PHP environment variables as the other lifecycle tests and checks worker readiness, output, failure propagation, restart and stop. Actual Node, Vite, Redis and Mailpit download/network checks use an isolated temporary `LARASERVE_HOME`; no payload is embedded or added to source history.

### Dashboard behavior checks

Run `make check-ui` with a local Node installation to check dashboard JavaScript syntax and the asynchronous state/notification regressions. It uses Node's built-in test runner and installs no packages. Keep these checks alongside `go test -race ./...` and `go vet ./...` when changing client state or shared project operations.

Project preparation can be inspected without changing files using `laraserve check ./project` or `laraserve check ./project --json`. A blocking issue produces a nonzero exit status; automatic preparation and warnings do not. The private general support report remains a separate `laraserve doctor --report FILE` operation.
