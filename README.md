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
