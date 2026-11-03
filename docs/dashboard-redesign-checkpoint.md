# Dashboard layout and theme checkpoint

This replaces the earlier card dashboard with a new workspace layout after Phase 5. Roadmap Phase 6 remains future work.

## Delivered

- A searchable project directory with rows and inline lifecycle controls, replacing the project-card grid and large summary cards.
- Project-specific Overview, Configuration, Environment, Commands, and Logs pages, with a project switcher and browser navigation.
- Focused configuration categories for connection, runtimes/database, PHP, and optional development services, saved as one revision.
- Draft preservation within a project, discard protection when leaving, loading/save states, and protection against repeated form submissions.
- Separate app creation, runtime library, and latest-task Activity pages. Command shortcuts fill the form without running it. Task output can be hidden and reopened.
- Light, Dark, and System appearances using the existing README logo and lime branding; locally persisted appearance across dashboard ports.

## Verification

- Full local Go race suite, vet, JavaScript syntax, and diff whitespace checks passed. The existing opt-in real-runtime integration tests were not rerun for this frontend change.
- Browser review used four disposable registrations and an isolated `LARASERVE_HOME`. Verified project search/status filtering, project navigation, settings categories, preserved drafts between sections, cancel/discard navigation, Node pin and service saves, environment saves with the original masked secret retained, command shortcuts, project-scoped log selection/search, runtime and creation pages, and latest-task output.
- A real task on a stopped fixture project failed with the expected requirement to start the app. Its output, completed state, and disabled cancellation were visible in Activity. No runtime download or user database was involved.
- Light/dark appearances, reload persistence, deep links, and desktop/mobile layouts were checked. At 390 px the directory and service configuration had no horizontal page overflow.
- Local previews: `dist/checkpoints/dashboard-redesign-light.png`, `dashboard-redesign-settings.png`, `dashboard-redesign-dark.png`, and `dashboard-redesign-mobile.png`.
- Local client rebuilt as `0.1.0-phase5.1-ui.2`. No public release, GitHub workflow, or push was performed.

## Restore

Before: `restore/before-dashboard-redesign-2026-11-03`.
Tested local result: `restore/dashboard-redesign-local-ready`.
Standalone source history: `dist/checkpoints/dashboard-redesign.bundle`.

These restore source, not private project/runtime/database data. The disposable browser review workspace and processes were removed after verification.
