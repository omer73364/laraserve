# Phase 5 local checkpoint

## Delivered

- Optional private Node downloads with current publisher SHA256 metadata, stable release discovery, exact/partial pins, engine constraints, cached selections and offline restarts.
- Explicit project-aware Node/npm commands, package-manager detection and conflict handling. pnpm/Yarn/Bun dependencies can be used after installation by their intended manager; automatic installation of those managers is deferred.
- Vite using the existing project configuration, reserved loopback port, app origin/host and HTTPS certificate.
- Optional private Redis and Mailpit instances, project-specific ports, persistent state and injected connections. Linux amd64 Redis packages and missing support libraries download privately without system installation.
- Queue and scheduler workers sharing the selected PHP runtime and private application environment.
- Group readiness, sibling cancellation on failure, owned process cleanup, per-service status and logs, and dashboard service controls.

See [development services](development-services.md) for setup, commands, data locations, package-manager boundaries, Redis platform requirements, queue side effects and remaining limits.

## Verification

- Unit/race checks cover Node constraints, pins and locks, package-manager conflicts, publisher checksum lookup/failure, cached release catalogs, actual process readiness/cancellation/exit codes, and generation-aware service status.
- Real PHP/MySQL/dashboard lifecycle regressions pass. A new real worker integration test verifies both worker logs/readiness, exit-failure propagation, recovery, and stop.
- A disposable project used a freshly downloaded and verified Node runtime, installed Vite, and started Vite, Redis, Mailpit, queue and scheduler together. The application and Vite client served successfully; SMTP delivery to Mailpit succeeded. Both app and Vite HTTPS certificates were verified against an isolated CA without changing host trust.
- Redis was downloaded from current Ubuntu package indexes with its support libraries. Authenticated writes worked and persisted across restart. An actual queue-process failure shut down the app and managed siblings.
- Browser checks verify saving a Node pin and selected services, live service status and available service log sources.
- Linux tests/static checks and Windows source/test cross-compilation are run locally. Native Windows/macOS service execution is not claimed as verified. No GitHub workflow or public release is activated.

## Restore points

- Before: `restore/before-phase-5-2026-11-03`.
- Tested local result: `restore/phase-5-local-ready`.
- Branch: `codex/phase-5-development-services`.
- Standalone history backup: `dist/checkpoints/phase-5.bundle`.

Git restores source, not private Redis/MySQL/SQLite data, captured mail, environment backups or downloaded caches. No changes have been pushed.

## Next phase

Phase 6: benchmark database resource use, then design optional shared MySQL with project databases and scoped credentials before adding database operations and other engines.
