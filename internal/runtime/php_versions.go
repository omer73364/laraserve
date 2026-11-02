package runtime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"laraserve/internal/process"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

//go:embed php_catalog.json
var phpCatalogJSON []byte

func PHPVersions() []string {
	if goruntime.GOOS == "windows" {
		return windowsPHPVersions()
	}
	if goruntime.GOOS != "linux" {
		return []string{"8.5.12"}
	}
	versions := []string{"8.0.30", "8.1.34", "8.2.35", "8.3.36", "8.4.27", "8.5.12"}
	if goruntime.GOARCH == "amd64" {
		versions = append([]string{"7.4.33"}, versions...)
	}
	return versions
}

func EnsurePHP(root, version string) (*Runtime, error) {
	if goruntime.GOOS == "windows" {
		return ensureWindowsPHP(root, version)
	}
	if version == "8.5.12" {
		rt, err := Ensure(root)
		if err != nil {
			return nil, err
		}
		rt.PHPVersion = version
		return rt, verifyPHP(rt, version)
	}
	if goruntime.GOOS != "linux" {
		return nil, fmt.Errorf("automatic PHP %s switching currently supports Linux; the default FrankenPHP runtime supports macOS arm64", version)
	}
	var catalog map[string]downloadSource
	if err := json.Unmarshal(phpCatalogJSON, &catalog); err != nil {
		return nil, err
	}
	source, ok := catalog[version+"/"+goruntime.GOARCH]
	if !ok {
		return nil, fmt.Errorf("no portable PHP %s for %s", version, goruntime.GOARCH)
	}
	packaged74 := strings.HasPrefix(version, "7.4.") && os.Getenv("LARASERVE_PHP_RUNTIME_URL") == ""
	source, err := sourceOverride("PHP_RUNTIME", source)
	if err != nil {
		return nil, err
	}
	keySource := source
	if packaged74 {
		keySource.SHA256 = fmt.Sprintf("%x", sha256.Sum256(php74PackagesJSON))
	}
	dir, err := installCache(root, cacheKey("php-"+version+"-"+goruntime.GOARCH, keySource), func(d string) error {
		if err := executable(filepath.Join(d, "libexec", "php")); err != nil {
			return err
		}
		_, err := os.Stat(filepath.Join(d, "php.ini"))
		return err
	}, func(d string) error {
		if packaged74 {
			return populatePHP74(d)
		}
		archive := filepath.Join(d, ".archive")
		if err := downloadFile(source, archive, "PHP "+version); err != nil {
			return err
		}
		if err := extractRuntimeArchive(archive, d, false); err != nil {
			return err
		}
		// Legacy bulk archives contain a single php executable. Normalize them
		// to the same layout used by newer runtime providers.
		if _, err := os.Stat(filepath.Join(d, "php")); err == nil {
			if err := os.MkdirAll(filepath.Join(d, "libexec"), 0755); err != nil {
				return err
			}
			if err := os.Rename(filepath.Join(d, "php"), filepath.Join(d, "libexec", "php")); err != nil {
				return err
			}
		}
		if err := os.Remove(archive); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(d, "php.ini"), phpINI, 0644)
	})
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(version, "7.4.") {
		if err := ensureLegacyPHPLibraries(dir); err != nil {
			return nil, err
		}
	}
	// FrankenPHP supplies Caddy/TLS; selected native PHP handles every app request.
	front, err := Ensure(root)
	if err != nil {
		return nil, err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
	native := filepath.Join(dir, "libexec", "php")
	if strings.HasPrefix(version, "7.4.") {
		wrapper := "#!/bin/sh\nexport LD_LIBRARY_PATH=" + quote(filepath.Join(dir, "lib", "private")) + "\nexport LARASERVE_PHP_EXTENSION_DIR=" + quote(filepath.Join(dir, "libexec", "extensions")) + "\nexec " + quote(native) + " \"$@\"\n"
		driverScript := filepath.Join(dir, "php-legacy")
		if err := writeLegacyPHPLauncher(driverScript, wrapper); err != nil {
			return nil, err
		}
		native = driverScript
	}
	script := "#!/bin/sh\nif [ \"${1-}\" = php-cli ]; then\n shift\n exec " + quote(native) + " \"$@\"\nfi\nexec " + quote(front.PHP) + " \"$@\"\n"
	driver, err := installCache(root, cacheKey("php-driver-"+version, downloadSource{URL: native, SHA256: front.PHP}), func(d string) error { return executable(filepath.Join(d, "frankenphp")) }, func(d string) error { return os.WriteFile(filepath.Join(d, "frankenphp"), []byte(script), 0700) })
	if err != nil {
		return nil, err
	}
	shim := filepath.Join(driver, "frankenphp")
	rt := &Runtime{Dir: dir, PHP: shim, Composer: filepath.Join(dir, "libexec", "composer.phar"), INI: dir, PHPVersion: version, NativeCLI: native, ServerPHP: front.PHP, cacheRoot: root}
	return rt, verifyPHP(rt, version)
}

func verifyPHP(rt *Runtime, want string) error {
	cmd := process.PHP(rt.PHP, "-r", "echo PHP_VERSION;")
	cmd.Env = []string{"PHPRC=" + rt.INI, "PHP_INI_SCAN_DIR=", "PATH=" + rt.Dir}
	out, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			out = append(out, exit.Stderr...)
		}
		return fmt.Errorf("PHP %s runtime failed: %w: %s", want, err, out)
	}
	if strings.TrimSpace(string(out)) != want {
		return fmt.Errorf("PHP runtime reports %q, project selected %s", strings.TrimSpace(string(out)), want)
	}
	return nil
}
