# laraserve

Serve any Laravel app over **HTTPS by default** on a machine with **no PHP, Composer or web server installed**.
A small Go executable downloads [FrankenPHP](https://frankenphp.dev) (PHP + Caddy),
Composer, MySQL and phpMyAdmin **on the client, only when needed**, and caches them
in `~/.laraserve/runtimes/`. Later runs can serve prepared apps offline.

```
laraserve new my-app                    # create a Laravel app (needs internet or Composer cache)
laraserve serve                          # https://localhost:8443
laraserve serve --domain testapp.test    # https://testapp.test (443 if allowed, else :8443)
laraserve serve my-app                   # detects MySQL settings and opens local phpMyAdmin
laraserve serve my-app --mysql           # force private MySQL for an app using another database
laraserve serve --http                   # http://localhost:8080  (no CA, no trust prompt, no host change)
laraserve serve ./other-app --copy       # run on a copy, original untouched
laraserve serve app.laraserve            # serve an offline bundle
laraserve pack                           # bundle app + vendor/ + built assets (run where you have internet)
laraserve trust | untrust                # install / remove the local CA
laraserve clean | uninstall              # reverse everything, driven by ~/.laraserve/manifest.json
laraserve doctor
```

## Build

Building does not download or embed server binaries. The executable contains only
LaraServe and its small PHP configuration.

```bash
make build                  # builds the small bin/laraserve client
make dist                   # cross-compiles small clients; no runtime downloads
make test
```

Requires Go 1.21+ on the build machine. Copy `bin/laraserve` to the client machine;
its first use downloads the required runtime over HTTPS and checks its SHA-256.
A failed or interrupted download is retried next time; completed tools are reused
without contacting the network. Internet is required for first use of each tool,
and for new application dependencies unless already cached.

For development, `LARASERVE_FRANKENPHP=/path/to/frankenphp` uses an existing binary.

## Create a Laravel app

```bash
laraserve new my-app
laraserve new ./apps/my-app --version "^12.0"
laraserve serve my-app
```

`new` uses the cached PHP and Composer to run `composer create-project laravel/laravel`.
The destination must be new or empty. Composer installs dependencies and runs the skeleton's setup scripts; no host PHP or Composer is required. Without `--version`, Composer chooses
a stable version compatible with the downloaded PHP. Use `--host-path` if installation needs
host tools such as Git or an archive extractor. Frontend assets still need your own Node tooling.
If installation fails, partial files are kept for inspection; use a fresh destination to retry.
Created applications belong to you and are not deleted by `clean` or `uninstall`.

## What it does

1. **Detect** – needs `artisan` and `laravel/framework` in `composer.json`.
2. **Download** PHP into `~/.laraserve/runtimes/` on first use (SHA-256 verified).
   Composer downloads only for `new` or missing dependencies; MySQL and phpMyAdmin
   download only when selected.
3. **Scrub the environment** – own `php.ini` (`PHPRC`, empty `PHP_INI_SCAN_DIR`), private `COMPOSER_HOME`,
   `XDG_*`, and a `PATH` containing only laraserve's binaries (`--host-path` to keep yours).
4. **Prepare** – `APP_KEY`, SQLite DB, storage and bootstrap caches live in `~/.laraserve/projects/<id>/` and are
   passed as environment variables (real env vars override `.env`), so **modern Laravel projects are not modified** (Laravel 8 uses its own storage directory; see below).
   Only `vendor/` is added if it was missing (needs network/Composer cache; otherwise use a bundle).
   MySQL/MariaDB settings start private MySQL by default (`--mysql=false` uses your external connection).
   Other configured connections are respected; otherwise SQLite is used and migrated.
   SQLite setup is marked ready only after migrations succeed; failures stop startup and are retried next time.
   Use `laraserve serve <path> --migrate` to apply migrations added after initial setup.
5. **TLS** – a local, **name-constrained** CA (may only sign `localhost`, `*.test`, loopback and private IPs;
   extend with `--allow-domain`) issues the leaf certificate.
6. **Serve** – FrankenPHP on `127.0.0.1` (`--host 0.0.0.0` for LAN).

## Project PHP and MySQL versions

```bash
laraserve serve ./my-app                         # detects project requirements
laraserve versions ./my-app                      # shows selected/available versions
laraserve serve ./my-app --php-version 8.3 --mysql-version 8.0
laraserve new my-app --php-version 8.3            # also writes .php-version
```

For explicit, shareable project pins, add `.laraserve.json`:

```json
{
  "php": "8.3",
  "mysql": "8.0"
}
```

PHP selection honors command flags, `.laraserve.json`, `.php-version`, then
Composer's `config.platform.php` (minor version). It checks root requirements,
locked dependencies including development packages, and PHP conflicts. Without
an explicit pin it chooses the lowest compatible supported PHP minor with the
current catalogued patch. A pin such as `8.3` selects the current 8.3 patch;
an exact patch pin must be available in the catalogue. Incompatible pins fail
before the app starts. Projects without a PHP requirement use PHP 8.5.

MySQL selection honors command flags, `.laraserve.json`, `.mysql-version`,
`.env` (`MYSQL_VERSION` / `DB_VERSION`), then MySQL image tags in Compose files.
MySQL version requirements cannot be inferred from `DB_CONNECTION=mysql` alone;
projects without another hint use 8.4. Multiple conflicting Compose tags require
an explicit pin. MariaDB version numbers are not MySQL version numbers.

Resolved versions are saved under the project's private LaraServe state in
`runtime-versions.json`, keeping subsequent runs stable while they remain
compatible with the project. Runtime downloads are cached separately by version
and checksum. PHP switching is supported on Linux amd64/arm64 for PHP 8.0–8.5.
PHP 7.4 is available on Linux amd64 with glibc 2.35+ (including Ubuntu 22.04/24.04).
macOS arm64 currently supports the default PHP 8.5 runtime. Available MySQL
families are 8.0 and 8.4 on Linux amd64. Run `versions` for exact catalogue patches.

PHP 8.5 uses FrankenPHP directly. PHP 8.2–8.4 use checksum-verified portable PHP
builds from `nunomaduro/static-php-builds`, with the same Caddy HTTPS frontend.
Those versions use PHP's development server on a private loopback port; this is
intended for local development. A matching Composer is included in those PHP
archives. The CLI and web requests both use the selected PHP version.

### Laravel 8 and older PHP

```bash
laraserve serve ./old-app --php-version 7.4
laraserve serve ./old-app --php-version 8.0
laraserve serve ./old-app --php-version 8.1
laraserve new old-app --version "^8.0" --php-version 7.4 --allow-insecure-dependencies
```

Available legacy patches are PHP 7.4.33, 8.0.30 and 8.1.34. Existing projects are
selected from their Composer requirements and lock file; `.php-version` and
`.laraserve.json` pins work the same way as for newer PHP. Both Artisan and web
requests use the selected version. Dependencies that exclude that PHP version
are still rejected.

PHP 8.0/8.1 use checksum-pinned static CLI builds mirrored by
`heera/php-binaries` on Linux amd64/arm64. PHP 7.4 uses matching CLI and extension
packages from Ondřej Surý's PHP repository for Ubuntu 22.04, extracted privately
on Linux amd64 (glibc 2.35+). Missing support libraries are downloaded from
checksum-pinned official Ubuntu packages into a shared `runtimes/php-support/`
cache. There is no global PHP install, no package install script execution, and
no compiler or Docker requirement. PHP 7.4 includes JSON, Phar, PDO/MySQL/SQLite,
cURL, mbstring, XML, intl, ZIP and other common Laravel extensions. Custom mirror
archives can still use `LARASERVE_PHP_RUNTIME_URL` and its checksum with the
portable `libexec/php` layout.

These PHP lines and Laravel 8 are legacy software for local compatibility work.
Composer may reject an unlocked Laravel 8 installation because its packages have
known security advisories. `--allow-insecure-dependencies` is an explicit option
on `new` and `serve` to allow advisory-affected dependencies for that installation
only; PHP platform checks and malware policies remain enabled. It does not edit
the project's Composer policy. Without the option, normal Composer policy applies.
It is generally unnecessary when installing an existing compatible lock file.

Laravel 8 does not honor the newer `LARAVEL_STORAGE_PATH` setting. Its logs,
sessions and compiled views use the project's own `storage/` directory; it must
be writable. Use `serve --copy` when the original folder must stay untouched.

Each MySQL release gets a separate data directory. Changing versions does not
upgrade, downgrade or import an existing database. Switching back reuses that
version's data. Databases created by older LaraServe builds remain accessible
when using their original MySQL 8.4.13 runtime.
