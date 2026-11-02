# 🔧 Troubleshooting & limits

[← Back to LaraServe](../README.md)

## 🦊 Firefox shows a certificate warning

Fully close Firefox, start your app normally, then reopen the browser:

```bash
laraserve serve ./my-app
```

After you approve trust once, normal HTTPS startup on Linux verifies browser certificate fingerprints and trust flags, repairs failed imports, and discovers new or reset Firefox profiles. It only asks for initial approval when no trust approval is recorded. `--no-trust` skips these operations. `laraserve trust` remains an optional explicit reinstall command.

<details>
<summary>🧰 The certutil command is missing</summary>

On Linux amd64 with glibc 2.35+ (including Ubuntu 22.04/24.04), LaraServe
reuses an installed `certutil` or automatically downloads a private, verified
copy with matching NSS libraries. Close Firefox and start your app again:

```bash
laraserve serve ./my-app
```

First use needs internet; later imports and removals reuse the cache offline.
No `sudo apt install libnss3-tools` is needed for this private helper. OS-wide CA
trust can still request administrator privileges through the normal trust prompt.
If no browser profile exists yet, open Firefox once, close it, then retry.

On other platforms, or if automatic downloading is unavailable, install your
distribution's tool and retry:

```bash
# Debian / Ubuntu
sudo apt install libnss3-tools

# Fedora
sudo dnf install nss-tools
```

</details>

<details>
<summary>🪪 Import the CA into an undiscovered Firefox profile</summary>

Open **Settings → Privacy & Security → Certificates → View Certificates → Authorities → Import**.

Select `~/.laraserve/ca/ca.pem`, or `LARASERVE_HOME/ca/ca.pem` under a custom home directory, and enable trust for identifying websites. Import **ca.pem**, never **ca.key**.

On macOS, Firefox may require `security.enterprise_roots.enabled` to use system keychain trust. Other LAN devices need their own CA trust setup.

</details>

For a localhost session without certificate setup:

```bash
laraserve serve ./my-app --http
```

## 🔌 Port 443 is already in use

Another web server may already own HTTPS port 443. LaraServe leaves that process
running and chooses an alternate HTTPS port for a development domain when the
conflict is detected. You can also choose one explicitly:

```bash
laraserve serve ./my-app --domain example-app.test --port 8443
```

Open the exact URL printed in the startup summary, including its port. An explicit
occupied `--port` fails with an actionable error. A last-moment bind conflict is
reported without announcing a successful startup; the full server log stays in
the project's private `logs/server.log`.

Startup readiness checks a private, random response from that particular launch;
an unrelated service accepting TCP connections cannot trigger a ready message.
Caddy's own trust installation is disabled explicitly. LaraServe manages its
local CA through its normal prompts and `trust` command.

## 🧩 MySQL reports a missing shared library

On Linux amd64, LaraServe automatically downloads missing **libaio** and **libnuma** into private runtime storage. It handles Ubuntu 24's renamed library and repairs cached runtimes on later starts. No manual system symlink is required.

Reconnect to the internet and retry if a dependency download was interrupted. An error for another library names the missing file and suggests an OS package; compatible glibc and C++ runtime libraries must come from the base system.

Do not delete database data to fix a library error. See [runtime compatibility](../README.md#-project-versions) and [download configuration](runtime.md#-downloads--mirrors).

## 🛠️ A table is missing or migrations failed

```bash
laraserve serve ./my-app --migrate
```

LaraServe does not run migrations automatically. A fresh managed database is empty until you explicitly migrate it. A migration failure stops that invocation and is not retried during normal startup.

`--migrate` explicitly applies pending migrations, including the initial schema. For a project's external database, use its normal migration workflow; LaraServe's managed connection is separate.

## 🕰️ Composer blocks Laravel 8 dependencies

Fresh legacy installs may include packages with known security advisories. For deliberate local compatibility work, use `--allow-insecure-dependencies` on `new` or `serve`. It only affects that Composer invocation's advisory blocking; it does not ignore PHP compatibility or change project policy.

Existing lock files are still checked against the selected PHP version. An incompatible pin needs a different PHP version or an intentional dependency update.

If `new` fails, keep the partial files for inspection and choose a new or empty destination for another creation attempt.

## 🔒 MySQL reports a running.lock after a crash

First verify that no MySQL process is using that data directory. Only then remove its stale `running.lock` and retry. Initialization failures retain partial data for inspection rather than deleting it.

`clean` keeps managed MySQL data but deletes private project state, including SQLite. `uninstall` removes the entire managed home, including MySQL data, after confirmation. Back up data before removing it.

## 🚧 Current limits

| | Area | Limit |
|---|---|---|
| 🐧 | Linux | The tested runtime and host-integration platform. PHP 7.4 requires amd64 and glibc 2.35+. |
| 🍎 | macOS | Runtime and host-integration paths are untested; automatic PHP is limited to the default 8.5 runtime on arm64. |
| 🪟 | Windows amd64 | Native runtime implementation is experimental; see [Windows setup](windows.md). Windows execution awaits CI validation. |
| 🗄️ | MySQL | Automatic distributions support Linux/Windows amd64. Automatic Linux support-library downloads are Linux amd64 only. |
| 🧶 | Node | Node installation and frontend builds are not managed. Build assets before serving or packing an app that needs them. |
| 🧱 | Sandbox | `--sandbox` is not implemented. The scrubbed environment is not an OS sandbox. |
| 🗂️ | Laravel 8 | Uses its own writable `storage/` directory; use `--copy` to preserve an untouched original. |
| 📡 | LAN HTTPS | Each client device needs to trust the local CA. |
| 🧳 | Offline bundles | Application bundles do not contain runtime downloads or managed database data. |

## Private support report

`laraserve doctor` inspects current state without downloading tools, running project code, installing certificates, or changing database schemas. Save a report with:

```bash
laraserve doctor --report report.json
```

The JSON contains the LaraServe version, OS/architecture, tool and project counts, and controlled health-check messages. It excludes paths, domains, environment variables, credentials, raw errors, configuration contents, and logs. It refuses to overwrite an existing file. Send or attach a report only when you choose to share it.

## Background startup or stale ownership

Use `laraserve status <path-or-ID>` and the private startup log printed by `start`. For first-time administrator prompts, use foreground `serve` once; background workers have no interactive input.

A remembered busy port requires an explicit `--port` override. Duplicate owners are rejected; use `stop` or `restart` instead of deleting lock files. Known stale MySQL ownership is recovered after the previous owner exits; unknown legacy locks are preserved for verification. See [project lifecycle](projects.md).

## Dashboard asks for its private link

Open the complete link printed by `laraserve dashboard`, including its fragment. The browser removes that fragment after session setup. Restarting the command creates a new session, so an old browser tab may need the new link. Use the printed `127.0.0.1` address; aliases and forwarded Hosts are deliberately rejected. A port conflict can be resolved by omitting `--port` to select an available port.

Dashboard actions use saved reusable settings and cannot answer privileged or dependency-consent prompts. Complete initial setup through `serve`, then use the dashboard. A pending download can outlive a dashboard wait; check status and Startup logs before trying another start. See [the dashboard guide](dashboard.md) for log and resource-counter limits.
