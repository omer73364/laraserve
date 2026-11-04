# Flock review: ideas for LaraServe

Reviewed 2026-11-05. Public source snapshot: `flock-php/flock` commit `ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc`.

This is a focused source and interface review, not a runtime verification of Flock. No Flock code was copied, built, or executed. These proposals do not start another roadmap phase.

## Most useful improvements

| Priority | Idea | Proposed LaraServe behavior |
| --- | --- | --- |
| First | Explain runtime selection | Show actual PHP, Node and npm versions on project details, with “From .nvmrc”, “Project setting”, or “Automatic compatibility selection”. Explain constraints that excluded another version. |
| First | Separate a local override from a shared project pin | Offer “Use project version”, “Use on this machine”, and an explicit “Save version to project”. Never rewrite a shared pin merely because someone tries another version. |
| First | Runtime management by major | Show installed Node lines, patch version, bundled npm version, projects using each line, and explicit install/update controls. Keep LaraServe's conservative automatic selection and engine checks. |
| First | Diagnose a busy port | Show the requested port, listening application when discoverable, and suggested next action. Distinguish LaraServe's own service from another program. Never stop another program automatically. |
| Next | Simpler settings navigation | Use a persistent sidebar and grouped settings rows. Put connection, runtime and service information together on project details; move uncommon options into advanced sections. Keep LaraServe's light/dark theme, custom dropdowns, notifications and progress UI. |
| Next | Service health with useful context | Show last failure, restart count and readiness separately from process existence. Offer bounded retries for suitable services, with a visible retry state. Retain explicit failures for dependency/configuration errors. |
| Later | Project folders | Let users register a parent folder and preview discovered apps before adding them. Handle duplicate registrations, nested apps and domain collisions explicitly. |
| Later | Per-project developer tools | Consider mail inbox filtering and a request timeline for dumps, queries and logs. These are separate capabilities, not simple visual changes. |

## Source evidence

- [Node documentation](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/docs/node.md): installed major lines, default selection, project isolation and pins.
- [Version documentation](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/docs/project-versions.md): local isolation takes precedence over shared pins; the interface explains the source of a version.
- [Linux site pane](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/linux/src/panes/sites.rs): version-source controls, grouped settings and pin/override differences.
- [Node resolver](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/internal/node/node.go): walks ancestor folders for version files; resolves versions within major lines.
- [npm management](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/internal/node/npm.go): isolates global npm packages by Node major and checks npm's Node engine before proposing an npm update.
- [Process supervisor](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/internal/supervisor/supervisor.go): records restart counts and last failure; retries with exponential backoff capped at 30 seconds.
- [Upstream probe](https://github.com/flock-php/flock/blob/ddff58c2d6ab106cacfd7e4430f8a4a1a1cf0cbc/internal/daemon/proxyprobe.go): checks reachability and identifies local listening programs, caching observations for three seconds.
- [Website](https://flockphp.com): interactive app preview and project-folder, mail and debugging concepts. Website copy and repository documentation are not completely synchronized; source behavior takes precedence in this review.

## Differences to preserve

Flock's installer keeps one published patch release per major and can resolve an exact pin to another patch in the same major. LaraServe should preserve exact pins rather than silently changing them. A newest-major policy would also undo the compatibility changes requested after Node 26 problems.

Automatic service retries do not repair incompatible Vue adapters or npm peer dependency conflicts. Show dependency conflicts separately from runtime failures; do not automatically force installs or ignore peer requirements.

Flock is AGPL-3.0-only; LaraServe currently uses MIT. This review proposes independently implemented behavior and layout ideas, without importing source or assets.

## Suggested first delivery

A project “Runtime” card showing resolved PHP/Node/npm versions, selection reasons and an explicit Node override, followed by a clearer runtime library. This directly addresses the recent Node compatibility problems before expanding services or adding developer tools.

## Implemented comparison follow-up

The following changes were independently implemented in LaraServe after comparing the source above:

| Area | Flock implementation | LaraServe before | LaraServe now |
| --- | --- | --- | --- |
| Version provenance | Site details identify an isolated, pinned or default version | Resolver returned a version string only | Structured selection records pin source, root engine, build-tool constraints and reuse; project overview and progress expose the explanation |
| Exact pins | Installer and resolver can use another patch in the requested major | Exact project pins are enforced | Exact behavior retained and regression tested; missing exact releases report the originating file and pin |
| LTS alias | Unrecognized aliases fall back to default selection | `lts/*` became `latest`, bypassing conservative selection | `lts/*` follows compatible baseline selection, so it no longer opts into the newest major |
| Runtime library | Version rows show Node and npm versions | Opaque cache identifiers | Readable PHP/MySQL/Node versions; installed npm version read from bounded metadata on Unix and Windows layouts |
| npm dependency conflicts | Runtime selection and npm management are distinct | Added explicit conflict guidance in an earlier fix | Kept separate from runtime compatibility; no forced installation or automatic peer-check bypass |
| Process supervision | Automatic exponential retries with restart counts | Project lifecycle fails visibly when a managed child exits | Retained existing lifecycle policy; automatic retries would change ownership/failure behavior and need separate design and tests |

Node selection shown on the overview is explicitly the **last selected** version. A saved settings change does not imply a running process has switched versions. Existing projects acquire the richer explanation at their next frontend preparation or managed Node/npm command. Runtime inventory reads local metadata only; it does not launch binaries or contact registries.

### Daily workflow follow-up

The project overview now groups PHP, Node, database and Laravel into readable stack cards, with the Node selection explanation kept visible. Services are grouped into Frontend, Background tasks, and Data & mail. Ready services with HTTP(S) URLs get direct Open links; others link to configuration. Automatically detected services are distinguished from explicitly disabled services. A tool strip links to commands, environment, logs and runtime settings, and the application URL can be copied with a toast confirmation. These controls use existing LaraServe capabilities and do not imply independent service start/stop support.
