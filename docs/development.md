# Development, releases, and restore points

## Phase-by-phase work

The roadmap lives in [ROADMAP.md](../ROADMAP.md). Each phase should have acceptance criteria, focused commits, recorded verification, and a restore point. Current work is on `codex/phase-1-foundation`.

The baseline before Phase 1 is the annotated tag `restore/before-phase-1-2026-11-02`. These are source-code restore points; databases and private runtime state are separate and are not stored in Git.

Inspect a checkpoint without changing the working tree:

```bash
git show restore/before-phase-1-2026-11-02
```

Create a separate checkout of the old code without discarding current work:

```bash
git worktree add --detach ../laraserve-before-phase1 restore/before-phase-1-2026-11-02
```

The end-of-phase local checkpoint is `restore/phase-1-local-ready`. A standalone Git bundle is saved at `dist/checkpoints/phase-1.bundle` after committing the phase. A bundle can restore source history in a new directory:

```bash
git clone -b codex/phase-1-foundation /absolute/path/to/phase-1.bundle laraserve-restored
```

Never reset or delete private database directories to roll back code.

## Local verification

```bash
make check
go test -race ./...
```

Integration tests are opt-in and use temporary state. Linux browser recovery can be tested without modifying the actual system or browser trust stores:

```bash
LARASERVE_TEST_TRUST_REPAIR=1 go test ./internal/trust -run TestRealBrowserTrustRecovery -v
```

See [Windows setup](windows.md) for native Windows tests. Cross-compilation checks that source builds for Windows; it does not execute Windows programs.

`docs/ci-workflow.example.yml` is an inactive template covering Ubuntu 22.04, Ubuntu 24.04, and Windows Server 2022. No automatic workflow is installed under `.github/workflows`, per the owner's request. Hosted CI runs and release publication are separate follow-up work.

## Local release artifacts

```bash
make dist
make release VERSION=0.1.0-phase1.2
```

`make dist` cross-compiles the clients without runtime payloads and generates `dist/SHA256SUMS` for the five raw executables. Linux, macOS, and Windows build outputs do not imply equal runtime validation across platforms; consult the support matrix.

`make release` additionally prepares `dist/releases/v<VERSION>/` containing ZIP archives and `SHA256SUMS`. Each archive includes its client executable, the MIT license, Go dependency license notices, and upstream license texts. Archive timestamps are fixed for reproducible packaging. Build from a clean tested commit when preparing public releases. Existing versioned release directories are never overwritten; choose another version for changed artifacts.

Verify locally:

```bash
cd dist/releases/v0.1.0-phase1.2
sha256sum -c SHA256SUMS
```

Checksums detect changed files; they are not release signatures. These commands do not upload files, publish a release, push Git changes, or run GitHub Actions.
