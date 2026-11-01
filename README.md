<div align="center">

<img src="website/dist/favicon.svg" width="64" height="64" alt="LaraServe logo">

# ⚡ LaraServe

**Less setup. More Laravel.**

Run Laravel locally with project-aware PHP, HTTPS, and a private database.<br>
No global PHP, Composer, MySQL, or Docker installation required.

![Go 1.21+](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat-square&logo=go&logoColor=white)
![Local HTTPS](https://img.shields.io/badge/HTTPS-local_by_default-C4F278?style=flat-square)
![Tested on Linux](https://img.shields.io/badge/Linux-tested-FCC624?style=flat-square&logo=linux&logoColor=black)

[Quick start](#-quick-start) · [Features](#-features) · [Versions](#-project-versions) · [Documentation](#-documentation)

</div>

---

## 🚀 Quick start

From the LaraServe source directory, build the client with **Go 1.21+**:

```bash
make build
./bin/laraserve serve /path/to/your-laravel-app
```

Open the URL printed in the terminal—normally **https://localhost:8443**. Follow the prompts to trust the local certificate authority.

Copy `bin/laraserve` to a directory on your `PATH` to use the shorter commands below. Go is needed only to build LaraServe, not to run the resulting executable.

```bash
# Create a new Laravel app
laraserve new my-app

# Serve it locally
laraserve serve my-app

# Or use HTTP without certificate setup
laraserve serve my-app --http
```

> 💡 The client contains configuration and download catalogues, not embedded server binaries. First use downloads the tools it needs and verifies their SHA-256 checksums. Prepared apps can run offline once their runtimes and dependencies are cached.

**Frontend assets need your own Node tooling.** LaraServe does not install Node or run your asset build.

---

## ✨ Features

| | Feature | What it does |
|---|---|---|
| 🐘 | **Project-aware PHP** | Reads Composer requirements and locked dependencies; supports explicit version pins. |
| 🔒 | **HTTPS by default** | Serves through Caddy with a constrained local certificate authority. |
| 🗄️ | **Private MySQL** | Detects MySQL settings and starts a separate, persistent database for the app. |
| 🧭 | **Local phpMyAdmin** | Prints a temporary sign-in link for browsing tables and running queries. |
| 📦 | **Downloads on demand** | Caches verified runtimes and missing supported libraries on the client. |
| 🌱 | **App creation** | Creates Laravel projects through Composer without a global PHP installation. |
| 🧳 | **App bundles** | Packages dependencies and built assets into a `.laraserve` file. |
| 🧹 | **Recorded cleanup** | Tracks managed state, hosts entries, and certificate trust for later removal. |

---

## 🧰 Everyday commands

| | Command | Purpose |
|---|---|---|
| ▶️ | `laraserve serve [path]` | Serve the current project or another app. |
| 🆕 | `laraserve new <path>` | Create an app in a new or empty directory. |
| 🎛️ | `laraserve versions [path]` | Show selected and available runtime versions without downloading them. |
| 📦 | `laraserve pack [path] -o app.laraserve` | Bundle an app with its dependencies and existing built assets. |
| 🔐 | `laraserve trust` / `laraserve untrust` | Install or remove local CA trust. |
| 🩺 | `laraserve doctor` | Inspect local setup. |
| 🧽 | `laraserve clean [path]` | Remove recorded project state and managed project artifacts. |
| 🗑️ | `laraserve uninstall` | Remove LaraServe-managed state and data after confirmation. |
| 🏷️ | `laraserve version` | Print the client version. |

```bash
# A development domain; tries port 443, then falls back to 8443
laraserve serve my-app --domain my-app.test

# Work on a copy
laraserve serve ./other-app --copy

# Apply migrations added since the initial setup
laraserve serve my-app --migrate

# Choose a Laravel skeleton and PHP version
laraserve new my-app --version "^12.0" --php-version 8.3
```

<details>
<summary>🌱 How app creation works</summary>

`new` runs `composer create-project laravel/laravel` with the selected PHP runtime. Composer installs dependencies and runs the skeleton's setup scripts. It does not run the interactive Laravel installer wizard.

Without `--version`, Composer selects a stable skeleton compatible with that PHP version. Use `--host-path` when installation needs host tools such as Git or an archive extractor.

If creation fails, partial files stay available for inspection. Choose a fresh destination to retry. Created app folders are yours; `clean` and `uninstall` do not delete the app itself.

</details>

---

## 🎛️ Project versions

LaraServe detects project requirements automatically. For shareable pins, commit a `.laraserve.json` file:

```json
{
  "php": "8.3",
  "mysql": "8.0"
}
```

```bash
laraserve versions ./my-app
laraserve serve ./my-app --php-version 8.3 --mysql-version 8.0
```

PHP pins must satisfy the project's Composer requirements, locked dependencies, and conflicts. A minor pin selects a catalogued patch; exact patch pins must exist in the catalogue. Compatible resolved versions are remembered in private project state.

| | Platform | Automatic PHP runtimes | Automatic MySQL runtimes |
|---|---|---|---|
| 🐧 | Linux amd64 | PHP 7.4 and 8.0–8.5 | MySQL 8.0 and 8.4 |
| 🦾 | Linux arm64 | PHP 8.0–8.5 | Not provided |
| 🍎 | macOS arm64 | Default PHP 8.5 | Not provided |
| 🪟 | Windows | Not implemented | Not implemented |

PHP 7.4 requires **glibc 2.35+**, including Ubuntu 22.04/24.04. MySQL 8.0 requires glibc 2.17+; MySQL 8.4 requires glibc 2.28+. Linux is tested; macOS runtime paths remain untested. A cross-compiled client does not guarantee automatic runtime availability.

<details>
<summary>🕰️ Laravel 8 and older PHP</summary>

Laravel 8.83.29 has been exercised with PHP **7.4.33**, **8.0.30**, and **8.1.34**, including web requests and database migrations. An existing app's locked dependencies still determine which versions it can use.

```bash
laraserve serve ./old-app --php-version 7.4
laraserve serve ./old-app --php-version 8.0
laraserve serve ./old-app --php-version 8.1
```

Composer can block a fresh Laravel 8 install because its dependencies have known security advisories. For deliberate legacy compatibility work, an explicit opt-in is available:

```bash
laraserve new old-app --version "^8.0" --php-version 7.4 --allow-insecure-dependencies
```

The option also works on `serve` when it installs missing dependencies. It allows advisory-affected packages for that Composer invocation; PHP platform checks and malware policies remain enabled. It does not edit the project's policy.

Laravel 8 uses its own writable `storage/` directory rather than honoring `LARAVEL_STORAGE_PATH`. Use `--copy` when the original folder must stay untouched.

</details>

[Version precedence, runtime sources, and overrides →](docs/runtime.md)

---

## 🗄️ MySQL & phpMyAdmin

When an app selects `mysql` or `mariadb` in standard project settings, LaraServe starts its own **private MySQL database**. Each app gets generated credentials and persistent data. The database starts and stops with the web server and uses a private Unix socket, with TCP and MySQL X disabled.

```bash
laraserve serve ./my-app                     # detect the database setting
laraserve serve ./my-app --mysql             # force private MySQL
laraserve serve ./my-app --mysql=false       # keep an external connection
laraserve serve ./my-app --phpmyadmin=false  # disable the database manager
```

Open the **complete phpMyAdmin link printed in the terminal**. It signs in as the app's database-scoped user and stays bound to loopback, even when the app is exposed to the LAN. Each run gets a fresh token; database passwords are not included in the URL.

> ⚠️ Automatic MySQL mode creates a separate local database. It does **not** connect to or import the database in your `.env`. Use `--mysql=false` for that existing connection. A `mariadb` setting selects managed MySQL; it does not install MariaDB.

Missing `libaio` and `libnuma` libraries are downloaded privately on Linux amd64, including support for Ubuntu 24. No manual `sudo apt install` or system symlink is needed for those libraries.

Each MySQL release has its own data directory. Changing versions does not migrate data; switching back reuses that release's database.

---

## 🧳 Offline apps

Install the app's dependencies and build its frontend assets before packing:

```bash
laraserve pack ./my-app -o my-app.laraserve
laraserve serve my-app.laraserve
```

A bundle contains the application, `vendor/`, and existing built assets. It does **not** contain runtime downloads or managed database data. The destination machine needs the selected tools cached before it can serve fully offline.

Environment files are excluded by default, except `.env.example`. `--include-env` includes them deliberately; treat the resulting bundle as sensitive. SQLite files, Git metadata, `node_modules`, and generated storage caches/logs are excluded.

---

## 🔐 Local HTTPS & cleanup

LaraServe binds the app to `127.0.0.1` by default. Host changes are prompted, recorded in its manifest, and reversible.

| | Change | When | Undo |
|---|---|---|---|
| 🪪 | Trust the local CA in OS/browser stores | First HTTPS setup | `laraserve untrust` |
| 🏠 | Add a managed hosts-file entry | A custom domain needs one | `laraserve clean <path>` |
| 🔌 | Grant Linux low-port capability to the server binary | Accepting port 443 setup | Removed with the managed runtime |

Use `--http` for the default localhost workflow without certificate or hosts setup. `--no-trust` and `--no-hosts` decline those changes individually. Without a terminal, prompts default to **no** unless `--yes` is supplied.

> ⚠️ `clean` preserves managed MySQL data but removes private project state, **including its SQLite database**, and removes `vendor/` if LaraServe installed it. `uninstall` also deletes managed MySQL data. Back up data you need before cleanup.

---

## 📚 Documentation

| | Guide | Contents |
|---|---|---|
| ⚙️ | [Runtime & configuration](docs/runtime.md) | Version selection, all serve flags, data layout, mirrors, and runtime sources. |
| 🔧 | [Troubleshooting & limits](docs/troubleshooting.md) | Firefox trust, missing libraries, migration failures, and platform gaps. |

## 🛠️ Development

```bash
make build  # build bin/laraserve
make test   # run the Go test suite
make dist   # cross-compile Linux and macOS clients
```

Builds do not download or embed runtime executables. Local website source lives in [`website/dist/`](website/dist/).

No license file is currently included in this repository.
