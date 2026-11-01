package runtime

import (
	"debug/elf"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed php_legacy_packages.json
var legacyPackagesJSON []byte

//go:embed php74_packages.json
var php74PackagesJSON []byte

type legacyPackage struct {
	Name, Version string
	Libraries     []string
	Source        downloadSource
}

// PHP 7.4 packages target Ubuntu 22.04. Inspect ELF dependencies
// without running ldd. Reuse host libraries and download only missing SONAMEs.
// glibc itself must be supplied by the OS; it is never replaced privately.
func ensureLegacyPHPLibraries(dir string) error {
	unlock, err := lockDownload(dir + ".libraries.lock")
	if err != nil {
		return err
	}
	defer unlock()
	if err := configurePHP74(dir); err != nil {
		return err
	}
	var packages []legacyPackage
	if err := json.Unmarshal(legacyPackagesJSON, &packages); err != nil {
		return err
	}
	catalog := map[string]legacyPackage{}
	for _, p := range packages {
		for _, name := range p.Libraries {
			catalog[name] = p
		}
	}
	private := filepath.Join(dir, "lib", "private")
	search := []string{private, "/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu", "/lib64", "/usr/lib64", "/lib", "/usr/lib"}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(path string) error {
		if visited[path] {
			return nil
		}
		visited[path] = true
		object, err := elf.Open(path)
		if err != nil {
			return fmt.Errorf("inspect PHP 7.4 dependency %s: %w", path, err)
		}
		needed, err := object.ImportedLibraries()
		object.Close()
		if err != nil {
			return err
		}
		for _, name := range needed {
			path := findLegacyLibrary(name, search)
			if path == "" {
				p, ok := catalog[name]
				if !ok {
					return fmt.Errorf("PHP 7.4 needs %s, which is not available automatically; Linux amd64 with glibc 2.35+ is required", name)
				}
				if err := installLegacyPackage(dir, p); err != nil {
					return fmt.Errorf("install PHP 7.4 dependency %s: %w", name, err)
				}
				path = findLegacyLibrary(name, search)
				if path == "" {
					return fmt.Errorf("PHP dependency package %s did not provide %s", p.Name, name)
				}
			}
			if err := visit(path); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(filepath.Join(dir, "libexec", "php")); err != nil {
		return err
	}
	modules, _ := filepath.Glob(filepath.Join(dir, "libexec", "extensions", "*.so"))
	for _, module := range modules {
		if err := visit(module); err != nil {
			return err
		}
	}
	return nil
}

func findLegacyLibrary(name string, search []string) string {
	if filepath.Base(name) != name {
		return ""
	}
	for _, dir := range search {
		path := filepath.Join(dir, name)
		if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func installLegacyPackage(runtimeDir string, p legacyPackage) error {
	kind := "PHP_" + strings.ToUpper(strings.NewReplacer("-", "_", "+", "_").Replace(p.Name))
	source, err := sourceOverride(kind, p.Source)
	if err != nil {
		return err
	}
	cache := filepath.Join(filepath.Dir(runtimeDir), "php-support")
	validate := func(root string) error {
		for _, name := range p.Libraries {
			if findLegacyLibrary(name, []string{filepath.Join(root, "usr/lib/x86_64-linux-gnu"), filepath.Join(root, "lib/x86_64-linux-gnu")}) == "" {
				return fmt.Errorf("package lacks %s", name)
			}
		}
		return nil
	}
	dep, err := installCache(cache, cacheKey(p.Name+"-linux-amd64", source), validate, func(root string) error {
		archive := filepath.Join(root, ".package.deb")
		if err := downloadFile(source, archive, "PHP 7.4 support: "+p.Name); err != nil {
			return err
		}
		if err := extractDebData(archive, root); err != nil {
			return err
		}
		return os.Remove(archive)
	})
	if err != nil {
		return err
	}
	private := filepath.Join(runtimeDir, "lib", "private")
	if err := os.MkdirAll(private, 0755); err != nil {
		return err
	}
	// Materialize symlink aliases as files in one private loader directory. Only
	// the verified package's libraries are copied, never configuration or scripts.
	for _, base := range []string{"usr/lib/x86_64-linux-gnu", "lib/x86_64-linux-gnu"} {
		entries, err := os.ReadDir(filepath.Join(dep, base))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !strings.Contains(entry.Name(), ".so") {
				continue
			}
			src := filepath.Join(dep, base, entry.Name())
			st, err := os.Stat(src)
			if err != nil {
				return err
			}
			if !st.Mode().IsRegular() {
				continue
			}
			data, err := os.ReadFile(src)
			if err != nil {
				return err
			}
			if err := writeMySQLLibrary(filepath.Join(private, entry.Name()), data); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeLegacyPHPLauncher(path, script string) error {
	if data, err := os.ReadFile(path); err == nil && string(data) == script {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".php-launcher-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0700); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func populatePHP74(dir string) error {
	var packages []legacyPackage
	if err := json.Unmarshal(php74PackagesJSON, &packages); err != nil {
		return err
	}
	extDir := filepath.Join(dir, "libexec", "extensions")
	if err := os.MkdirAll(extDir, 0755); err != nil {
		return err
	}
	for _, p := range packages {
		root := filepath.Join(dir, "packages", p.Name)
		if err := os.MkdirAll(root, 0755); err != nil {
			return err
		}
		source, err := sourceOverride("PHP74_"+strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(p.Name)), p.Source)
		if err != nil {
			return err
		}
		archive := filepath.Join(root, ".package.deb")
		if err := downloadFile(source, archive, "PHP 7.4: "+p.Name); err != nil {
			return err
		}
		if err := extractDebData(archive, root); err != nil {
			return err
		}
		if err := os.Remove(archive); err != nil {
			return err
		}
		if p.Name == "php7.4-cli" {
			data, err := os.ReadFile(filepath.Join(root, "usr", "bin", "php7.4"))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, "libexec", "php"), data, 0755); err != nil {
				return err
			}
		}
		modules, _ := filepath.Glob(filepath.Join(root, "usr", "lib", "php", "*", "*.so"))
		for _, module := range modules {
			data, err := os.ReadFile(module)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(extDir, filepath.Base(module)), data, 0644); err != nil {
				return err
			}
		}
	}
	return configurePHP74(dir)
}

func configurePHP74(dir string) error {
	extDir := filepath.Join(dir, "libexec", "extensions")
	if _, err := os.Stat(extDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Keep dependencies before their consumers. PHPRC points only to this ini;
	// package configuration and maintainer scripts are intentionally unused.
	ini := string(phpINI) + "\nextension_dir = \"${LARASERVE_PHP_EXTENSION_DIR}\"\n"
	for _, name := range []string{"pdo", "mysqlnd", "json", "phar", "posix", "shmop", "sysvsem", "sysvshm", "sysvmsg", "ftp", "ctype", "fileinfo", "tokenizer", "iconv", "calendar", "exif", "gettext", "sockets", "bcmath", "curl", "mbstring", "mysqli", "pdo_mysql", "sqlite3", "pdo_sqlite", "dom", "simplexml", "xml", "xmlreader", "xmlwriter", "xsl", "zip", "intl", "readline"} {
		if _, err := os.Stat(filepath.Join(extDir, name+".so")); err == nil {
			ini += "extension=" + name + ".so\n"
		}
	}
	return writeMySQLLibrary(filepath.Join(dir, "php.ini"), []byte(ini))
}
