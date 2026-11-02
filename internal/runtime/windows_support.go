package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

func windowsPHPVersions() []string {
	if goruntime.GOARCH != "amd64" {
		return nil
	}
	return []string{"7.4.33", "8.0.30", "8.1.34", "8.2.35", "8.3.36", "8.4.27", "8.5.12"}
}

func windowsPHPSource(version string) (downloadSource, error) {
	builds := map[string]string{
		"7.4.33": "7.4.33-nts-Win32-vc15",
		"8.0.30": "8.0.30-nts-Win32-vs16",
		"8.1.34": "8.1.34-nts-Win32-vs16",
		"8.2.35": "8.2.35-1-nts-Win32-vs16",
		"8.3.36": "8.3.36-1-nts-Win32-vs16",
		"8.4.27": "8.4.27-1-nts-Win32-vs17",
		"8.5.12": "8.5.12-1-nts-Win32-vs17",
	}
	build, ok := builds[version]
	if !ok {
		return downloadSource{}, fmt.Errorf("no Windows PHP download for %s", version)
	}
	return sourceOverride("PHP_RUNTIME", downloadSource{"https://www.php.net/~windows/releases/php-" + build + "-x64.zip", "publisher"})
}

func ensureWindowsPHP(root, version string) (*Runtime, error) {
	if goruntime.GOARCH != "amd64" {
		return nil, fmt.Errorf("native Windows runtime downloads currently support amd64 only")
	}
	source, err := windowsPHPSource(version)
	if err != nil {
		return nil, err
	}
	dir, err := installCache(root, cacheKey("php-windows-amd64-"+version, source), func(d string) error {
		for _, f := range []string{"php.exe", "php.ini", "cacert.pem"} {
			if _, err := os.Stat(filepath.Join(d, f)); err != nil {
				return err
			}
		}
		return nil
	}, func(d string) error {
		archive := filepath.Join(d, ".archive")
		if err := downloadFile(source, archive, "PHP "+version+" for Windows"); err != nil {
			return err
		}
		if err := extractZip(archive, d, false); err != nil {
			return err
		}
		if err := os.Remove(archive); err != nil {
			return err
		}
		ca, err := sourceOverride("CA_BUNDLE", downloadSource{"https://curl.se/ca/cacert.pem", "publisher"})
		if err != nil {
			return err
		}
		if err = downloadFile(ca, filepath.Join(d, "cacert.pem"), "PHP CA bundle"); err != nil {
			return err
		}
		// Relative paths are not reliable when the working directory is the app.
		// php.ini is rewritten with the final cache path after atomic publication.
		return os.WriteFile(filepath.Join(d, "php.ini"), phpINI, 0600)
	})
	if err != nil {
		return nil, err
	}
	// Keep configuration in a separate versioned cache: never rewrite a shared
	// runtime's ini file while another project may be starting.
	iniSource := downloadSource{URL: dir, SHA256: "windows-ini-v1"}
	ini, err := installCache(root, cacheKey("windows-php-ini", iniSource), func(d string) error { _, err := os.Stat(filepath.Join(d, "php.ini")); return err }, func(d string) error {
		config := string(phpINI) + "\nextension_dir = " + iniQuote(filepath.Join(dir, "ext")) + "\n"
		extensions := []string{"curl", "fileinfo", "intl", "mbstring", "mysqli", "openssl", "pdo_mysql", "pdo_sqlite", "sqlite3"}
		if strings.HasPrefix(version, "7.4.") {
			extensions = append(extensions, "gd2")
		} else {
			extensions = append(extensions, "gd")
		}
		if !strings.HasPrefix(version, "7.4.") && !strings.HasPrefix(version, "8.0.") && !strings.HasPrefix(version, "8.1.") {
			extensions = append(extensions, "zip")
		}
		for _, ext := range extensions {
			config += "extension = " + ext + "\n"
		}
		config += "curl.cainfo = " + iniQuote(filepath.Join(dir, "cacert.pem")) + "\nopenssl.cafile = " + iniQuote(filepath.Join(dir, "cacert.pem")) + "\n"
		return os.WriteFile(filepath.Join(d, "php.ini"), []byte(config), 0600)
	})
	if err != nil {
		return nil, err
	}
	caddy, err := ensureWindowsCaddy(root)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{Dir: dir, PHP: filepath.Join(dir, "php.exe"), NativeCLI: filepath.Join(dir, "php.exe"), ServerPHP: caddy, Composer: filepath.Join(dir, "composer.phar"), INI: ini, PHPVersion: version, cacheRoot: root}
	if err = verifyPHP(rt, version); err != nil {
		return nil, fmt.Errorf("%w; Windows PHP requires the Microsoft Visual C++ 2015–2022 x64 runtime (https://aka.ms/vs/17/release/vc_redist.x64.exe)", err)
	}
	return rt, nil
}

func iniQuote(s string) string { return `"` + strings.ReplaceAll(filepath.ToSlash(s), `"`, `\"`) + `"` }

func ensureWindowsCaddy(root string) (string, error) {
	source, err := sourceOverride("CADDY", downloadSource{"https://github.com/caddyserver/caddy/releases/download/v2.11.7/caddy_2.11.7_windows_amd64.zip", "publisher"})
	if err != nil {
		return "", err
	}
	dir, err := installCache(root, cacheKey("caddy-windows-amd64", source), func(d string) error { return executable(filepath.Join(d, "caddy.exe")) }, func(d string) error {
		archive := filepath.Join(d, ".archive")
		if err := downloadFile(source, archive, "Caddy for Windows"); err != nil {
			return err
		}
		if err := extractZip(archive, d, false); err != nil {
			return err
		}
		return os.Remove(archive)
	})
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "caddy.exe"), nil
}
