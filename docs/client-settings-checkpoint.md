# Client settings and creation UX checkpoint

Local build: `0.1.0-phase5.1-ui.4`. No push, publication, workflow execution, or Phase 6 work.

## Included

- Reversible project removal with confirmation and retained private/source data.
- Per-project startup selection and per-user LaraServe login integration.
- Client preferences, official release checks, deliberate managed-data uninstall.
- Creation connection options, automatic npm installation before enabling Vite, live progress, automatic navigation to Configuration, and notification toasts.
- Project npm tasks and private phpMyAdmin button.
- Explicit requested-domain setup and HTTPS trust attempts, with honest permission failures.

## Validation

- Full race-enabled Go tests, vet, JavaScript syntax, and diff whitespace checks.
- Linux executable built; Windows amd64 and macOS arm64 cross-builds succeeded. Native login execution on Windows/macOS remains unverified.
- Disposable lifecycle test: login helper started selected PHP/queue app without migrations; removal stopped it, retained SQLite/source data, stayed removed across legacy import, and restored through Add app. Uninstall removed only the isolated private home and login entry, preserving source and executable.
- Real official React starter: Composer creation, npm install, private SQLite migration, saved connection options, and Vite startup succeeded.
- Private MySQL/phpMyAdmin: authenticated dashboard exposed a working per-run sign-in URL while running and removed it after stop.
- Browser checks: preferences save, startup switch, removal/uninstall confirmation guards, npm task output, starter dropdown/Vite selection, creation form and progress, appearance/responsive layout.

Trust tests use mocked system tools. No test installs a CA or changes the real hosts file. Startup tests use an isolated XDG directory. Temporary projects and private data are disposable.

## Recovery

Before-change tag: `restore/before-client-settings-2026-11-04`.
Completed tag: `restore/client-settings-local-ready`.
Bundle: `dist/checkpoints/client-settings.bundle`.
Screenshots: `dist/checkpoints/dashboard-{settings-light,create-light,task-progress,remove-confirmation}.png` plus dark/mobile checks.
