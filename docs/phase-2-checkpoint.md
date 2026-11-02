# Phase 2 local checkpoint

## Delivered

- Canonical project registry with path/name/ID lookup, ambiguous-name rejection, atomic metadata, schema versioning, and safe legacy import.
- Foreground and detached background ownership with lifetime OS locks and generation-specific readiness.
- `projects`/`list`, `start`, `status`, `stop`, and `restart` commands.
- Stable domain/protocol/port assignments and reusable settings. Migrations and one-shot consent are never replayed.
- A shared launcher, authenticated loopback controls, and service-group interface for CLI and future dashboard consumers.
- Serialized preparation through readiness, protecting shared manifest/CA state and avoiding concurrent port assignments.
- Cancellation for preparation waits, runtime downloads, and Composer installation.
- Linux pipe guards with owned-group cleanup that also works for file-capability executables; Windows job-object implementation cross-builds.
- Verified stale MySQL ownership recovery without deleting database data; unknown/live locks remain protected.
- Preservation of legacy private state IDs and refusal to arbitrarily choose between duplicate legacy databases.
- Active-project protection for cleanup/uninstall; atomic footprint writes.
- PHP version checks tolerate harmless stderr warnings while preserving actual failure diagnostics.

## Verification

- Unit and race tests cover registry identity/ambiguity, duplicate owners, token/Host/Origin rejection, readiness aggregation, sibling cancellation, download cancellation, migration opt-in, and database lock preservation/recovery.
- Real CLI/PHP/Caddy integration checks detached serving, concurrent starts, two apps, legacy SQLite state preservation, stable restart URLs, duplicate rejection, active cleanup refusal, isolated stopping, owner crash cleanup, and restart recovery.
- Real MySQL integration verifies an app-named empty schema, explicit probe data, owner crash, stale-lock recovery, restart, and persistent records. Its Artisan fixture fails if migrations run unexpectedly.
- Linux verification uses Linux Mint 22 (Ubuntu 24.04 base), including a capability-enabled FrankenPHP executable.
- Windows source/test cross-compilation is checked; native Windows execution remains pending. macOS hard-crash cleanup remains deferred.

All application fixtures, server state, and probe databases are temporary. Tests do not install browser certificates or edit hosts. Hosted CI and public release publication remain inactive.

## Restore points

- Before: `restore/before-phase-2-2026-11-03`.
- Local checkpoint: `restore/phase-2-local-ready`.
- Branch: `codex/phase-2-supervision`.
- Standalone source backup: `dist/checkpoints/phase-2.bundle`.

See [development and restore instructions](development.md). Git restores source code, not private database contents.

## Next phase

Phase 3: a protected local management dashboard, beginning with project status and logs, then start/stop/restart controls using the shared lifecycle core. Keep credentials and private per-project control tokens on the backend.
