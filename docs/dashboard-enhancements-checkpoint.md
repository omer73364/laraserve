# Dashboard monitoring and app management checkpoint

This extends the project-workspace redesign after Phase 5. Phase 6 remains future work.

## Delivered

- Live workspace cards for running apps, CPU, memory, and development services, with short actual-sample trends and attention filtering.
- Custom themed dropdowns with keyboard controls across appearance, project switching, filters, settings, creation, logs, and runtime selection.
- A single, subtle search focus treatment and responsive table rows with Open app and Restart actions.
- A Laravel version column distinguishing a locked framework version from its declared constraint.
- Add app registration for existing Laravel folders, preserving prior registrations, launch options, source files, and private data.
- A local folder browser for both existing apps and new app destinations. Selecting a path creates no files.

## Verification

- Full local Go race suite, vet, JavaScript syntax, and whitespace checks passed. New tests cover registration/idempotence, preservation of older identities and source files, invalid apps, folder listing limits/hidden folders, framework metadata fallbacks, authentication, origin checks, and CSRF.
- Dashboard and project-tools tests cross-compiled for Windows amd64. Native Windows/macOS execution of the picker and controls is not claimed.
- Browser checks used an isolated home and disposable Laravel fixtures. A real PHP 8.4 app, queue worker, and scheduler ran using cached runtimes. Monitoring showed real CPU/RSS and two active workers; a table restart recovered, Open app served the fixture, and Stop shut down its owned processes.
- Verified keyboard dropdown selection/Escape, project filtering, project switching, and saving a dropdown setting through the existing revision-aware API. Verified search focus has no inner outline.
- Browsed a new-app destination without creating it; added an existing app through Browse; checked it stayed stopped with its masked environment secret unchanged. Registration unit tests verify no source changes or database creation.
- Desktop and 390 px mobile layouts, light/dark appearances, row actions, and the modal picker were checked without horizontal page overflow.
- Local previews: `dist/checkpoints/dashboard-enhancements-light.png`, `dashboard-enhancements-dark.png`, `dashboard-enhancements-dropdown.png`, `dashboard-enhancements-folders.png`, and `dashboard-enhancements-mobile.png`.
- Local client rebuilt as `0.1.0-phase5.1-ui.3`. No new runtime payload was downloaded for review. No workflow, public release, or push occurred.

## Restore

Before: `restore/before-dashboard-enhancements-2026-11-04`.
Tested local result: `restore/dashboard-enhancements-local-ready`.
Standalone source history: `dist/checkpoints/dashboard-enhancements.bundle`.

Restore points cover source only. Review processes and disposable private data were removed after verification; user databases and cached runtimes were preserved.
