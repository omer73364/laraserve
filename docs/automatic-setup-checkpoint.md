# Automatic setup and progress checkpoint

Build: `0.1.0-phase5.1-ui.5`. This extends Phase 5; no Phase 6 or GitHub workflow work.

## Behavior

Start prepares a missing environment file, PHP/Composer dependencies, project-compatible Node/frontend dependencies, and detected local frontend/Redis/mail services. Existing databases are never automatically migrated. Queue workers and schedulers stay deliberate selections.

Preparation has named steps, checked completion, skipped/not-run states, download counters, an inline actual error, and collapsible logs. Activity and each project’s Commands show named task history with separate output. Common shortcuts include list, migrations/status/rollback, cache/config/view clearing, storage linking, queue restart, and npm install/build.

Automatic port choices are separate from actual assigned ports, and domain names are never silently substituted in automatic setup. Hosts/trust operations request OS authorization without collecting passwords in LaraServe. The phpMyAdmin sign-on cookie and loopback host routing have been corrected.

## Validation

- Race-enabled Go tests, vet, JavaScript syntax, and diff checks.
- Windows amd64 and macOS arm64 cross-builds. Native authorization dialogs are not verified on those platforms; Linux prompt selection is tested with harmless mocked executables. No test changes the real hosts file or trust store.
- Disposable official React app: removed `.env` and frontend dependencies before Start; Start copied the example, installed npm dependencies, selected Node, started Vite, avoided an occupied automatic port, and completed the structured checklist. Frontend asset compilation also succeeded.
- Browser: Start checklist, direct occupied-port error, collapsed task output, named Artisan list task, per-project command history, common command shortcuts, and authenticated phpMyAdmin showing the disposable database and logged-in user.
- Cookie/redirect HTTP check reached authenticated phpMyAdmin with no access-denied/sign-in-link error. The link is absent after stop.
- Tests cover distinct compatible Node selections for different build-tool constraints, LTS preference/pins, preserved environment secrets, preserved remote service connections, project-history isolation, stage failure, and progress API authentication.

## Recovery

Before: `restore/before-automatic-setup-2026-11-04`.
After: `restore/automatic-setup-local-ready`.
Bundle: `dist/checkpoints/automatic-setup.bundle`.
Screenshots: `dist/checkpoints/automatic-setup-*.png`.
