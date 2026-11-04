# Phase 4 local checkpoint

## Delivered

- Shared validated configuration for domain, protocol, port, runtime pins, database mode, phpMyAdmin, and allowlisted PHP settings/raw directives.
- Explicit request flags beat the project file; the file beats replayed flags. Old remembered service choices are promoted visibly when settings are saved.
- Private derived PHP configuration, optimistic edit revisions, private backups, and masked environment comparison/update without returning stored secrets.
- Owned asynchronous Artisan/Composer tasks with output, actual exit codes, cancellation, generation matching, one-task ownership, and CLI reconnection.
- Cleanup/preparation protection for queued/running tasks. Composer scripts/plugins are excluded from existing-project installs/actions; known Artisan schema commands require explicit confirmation.
- Shared new-app creation with official starter packages, database choices, and creation-only initial migrations. Actual terminal Laravel installer integration is available with `--wizard` (SQLite).
- On-demand runtime download tasks, installed cache sizes, and guarded removal of unreferenced published PHP/MySQL caches.
- Dashboard workbench, new-app form, runtime library and task monitor, using the same core as terminal commands.

## Verification

- Unit/race tests cover config validation/conflicts/backups, PHP isolation, secret masking and literal dotenv quoting, remembered service settings, creation destination protection, schema consent, and cache references/data preservation.
- Extended real dashboard integration checks live PHP settings after restart, masked environment updates preserving credentials, task output, exit 7 propagation, explicit migrations, and cancellation without stopping the app.
- A real Laravel 13 app was created from official Composer sources, migrated in its private SQLite database, served over HTTP, and operated through Artisan. A newly added migration remained unapplied across subsequent start/restart.
- A second real Laravel creation used managed MySQL initial migrations, then served successfully and reported those migrations applied through Artisan.
- The actual Laravel installer 5.32.0 completed native terminal prompts, creation, SQLite migrations and Pest setup. A Composer validate task then completed successfully against that generated app.
- Browser review checks settings save, masked environment/example comparison and task progress/output at the available narrow viewport.
- Linux race/static checks and Windows source/test cross-compilation pass. Local archives are built for all five existing targets; checksums and source-bundle restore are verified.

See [project tools](project-tools.md) for bounded Composer/INI support, read-only bundles, strict dotenv syntax, recent-output limits, task cancellation effects, conservative cleanup, and native wizard/platform limitations. Native Windows, every starter-kit combination, and MySQL creation through the native interactive installer are not claimed as verified.

## Restore points

- Before: `restore/before-phase-4-2026-11-03`.
- Local checkpoint: `restore/phase-4-local-ready`.
- Branch: `codex/phase-4-project-tools`.
- Standalone source backup: `dist/checkpoints/phase-4.bundle`.

Git restores source, not private application data or environment backups. No code has been pushed, no hosted workflow activated, and no public release published.

## Next phase

Phase 5: optional Node/version selection, package managers and Vite, followed by Redis/Mailpit, queues and scheduler services using the shared lifecycle core.
