package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"
)

// EnsureMySQL downloads a private distribution only when MySQL is selected.
func EnsureMySQL(runtimesDir string) (string, error) {
	return EnsureMySQLVersion(runtimesDir, "8.4.13")
}

func MySQLVersions() []string { return []string{"8.0.46", "8.4.13"} }

func EnsureMySQLVersion(runtimesDir, version string) (string, error) {
	if goruntime.GOOS == "windows" {
		return "", fmt.Errorf("private MySQL currently supports Linux and macOS only")
	}
	if dir := os.Getenv("LARASERVE_MYSQL_DIR"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		if err := validateMySQL(abs); err != nil {
			return "", err
		}
		return abs, prepareMySQL(abs, version)
	}
	if (goruntime.GOOS != "linux" || goruntime.GOARCH != "amd64") && os.Getenv("LARASERVE_MYSQL_URL") == "" {
		return "", fmt.Errorf("automatic MySQL download is currently available for Linux amd64; set LARASERVE_MYSQL_DIR or LARASERVE_MYSQL_URL and LARASERVE_MYSQL_SHA256 for this platform")
	}
	sources := map[string]downloadSource{"8.0.46": {"https://cdn.mysql.com/Downloads/MySQL-8.0/mysql-8.0.46-linux-glibc2.17-x86_64-minimal.tar.xz", "e2777d490fcaadc5a4be06db89b6935c78909bd9c3382af859ab284af0c950b0"}, "8.4.13": {"https://cdn.mysql.com/Downloads/MySQL-8.4/mysql-8.4.13-linux-glibc2.28-x86_64-minimal.tar.xz", "5dfbb764f731df40ef9e4f5d69aed32bdac8ba1af20bd96b7d40a942d83d4ed2"}}
	selected, ok := sources[version]
	if !ok {
		return "", fmt.Errorf("no MySQL download for %s", version)
	}
	source, err := sourceOverride("MYSQL", selected)
	if err != nil {
		return "", err
	}
	dir, err := installCache(runtimesDir, cacheKey("mysql-"+version+"-"+goruntime.GOOS+"-"+goruntime.GOARCH, source), validateMySQL, func(dir string) error {
		archive := filepath.Join(dir, ".archive")
		if err := downloadFile(source, archive, "MySQL"); err != nil {
			return err
		}
		if err := extractDistribution(archive, dir); err != nil {
			return err
		}
		if err := os.Remove(archive); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return dir, prepareMySQL(dir, version)
}

// Repair cached installations too: support libraries may have been installed
// after the first download. Serialize repairs and publish each library atomically.
func prepareMySQL(dir, version string) error {
	unlock, err := lockDownload(dir + ".libraries.lock")
	if err != nil {
		return err
	}
	defer unlock()
	if err := mysqlLibraries(dir); err != nil {
		return err
	}
	if err := downloadMySQLLibraries(dir); err != nil {
		return err
	}
	return verifyMySQL(dir, version)
}

// MySQLEnv makes the private distribution's libraries available for all server
// phases, including version checks, initialization, and normal startup.
func MySQLEnv(dir string, env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "LD_LIBRARY_PATH=") {
			out = append(out, entry)
		}
	}
	if goruntime.GOOS == "linux" {
		out = append(out, "LD_LIBRARY_PATH="+filepath.Join(dir, "lib", "private")+":"+filepath.Join(dir, "lib"))
	}
	return out
}

func verifyMySQL(dir, want string) error {
	cmd := exec.Command(filepath.Join(dir, "bin", "mysqld"), "--version")
	cmd.Env = MySQLEnv(dir, os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("MySQL %s cannot start: %w: %s%s", want, err, out, mysqlLibraryHelp(string(out)))
	}
	m := regexp.MustCompile(`\bVer\s+(\d+\.\d+\.\d+)`).FindStringSubmatch(string(out))
	if len(m) != 2 || m[1] != want {
		return fmt.Errorf("MySQL runtime reports %q, project selected %s", string(out), want)
	}
	return nil
}

// Reuse local OS support libraries without installing a global database service.
// Ubuntu 24.04 renamed libaio's soname; MySQL still requests libaio.so.1.
func mysqlLibraries(dir string) error {
	triplet := "x86_64-linux-gnu"
	if goruntime.GOARCH == "arm64" {
		triplet = "aarch64-linux-gnu"
	}
	return mysqlLibrariesFrom(dir, []string{"/lib/" + triplet, "/usr/lib/" + triplet, "/usr/lib64", "/lib64", "/usr/lib", "/lib"})
}

func mysqlLibrariesFrom(dir string, search []string) error {
	if goruntime.GOOS != "linux" {
		return nil
	}
	for _, name := range []string{"libaio.so.1", "libnuma.so.1"} {
		target := filepath.Join(dir, "lib", "private", name)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		var candidates []string
		for _, base := range search {
			matches, _ := filepath.Glob(filepath.Join(base, name+"*"))
			candidates = append(candidates, matches...)
		}
		for _, src := range candidates {
			b, err := os.ReadFile(src)
			if err != nil {
				continue
			}
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err = writeMySQLLibrary(target, b); err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func validateMySQL(dir string) error {
	for _, name := range []string{"mysqld", "mysql"} {
		st, err := os.Stat(filepath.Join(dir, "bin", name))
		if err != nil {
			return fmt.Errorf("MySQL distribution needs bin/%s: %w", name, err)
		}
		if !st.Mode().IsRegular() || st.Mode()&0o111 == 0 {
			return fmt.Errorf("MySQL bin/%s is not executable", name)
		}
	}
	return nil
}

func writeMySQLLibrary(target string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(target), ".library-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), target)
}

func mysqlLibraryHelp(output string) string {
	missing := regexp.MustCompile(`error while loading shared libraries: ([^: \n]+)`).FindStringSubmatch(output)
	if len(missing) != 2 {
		return ""
	}
	library := missing[1]
	hint := "Install the OS package providing this library with your distribution's package manager."
	switch library {
	case "libaio.so.1":
		hint = "Install libaio1t64 (recent Ubuntu/Debian), libaio1 (older Ubuntu/Debian), or libaio (Fedora/RHEL/Arch). LaraServe handles the libaio.so.1t64 name privately; no system symlink is needed."
	case "libnuma.so.1":
		hint = "Install libnuma1 (Ubuntu/Debian), numactl-libs (Fedora/RHEL), or numactl (Arch)."
	case "libstdc++.so.6":
		hint = "Install libstdc++6 (Ubuntu/Debian), libstdc++ (Fedora/RHEL), or gcc-libs (Arch)."
	}
	return "\nMissing MySQL support library: " + library + ". " + hint + " Then rerun LaraServe; the cached runtime will be repaired without deleting database data. Use --mysql=false only if you already have an external database."
}
