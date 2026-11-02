# LaraServe development roadmap

Status: proposed roadmap, saved for future development. No implementation is implied by this document.

Updated: 2026-11-02

## Direction

LaraServe manages each project's complete local development environment, with the same capabilities available through the terminal and a local web UI. Keep the client small and download runtimes and optional services on demand.

This roadmap combines the current development priorities with ideas from [Omar's conversation with Claude](https://claude.ai/share/eeca3033-d3a3-42f4-8331-c9c1b90579d6).

## Phase 1: Dependable foundation

- [ ] Finish automatic dependency repair and recovery from incomplete browser trust setup during normal startup.
- [ ] Make startup stages, port conflicts, and failures clear, with actionable diagnostics and a support report that excludes secrets.
- [ ] Add CI for builds, tests, and supported Linux environments.
- [ ] Publish versioned, prebuilt releases with checksums and documented runtime sources.
- [ ] Choose and add a license with the owner's approval.

Completion target: a fresh supported Ubuntu installation can reliably start an app with minimal manual setup.

## Phase 2: Project and service management

- [ ] Introduce a project registry with stable project identities.
- [ ] Build a background supervisor with explicit service ownership, lifecycle management, and crash recovery.
- [ ] Provide project list, start, stop, restart, and status commands.
- [ ] Persist domain and port assignments and prevent duplicate processes or conflicting starts.
- [ ] Define a common service interface for web servers, databases, and later development services.

Completion target: several projects run together without duplicate processes or conflicts.

## Phase 3: Local dashboard

- [ ] Expose the shared management core through a protected local API.
- [ ] Start with a read-only project dashboard showing status, URLs, PHP versions, database versions, and logs.
- [ ] Add start, stop, restart, and open-app controls.
- [ ] Add searchable Laravel, PHP, web server, and database logs.
- [ ] Show service health and measured CPU and memory use.

Completion target: manage existing projects comfortably from the browser.

## Phase 4: Project settings and actions

- [ ] Configure domain, HTTPS, PHP version, database mode, and service options per project.
- [ ] Provide common PHP settings as form fields, with a validated raw configuration editor where appropriate.
- [ ] Run Artisan and Composer actions with live output and clear exit status.
- [ ] Integrate Laravel's creation wizard, starter kit choices, and database choices.
- [ ] Add a safe environment editor with masked secrets and comparison against `.env.example`.
- [ ] Manage installed runtime versions, downloads, and unused cache removal.

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
5. Protect data during cleanup, version changes, migration, and restore operations. Never run schema migrations automatically; require an explicit user request, including for new databases.
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

Finish remaining startup reliability fixes and implement the project registry and service supervisor. This foundation supports the dashboard, shared databases, queues, Vite, and optional startup at login.
