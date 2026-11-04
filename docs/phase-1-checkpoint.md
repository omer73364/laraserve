# Phase 1 local checkpoint

## Scope

Implement and verify the local foundation before beginning project supervision. This checkpoint does not activate hosted CI or publish releases.

## Delivered

- Normal Linux HTTPS startup retries previously approved browser trust, checks the certificate fingerprint and trust flags, and repairs new/reset profiles.
- Missing recorded system certificate anchors trigger repair; healthy anchors avoid another system installation.
- `--no-trust` skips certificate-store operations, even when local CA constraints change.
- Certificate replacement retains the previous tracking record if removing old trust fails.
- `doctor` inspects local state without downloading or executing runtimes.
- `doctor --report FILE` writes only version/platform information, counts, and controlled health messages; no paths, domains, credentials, environment values, config contents, raw errors, or logs are included.
- Broken ready-marked tool caches are repaired without touching persistent database data. NSS cache validation executes the helper to catch loader failures before accepting a warm cache.
- Tests cover browser import failures, retry, healthy imports, reset/new profiles, opt-out, support-report privacy, concurrent cache publication, and release integrity.
- MIT license added with owner approval.
- Local binary checksums and immutable versioned release archives are supported. Archives include client and dependency license notices.
- CI template includes Ubuntu 22.04, Ubuntu 24.04, and Windows Server 2022 and remains inactive.

## Acceptance and verification

- Focused trust, report, cache, and packaging tests passed.
- Real first-use private NSS download and temporary Firefox database recovery passed without system `certutil`, including a deleted certificate being reimported.
- Full Go test suite with race detection passed on Linux Mint 22 (Ubuntu 24.04 base).
- Static checks and Windows cross-compilation checks passed.
- Five local client builds and versioned release archives (`0.1.0-phase1.2`) are prepared, with SHA-256 and ZIP/license integrity verification.
- A fresh-home CLI smoke test produced valid private report JSON without downloading a runtime.

Fresh Ubuntu VM matrix execution, native Windows execution, and public release publication remain unverified or deferred. The inactive workflow template is preparation, not evidence of a hosted CI run.

## Restore points

- Before: `restore/before-phase-1-2026-11-02` (`b86c480`).
- Local checkpoint: `restore/phase-1-local-ready`.
- Development branch: `codex/phase-1-foundation`.
- Standalone source backup: `dist/checkpoints/phase-1.bundle`.

Use [development instructions](development.md) to inspect or restore source safely. Database schemas are not changed automatically.

## Next phase

Phase 2 begins with the project registry and shared service lifecycle core. Define stable identity, process ownership, and crash recovery before adding background commands or a dashboard. Retain the CLI's current foreground workflow throughout the transition.
