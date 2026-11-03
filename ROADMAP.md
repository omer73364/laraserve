# LaraServe development roadmap

Status: Phase 4 project tools are ready for their tested local checkpoint. See [Phase 4](docs/phase-4-checkpoint.md) and earlier checkpoint documents for evidence, limits, and restore points.

Updated: 2026-11-03

## Direction

LaraServe manages each project's complete local development environment, with the same capabilities available through the terminal and a local web UI. Keep the client small and download runtimes and optional services on demand.

This roadmap combines the current development priorities with ideas from [Omar's conversation with Claude](https://claude.ai/share/eeca3033-d3a3-42f4-8331-c9c1b90579d6).

## Phase 1: Dependable foundation

- [x] Finish automatic dependency repair and recovery from incomplete browser trust setup during normal startup.
- [x] Make startup stages, port conflicts, and failures clear, with actionable diagnostics and a support report that excludes secrets.
- [ ] Add CI for builds, tests, and supported Linux environments. An inactive Ubuntu 22.04/24.04 and Windows template is prepared; activation and hosted execution remain deferred by owner request.
- [ ] Publish versioned, prebuilt releases with checksums and documented runtime sources. Local packaging and checksums are implemented; public publication remains deferred.
- [x] Choose and add a license with the owner's approval (MIT).

Completion target: a fresh supported Ubuntu installation can reliably start an app with minimal manual setup.

## Phase 2: Project and service management

- [x] Introduce a project registry with stable project identities.
- [x] Build a background supervisor with explicit service ownership, lifecycle management, and crash recovery (Linux verified; Windows cross-builds; macOS hard-crash cleanup deferred).
- [x] Provide project list, start, stop, restart, and status commands.
- [x] Persist domain and port assignments and prevent duplicate processes or conflicting starts.
- [x] Define a common service interface for web servers, databases, and later development services.

Completion target: several projects run together without duplicate processes or conflicts.

## Phase 3: Local dashboard

- [x] Expose the shared management core through a protected local API.
- [x] Start with a read-only project dashboard showing status, URLs, PHP versions, database versions, and logs.
- [x] Add start, stop, restart, and open-app controls.
- [x] Add searchable Laravel, combined PHP/web server, managed MySQL, and startup log tails.
- [x] Show owner/readiness health and measured CPU and memory use (Linux counters; other platforms report unavailable).

Completion target: manage existing projects comfortably from the browser.

## Phase 4: Project settings and actions

- [x] Configure domain, HTTPS, PHP version, database mode, and service options per project.
- [x] Provide common PHP settings as form fields, with a validated raw configuration editor where appropriate.
- [x] Run Artisan and Composer actions with live output and clear exit status.
- [x] Integrate the terminal Laravel creation wizard (SQLite), official starter packages, and built-in database choices.
- [x] Add a safe environment editor with masked secrets and comparison against `.env.example`.
- [x] Manage installed runtime versions, downloads, and unused cache removal.

Completion target: configure and operate projects through either interface.

## Phase 5: Complete Laravel workflow

- [ ] Download Node optionally and select a version from project requirements.
- [ ] Detect the project's package manager and manage Vite startup and output.
- [ ] Add optional Redis and Mailpit services.
- [ ] Manage queue workers and the scheduler with visible status and logs.
- [ ] Start and stop the project's selected services together.

Completion target: one project start can launch the services a typical Laravel application needs.

## Phase 6: Database efficiency and expansion

- [ ] Benchmark current database memory, CPU, and startup costs before selecting tuning defaults.
- [ ] Add optional shared MySQL instances for projects selecting the same exact version and compatible server configuration.
- [ ] Retain isolated mode and assign a separate database and scoped user to every project in shared mode.
- [ ] Make managed versus external database connections explicit.
- [ ] Add import, export, backup, restore, and deliberate reset workflows.
- [ ] Provide database administration links and connection details without exposing credentials unnecessarily.
- [ ] Add PostgreSQL, then a distinct MariaDB implementation, using the common service interface.

Completion target: flexible databases with predictable resource use and protected data.

Existing private databases must remain intact. Introduce shared mode as an option first; moving existing data requires an explicit migration workflow with backups. Changing the selected engine version must never silently reuse an incompatible data directory.

## Phase 7: Advanced tools and platform support

- [ ] Add PHP extension compatibility checks and supported extension configuration.
- [ ] Add Xdebug where runtime builds support it, with per-project settings and IDE guidance.
- [ ] Offer optional startup at login and per-project automatic startup.
- [ ] Add project templates and service presets.
- [ ] Validate the experimental native Windows amd64 implementation in Windows CI and on user machines.
- [ ] Document and test WSL workflows.
- [ ] Expand tested Linux architectures and macOS support before claiming full support.

Completion target: cover more development workflows with clearly documented platform capabilities.

## Later exploration

- MongoDB, including the required PHP extension and an appropriate administration workflow.
- Optional public tunnels for demos and webhooks, explicitly enabled by the user.
- Advanced monitoring and resource history.
- Idle service shutdown only after active connections, workers, scheduler activity, and reliable restart behavior are accounted for.
- Broader database tooling and expansion of native Windows support beyond amd64.

## Architecture and product rules

1. Share one management core between the CLI and web UI; avoid features that work only in one interface.
2. Bind the management dashboard to loopback, authenticate sessions, validate Host and Origin, and protect state-changing actions.
3. Keep shareable project options in `.laraserve.json`. Keep credentials, machine paths, and process state private.
4. Share services only when versions and server configuration are compatible. Keep isolation available.
5. Protect data during cleanup, version changes, migration, and restore operations. Initial migrations are allowed during new project creation. Existing projects, including newly provisioned databases for them, require an explicit migration action; ordinary start/restart never replay migrations.
6. Download tools and services on demand; do not embed large runtime distributions in the client.
7. Use measured resource consumption rather than estimates to choose defaults.
8. Add database engines and services incrementally, with verification for each supported platform.

## Working method

For each feature:

- Define expected behavior and acceptance criteria.
- Inspect the current implementation and preserve existing working flows.
- Implement in small, focused commits.
- Verify the real user workflow and relevant failure cases.
- Update documentation and platform limitations.

The phases describe sequencing, not release dates. Revisit priorities when implementation reveals dependencies or user needs change.

## Recommended next milestone

Build Phase 5 on the tested project tools: optional Node and package-manager selection, Vite, then additional development services. Hosted CI activation and public release publication remain separately deferred.
