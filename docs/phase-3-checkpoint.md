# Phase 3 local checkpoint

## Delivered

- `laraserve dashboard [--port N]` with an embedded, responsive project UI and no frontend toolchain dependency.
- Loopback-only, exact Host/Origin protection, fragment bootstrap, HttpOnly strict session cookies, CSRF action protection, and restrictive content policy.
- Project state, versions, database selections, URLs, owner health, search, and independent start/stop/restart controls through the Phase 2 core.
- Bounded searchable Laravel, combined PHP/web server, managed MySQL, and startup log tails with path confinement and text-only rendering.
- Measured Linux process-tree RSS and interval CPU counters; unsupported platforms report unavailable instead of fabricated usage.
- Async operation feedback, duplicate-action rejection, and retention of focused action controls during polling.
- No automatic or replayed migrations. No remote hosting, public management binding, hosted workflows, or public release publication.

## Verification

- Unit/race tests cover authentication, Host/Origin and CSRF rejection, cookie properties, private control data exclusion, body restrictions, authenticated stopping, log search/bounds/symlink confinement, one-shot action stripping, and real process measurements.
- A real CLI dashboard controls a temporary capability-enabled FrankenPHP app: login, start/readiness, logs/search, resource metrics, stable restart with a new owner, stop, and project independence after dashboard exit. The saved fixture deliberately includes an obsolete migration flag and fails if Artisan is invoked.
- Browser review covers private-link login, project cards, log selection/search, a real start from a card, and the narrow responsive layout.
- Linux tests and static checks pass. Windows source/test cross-compilation passes; native Windows execution remains pending. Local archives are cross-built for all five existing targets.

See [dashboard behavior and limits](dashboard.md), including combined server/PHP logs, recent-tail search, shared-page RSS accounting, Linux-only counters, and readiness versus continuous application health.

## Restore points

- Before: `restore/before-phase-3-2026-11-03`.
- Local checkpoint: `restore/phase-3-local-ready`.
- Branch: `codex/phase-3-dashboard`.
- Standalone source backup: `dist/checkpoints/phase-3.bundle`.

These restore source code, not private application data. No changes have been pushed.

## Next phase

Phase 4: project settings and deliberate actions, beginning with validated configuration shared between terminal and dashboard. Keep environment secrets private and require explicit migration actions.
