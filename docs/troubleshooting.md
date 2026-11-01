# 🔧 Troubleshooting & limits

[← Back to LaraServe](../README.md)

## 🦊 Firefox shows a certificate warning

Fully close Firefox, reinstall local CA trust, then reopen the browser:

```bash
laraserve trust
```

Trust installation is retried even when LaraServe previously recorded the CA as installed. On Linux, existing Firefox NSS profiles are included; failed imports show their profile paths.

<details>
<summary>🧰 The certutil command is missing</summary>

Browser trust may need your distribution's NSS tools:

```bash
# Debian / Ubuntu
sudo apt install libnss3-tools

# Fedora
sudo dnf install nss-tools
```

Run `laraserve trust` again after installing the tool. These browser tools are separate from the MySQL support libraries LaraServe downloads automatically.

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

## 🧩 MySQL reports a missing shared library

On Linux amd64, LaraServe automatically downloads missing **libaio** and **libnuma** into private runtime storage. It handles Ubuntu 24's renamed library and repairs cached runtimes on later starts. No manual system symlink is required.

Reconnect to the internet and retry if a dependency download was interrupted. An error for another library names the missing file and suggests an OS package; compatible glibc and C++ runtime libraries must come from the base system.

Do not delete database data to fix a library error. See [runtime compatibility](../README.md#-project-versions) and [download configuration](runtime.md#-downloads--mirrors).

## 🛠️ A table is missing or migrations failed

```bash
laraserve serve ./my-app --migrate
```

Initial SQLite/private MySQL setup is marked ready only after migrations succeed. A failure stops startup and retries on the next run. Fix the reported application migration error before restarting.

`--migrate` applies migrations added after the first successful setup. For a project's external database, use its normal migration workflow; LaraServe's managed connection is separate.

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
| 🪟 | Windows | Automatic runtime installation is not implemented. |
| 🗄️ | MySQL | Automatic distribution and support-library downloads are Linux amd64 only. |
| 🧶 | Node | Node installation and frontend builds are not managed. Build assets before serving or packing an app that needs them. |
| 🧱 | Sandbox | `--sandbox` is not implemented. The scrubbed environment is not an OS sandbox. |
| 🗂️ | Laravel 8 | Uses its own writable `storage/` directory; use `--copy` to preserve an untouched original. |
| 📡 | LAN HTTPS | Each client device needs to trust the local CA. |
| 🧳 | Offline bundles | Application bundles do not contain runtime downloads or managed database data. |
