# Native Windows support

LaraServe has an experimental native Windows amd64 implementation. WSL and Docker are not required. The implementation cross-builds from Linux, but Windows execution must be verified by the included Windows CI workflow before treating it as tested support.

## Setup

Use Windows 10/11 x64 with the [Microsoft Visual C++ 2015–2022 x64 runtime](https://aka.ms/vs/17/release/vc_redist.x64.exe) installed. LaraServe does not install this system prerequisite automatically. A missing DLL or `0xc0000135` error usually indicates that prerequisite is missing; install it from Microsoft and restart LaraServe.

Build from the repository with Go:

```powershell
go build -buildvcs=false -o bin/laraserve.exe .
.\bin\laraserve.exe serve C:\Projects\my-app
```

The distribution build script also produces `dist/laraserve-windows-amd64.exe`. Adding build support does not publish a GitHub release automatically.

For a custom development hostname without changing the hosts file:

```powershell
.\bin\laraserve.exe serve C:\Projects\my-app --domain my-app.localhost
```

Start normally and approve the current-user CA trust setup when prompted. Custom `.test` hosts-file entries require an Administrator terminal; LaraServe can fall back to `.localhost` if hosts-file access is unavailable. Firefox settings or organizational browser policies may affect whether it accepts Windows certificate-store roots.

Plain HTTP skips certificate setup:

```powershell
.\bin\laraserve.exe serve C:\Projects\my-app --http
```

## Included capabilities

- Project-aware native PHP 7.4 and 8.0–8.5 downloads from the PHP project.
- Caddy HTTP/HTTPS proxy, private PHP configuration, and a downloaded CA bundle for Composer/cURL HTTPS.
- Composer installs, native `@php` script execution, and new-app creation.
- Managed MySQL 8.0/8.4, with a generated root password and database-scoped application credentials.
- phpMyAdmin configured for the managed MySQL loopback connection.
- Pack, extraction, project copies, and private state under `%USERPROFILE%\.laraserve` or `LARASERVE_HOME`.
- OS file locks for concurrent runtime downloads and Windows process termination handling.

Migrations remain explicitly opt-in. Use `--migrate` only when you intend to apply schema changes. New database names follow the app folder or bundle name; existing database assignments are preserved.

## Platform differences and limits

- MySQL uses an ephemeral port bound to `127.0.0.1`, rather than a Unix socket. It is not bound to the LAN.
- Stop web/PHP processes by terminating them on cancellation. MySQL first receives an authenticated shutdown request so it can flush data.
- Runtime mirror overrides must provide Windows ZIP distributions: PHP and Caddy ZIPs have flat roots, while MySQL has a single enclosing folder. Explicit mirror SHA-256 values remain required.
- Windows uses `LARASERVE_PHP_RUNTIME_URL` / `LARASERVE_PHP_RUNTIME_SHA256` for PHP and `LARASERVE_CADDY_URL` / `LARASERVE_CADDY_SHA256` for Caddy. `LARASERVE_FRANKENPHP` is a Unix runtime override and is not used on Windows.
- The PHP CA bundle supports `LARASERVE_CA_BUNDLE_URL` / `LARASERVE_CA_BUNDLE_SHA256` overrides.
- Native Windows ARM64 and x86 runtime downloads are not implemented.
- Node/Vite management and additional database engines remain roadmap items.
- PHP 7.4/8.0/8.1 are legacy versions; available official Windows builds do not imply ongoing upstream security support.

## Verification

`.github/workflows/test.yml` runs unit tests and a native integration job on Windows Server 2022. The integration test downloads official runtimes into temporary storage, creates a Laravel project in a path containing spaces, checks Composer `@php` hooks, verifies a fresh database has no tables, explicitly migrates it, requests Laravel over verified HTTPS, and restarts MySQL. It does not edit hosts or install certificates into the machine trust store.

Run that integration test on a Windows machine with the prerequisite installed:

```powershell
$env:LARASERVE_TEST_WINDOWS = '1'
go test ./integration -v -timeout 20m
```

Linux can also verify the actual Windows PHP ZIP layouts and publisher metadata without executing Windows binaries:

```bash
LARASERVE_TEST_WINDOWS_ARCHIVES=1 go test ./internal/runtime -run TestLiveWindowsDistributions -v
```
