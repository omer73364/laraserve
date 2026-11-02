// Package trust installs/removes the local CA in the OS trust store.
// This is one of the only two changes laraserve makes to the host
// (the other is the hosts file) and both are recorded in the manifest.
package trust

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"laraserve/internal/certs"
	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
)

func run(sudo bool, name string, args ...string) error {
	if sudo && goruntime.GOOS != "windows" && os.Geteuid() != 0 {
		args = append([]string{name}, args...)
		name = "sudo"
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func have(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// Install adds the CA to the OS store (and to NSS databases on Linux).
func Install(ca *certs.CA) (*manifest.CAEntry, error) {
	e := &manifest.CAEntry{SHA256: ca.SHA256, SHA1: ca.SHA1}
	switch goruntime.GOOS {
	case "darwin":
		e.Method = "macos"
		home, _ := os.UserHomeDir()
		kc := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
		if err := run(false, "security", "add-trusted-cert", "-r", "trustRoot", "-k", kc, ca.PEMPath); err != nil {
			return nil, fmt.Errorf("security add-trusted-cert: %w", err)
		}
	case "windows":
		e.Method = "windows"
		if err := run(false, windowsCertutil(), "-user", "-addstore", "Root", ca.PEMPath); err != nil {
			return nil, fmt.Errorf("certutil: %w", err)
		}
	case "linux":
		e.Method = "linux"
		d, err := linuxDistro()
		if err != nil {
			ui.Warn("%v; skipping system store (browsers using NSS can still be trusted)", err)
		} else {
			dst := filepath.Join(d.dir, "laraserve-local-ca."+d.ext)
			if err := run(true, "cp", ca.PEMPath, dst); err != nil {
				return nil, err
			}
			if err := run(true, d.update[0], d.update[1:]...); err != nil {
				return nil, err
			}
			e.Location, e.Update = dst, d.update
		}
		e.NSSDBs = installNSS(ca.PEMPath)
	default:
		return nil, fmt.Errorf("unsupported OS %s", goruntime.GOOS)
	}
	return e, nil
}

// Remove reverses Install.
func Remove(e *manifest.CAEntry) error {
	var errs []error
	switch e.Method {
	case "macos":
		_ = run(false, "security", "remove-trusted-cert", "-d", "-c", certs.CAName) // best effort
		errs = append(errs, run(false, "security", "delete-certificate", "-Z", e.SHA1))
	case "windows":
		errs = append(errs, run(false, windowsCertutil(), "-user", "-delstore", "Root", e.SHA1))
	case "linux":
		if e.Location != "" {
			errs = append(errs, run(true, "rm", "-f", e.Location))
			if len(e.Update) > 0 {
				errs = append(errs, run(true, e.Update[0], e.Update[1:]...))
			}
		}
		if len(e.NSSDBs) > 0 {
			tool, err := nssTool()
			if err != nil {
				errs = append(errs, fmt.Errorf("browser certificate removal needs NSS tools: %w", err))
			} else {
				for _, db := range e.NSSDBs {
					if _, err := os.Stat(db); os.IsNotExist(err) {
						continue
					}
					cmd := exec.Command(tool, "-d", "sql:"+db, "-D", "-n", certs.CAName)
					cmd.Stdin = os.Stdin
					out, err := cmd.CombinedOutput()
					if err != nil && !strings.Contains(string(out), "SEC_ERROR_UNKNOWN_CERT") {
						errs = append(errs, fmt.Errorf("remove browser CA from %s: %w: %s", db, err, out))
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}

type distro struct {
	dir, ext string
	update   []string
}

func linuxDistro() (*distro, error) {
	switch {
	case have("update-ca-certificates"): // Debian, Ubuntu, Alpine, SUSE
		return &distro{"/usr/local/share/ca-certificates", "crt", []string{"update-ca-certificates"}}, nil
	case have("update-ca-trust"): // Fedora, RHEL, CentOS
		return &distro{"/etc/pki/ca-trust/source/anchors", "pem", []string{"update-ca-trust", "extract"}}, nil
	case have("trust"): // Arch
		return &distro{"/etc/ca-certificates/trust-source/anchors", "crt", []string{"trust", "extract-all"}}, nil
	}
	return nil, errors.New("no supported system CA tool found")
}

// installNSS imports the CA into Chrome/Chromium and Firefox databases
// (user-level, no root). Returns the databases that were modified.
func installNSS(pemPath string) []string {
	home, _ := os.UserHomeDir()
	var dbs []string
	for _, p := range []string{
		filepath.Join(home, ".pki", "nssdb"),
		filepath.Join(home, "snap", "chromium", "current", ".pki", "nssdb"),
	} {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			dbs = append(dbs, p)
		}
	}
	for _, g := range []string{
		filepath.Join(home, ".mozilla", "firefox", "*", "cert9.db"),
		filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox", "*", "cert9.db"),
		filepath.Join(home, ".var", "app", "org.mozilla.firefox", ".mozilla", "firefox", "*", "cert9.db"),
	} {
		m, _ := filepath.Glob(g)
		for _, f := range m {
			dbs = append(dbs, filepath.Dir(f))
		}
	}
	if len(dbs) == 0 {
		ui.Warn("No browser certificate databases found. Create a Firefox profile, then run `laraserve trust` again.")
		return nil
	}
	tool, err := nssTool()
	if err != nil {
		ui.Warn("Browser trust setup could not obtain NSS tools: %v. Retry `laraserve trust` when online, or install your distribution's libnss3-tools/nss-tools.", err)
		return nil
	}
	var done []string
	for _, db := range dbs {
		if err := run(false, tool, "-d", "sql:"+db, "-A", "-t", "C,,", "-n", certs.CAName, "-i", pemPath); err == nil {
			done = append(done, db)
		} else {
			ui.Warn("Browser CA import failed for %s: %v. Close the browser and retry `laraserve trust`.", db, err)
		}
	}
	if len(done) > 0 {
		ui.OK("Local CA imported into %d browser certificate database(s)", len(done))
	}
	return done
}

var ensureNSSRuntime = runtime.EnsureNSS

func nssTool() (string, error) {
	if tool, err := exec.LookPath("certutil"); err == nil {
		return tool, nil
	}
	home, err := paths.New()
	if err != nil {
		return "", err
	}
	ui.Info("Preparing private browser certificate tools")
	return ensureNSSRuntime(home.Runtimes())
}

func windowsCertutil() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "certutil.exe")
}
