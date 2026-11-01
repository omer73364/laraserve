# ⚙️ Runtime & configuration

[← Back to LaraServe](../README.md)

## 🎛️ Version selection

Explicit PHP pins are read in this order:

1. `--php-version`
2. `.laraserve.json` → `php`
3. `.php-version`
4. Composer's `config.platform.php`, using its minor version

Every selection is checked against root PHP requirements, locked dependencies—including development packages—and PHP conflicts. Without a pin, LaraServe selects the lowest compatible supported minor and its catalogued patch. Without PHP requirements, the default is PHP 8.5.

MySQL version hints are read in this order:

1. `--mysql-version`
2. `.laraserve.json` → `mysql`
3. `.mysql-version`
4. `.env` → `MYSQL_VERSION` or `DB_VERSION`
5. MySQL image tags in standard Compose files

`DB_CONNECTION=mysql` identifies an engine, not a version. Without another hint, the default is MySQL 8.4. Conflicting Compose tags need an explicit pin. MariaDB version numbers are not treated as MySQL versions.

Resolved versions are saved in each project's private `runtime-versions.json`. Compatible selections are reused. Exact patch pins must be available in the current catalogue; LaraServe does not silently substitute another patch or upgrade tools on every run.

```bash
laraserve versions ./my-app
laraserve serve ./my-app --php-version 8.3 --mysql-version 8.0
laraserve new my-app --php-version 8.3
```

An explicit `new --php-version` also writes `.php-version` into the created app.

## 🐘 Runtime sources

| | Runtime | Source & serving model |
|---|---|---|
| ⚡ | PHP 8.5 | [FrankenPHP](https://frankenphp.dev), serving PHP directly with Caddy. |
| 🧩 | PHP 8.2–8.4 | Portable CLI builds from [nunomaduro/static-php-builds](https://github.com/nunomaduro/static-php-builds), with Caddy proxying to a private PHP development server. These archives include Composer. |
| 🕰️ | PHP 8.0 / 8.1 | Static CLI builds mirrored by [heera/php-binaries](https://github.com/heera/php-binaries), using the same private backend model. |
| 📦 | PHP 7.4 | Matching CLI and extension packages from [Ondřej Surý's PHP repository](https://launchpad.net/~ondrej/+archive/ubuntu/php), built for Ubuntu 22.04 and extracted privately. Missing supported libraries come from pinned official Ubuntu packages. |

Both Artisan and web requests use the selected PHP version. Native PHP backends are intended for local development. The current legacy patches are 7.4.33, 8.0.30, and 8.1.34; run `versions` for the full platform-specific catalogue.

PHP 7.4 includes JSON, Phar, PDO/MySQL/SQLite, cURL, mbstring, XML, intl, ZIP, and other common Laravel extensions. Its automatic setup requires Linux amd64 with glibc 2.35+.

## 🧰 Serve flags

Flags may follow the project path: `laraserve serve ./app --http`.

| | Flag | Behavior |
|---|---|---|
| 🌐 | `--domain NAME` | Use a development domain; defaults to `localhost`. |
| 🔓 | `--http` | Use HTTP; the default port is 8080. |
| 🔌 | `--port N` | Set the port. HTTPS defaults to 8443; custom domains try 443 first. |
| 📡 | `--host ADDR` | Bind address; defaults to `127.0.0.1`. `0.0.0.0` exposes the app to the LAN. |
| 🪪 | `--allow-domain SUFFIX` | Extend the local CA's permitted names; accepts comma-separated suffixes. |
| 🔐 | `--no-trust` | Skip installing the local CA in trust stores. |
| 🏠 | `--no-hosts` | Skip hosts-file changes and use the `.localhost` fallback when needed. |
| 🗂️ | `--copy` | Serve a workspace copy of the app. |
| 🛠️ | `--migrate` | Apply pending SQLite or private MySQL migrations. |
| 🐘 | `--php-version V` | Select a compatible PHP version. |
| 🎚️ | `--mysql-version V` | Select the private MySQL release when managed MySQL is used. |
| 🗄️ | `--mysql` | Force private MySQL. Standard MySQL/MariaDB settings already enable it automatically. |
| 🔗 | `--mysql=false` | Keep the project's external database connection. |
| 🧭 | `--phpmyadmin=false` | Disable the local manager for managed MySQL. |
| 🧱 | `--host-path` | Retain host tools on `PATH`; the default uses LaraServe's private tools. |
| 🕰️ | `--allow-insecure-dependencies` | Explicitly allow advisory-affected Composer dependencies during installation. It does not bypass PHP platform checks or malware policy. |
| 📋 | `--verbose` | Stream server logs. |
| ✅ | `--yes`, `-y` | Answer yes to prompts. |
| 🚧 | `--sandbox` | Reserved; currently returns an unimplemented-feature error. |

`new` accepts `--version`, `--php-version`, `--host-path`, and `--allow-insecure-dependencies`. `pack` accepts `-o` and `--include-env`. `uninstall` accepts `--yes` / `-y`.

## 🗃️ Private state

The default home is `~/.laraserve/`; set `LARASERVE_HOME` to choose another location.

```text
~/.laraserve/
├── runtimes/        # verified tools and support-library caches
├── projects/        # app keys, SQLite, caches, and resolved versions
├── databases/       # persistent, version-specific managed MySQL data
├── workspaces/      # copied apps and extracted bundles
├── ca/              # local certificate authority
└── manifest.json    # recorded managed changes
```

LaraServe detects an app through `artisan` and `laravel/framework` in `composer.json`. It provides its own PHP configuration, private Composer/XDG directories, and a scrubbed `PATH`. `--host-path` keeps host tools available.

Modern Laravel apps receive private storage and cache paths through environment variables. Missing `APP_KEY` and SQLite setup are prepared in private state. Missing dependencies add `vendor/` to the project. Laravel 8 uses its own writable `storage/`; use `--copy` for an untouched original.

Initial SQLite/private MySQL migrations must succeed before setup is marked ready. Failures stop startup and are retried. Use `--migrate` after adding migrations later.

## 🗄️ Database behavior

Database detection reads `.env` (`DB_CONNECTION` / `DB_URL`) and standard defaults in `config/database.php`, without booting PHP. Dynamic custom configuration needs an explicit flag.

Managed MySQL injects a private connection without editing `.env`. It never imports an existing external or SQLite database. The runtime needs `pdo_mysql`; phpMyAdmin also needs `mysqli` and `mbstring`. An unexpected database exit stops the app and reports its log path.

Every MySQL release has separate data. Switching versions does not upgrade, downgrade, or import databases. Data from older LaraServe builds remains accessible with its original MySQL 8.4.13 runtime.

phpMyAdmin normally uses `https://localhost:8444` or `http://localhost:8081`; follow the full printed URL. It runs on loopback with a fresh per-run sign-in token, database-scoped credentials, and temporary configuration/sessions removed on normal shutdown.

## 📥 Downloads & mirrors

Tools download only when needed. Completed, verified installations are reused without network requests. Incomplete downloads retry on the next run. Runtime caches and database data stay separate.

Missing MySQL `libaio` / `libnuma` libraries are reused from the OS or downloaded privately on Linux amd64. Ubuntu's `libaio1t64` naming is handled locally. PHP 7.4 similarly resolves supported missing libraries from pinned packages. No package installation scripts run. Dependency caches retain copyright notices and can be reused offline across runtime installations.

<details>
<summary>📂 Use an existing distribution</summary>

| | Variable | Expected value |
|---|---|---|
| ⚡ | `LARASERVE_FRANKENPHP` | Path to an existing FrankenPHP executable. |
| 🗄️ | `LARASERVE_MYSQL_DIR` | An unpacked MySQL distribution matching the selected exact version. |
| 🧭 | `LARASERVE_PHPMYADMIN_DIR` | An unpacked phpMyAdmin distribution including `vendor/`. |

Automatic MySQL downloads are Linux amd64 only. Other supported Unix environments can provide a matching distribution themselves.

</details>

<details>
<summary>🌍 Configure a download mirror</summary>

Set a URL and its matching SHA-256 value:

```bash
export LARASERVE_MYSQL_URL="https://mirror.example.com/mysql.tar.xz"
export LARASERVE_MYSQL_SHA256="<sha256-of-the-archive>"
```

Replace the example URL and checksum with your own mirror's values.

| | Tool | URL variable | Checksum variable |
|---|---|---|---|
| ⚡ | FrankenPHP frontend | `LARASERVE_PHP_URL` | `LARASERVE_PHP_SHA256` |
| 🐘 | Selected native PHP | `LARASERVE_PHP_RUNTIME_URL` | `LARASERVE_PHP_RUNTIME_SHA256` |
| 📦 | Composer | `LARASERVE_COMPOSER_URL` | `LARASERVE_COMPOSER_SHA256` |
| 🗄️ | MySQL | `LARASERVE_MYSQL_URL` | `LARASERVE_MYSQL_SHA256` |
| 🧭 | phpMyAdmin | `LARASERVE_PHPMYADMIN_URL` | `LARASERVE_PHPMYADMIN_SHA256` |
| 🧩 | MySQL libaio | `LARASERVE_LIBAIO_URL` | `LARASERVE_LIBAIO_SHA256` |
| 🧱 | MySQL libnuma | `LARASERVE_LIBNUMA_URL` | `LARASERVE_LIBNUMA_SHA256` |

Public mirrors require HTTPS; localhost mirrors may use HTTP. MySQL/phpMyAdmin archives have one enclosing directory. Native PHP archives use `libexec/php` without an enclosing directory and must report the selected exact version.

The extractor supports gzip, xz, and Zstandard tar data. Support-library mirrors use matching amd64 `.deb` packages containing `usr/lib/x86_64-linux-gnu/`.

</details>

## 🔒 Host integration

The local CA is constrained to local development names and addresses: localhost, `.test`, loopback, and private IP ranges by default. `--allow-domain` extends its permitted suffixes.

OS/browser trust, managed hosts entries, and Linux low-port capabilities require prompts. Declining hosts changes uses a `.localhost` fallback where needed. Choosing plain HTTP does not require CA trust; a custom domain can still require name resolution.

LAN clients must trust the local CA themselves. phpMyAdmin remains loopback-only even when the Laravel app binds to `0.0.0.0`.
