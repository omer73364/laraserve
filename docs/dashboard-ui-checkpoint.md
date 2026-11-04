# Dashboard UI checkpoint

The dashboard now follows the logo and charcoal/lime theme referenced in README.md. The bundled logo also serves as its favicon. This is a UI improvement after Phase 5; roadmap Phase 6 remains future work.

Changes include more readable text and spacing, clearer project/service state, a combined status/search filter, responsive settings and optional-service controls, keyboard focus styling, selected-tool accessibility state, and labeled log controls.

Verification:

- `go test -race ./...`, `go vet ./...`, JavaScript syntax, and diff whitespace checks passed locally.
- Browser checks used three disposable project registrations and an isolated `LARASERVE_HOME`. Checked stopped/running filters, saving and reopening a Node pin, the environment tab, selecting logs and literal log search.
- Desktop (1365 px) and mobile (390 px) layouts were checked; neither had horizontal page overflow. Previews are saved locally as `dist/checkpoints/dashboard-ui-brand.png` and `dist/checkpoints/dashboard-ui-mobile.png`.
- Backend lifecycle behavior and data handling are unchanged. No runtime was downloaded or user database touched during UI review. The opt-in real-runtime integration tests were not rerun for this appearance-only change.

Restore points: `restore/before-dashboard-ui-2026-11-03` and `restore/dashboard-ui-local-ready`. Standalone source backup: `dist/checkpoints/dashboard-ui.bundle`. No changes pushed or workflows activated.
