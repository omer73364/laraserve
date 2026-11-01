// Package runtime downloads and caches the tools LaraServe needs on first use.
package runtime

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
)

// Runtime executables are downloaded; only configuration and source catalogues
// are compiled into LaraServe.
//
//go:embed assets/php.ini
var phpINI []byte

var Version = "0.1.0"

const phpVersion = "1.13.4"

type Runtime struct {
	Dir        string
	PHP        string
	Composer   string
	INI        string
	ServerPHP  string
	PHPVersion string
	NativeCLI  string
	cacheRoot  string
}

func localName() string { return "frankenphp" }

func phpSource() (downloadSource, error) {
	type release struct{ name, sum string }
	releases := map[string]release{
		"linux/amd64":  {"frankenphp-linux-x86_64", "7a603536acfe3ce04af26230677e7d4fab9a4527ea27eb5f46e38b07d28df32b"},
		"linux/arm64":  {"frankenphp-linux-aarch64", "8942bc0c1c2876c3cae6bf6a2ae1f7ed3fba750ed3e83654b6c35b12e89d6c02"},
		"darwin/arm64": {"frankenphp-mac-arm64", "50812881758de5406481847a508024a888450ea88cc2e6d6edf57e9274f98354"},
	}
	r, ok := releases[goruntime.GOOS+"/"+goruntime.GOARCH]
	if !ok && os.Getenv("LARASERVE_PHP_URL") == "" {
		return downloadSource{}, fmt.Errorf("no automatic PHP download for %s/%s; set LARASERVE_FRANKENPHP to an existing binary", goruntime.GOOS, goruntime.GOARCH)
	}
	return sourceOverride("PHP", downloadSource{"https://github.com/dunglas/frankenphp/releases/download/v" + phpVersion + "/" + r.name, r.sum})
}

func executable(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode()&0111 == 0 {
		return fmt.Errorf("%s is not executable", path)
	}
	return nil
}

// Ensure downloads PHP once. Composer is deliberately deferred until needed.
func Ensure(runtimesDir string) (*Runtime, error) {
	if p := os.Getenv("LARASERVE_FRANKENPHP"); p != "" {
		rt, err := fromOverride(p)
		if err == nil {
			rt.cacheRoot = runtimesDir
		}
		return rt, err
	}
	source, err := phpSource()
	if err != nil {
		return nil, err
	}
	dir, err := installCache(runtimesDir, cacheKey("php-"+goruntime.GOOS+"-"+goruntime.GOARCH, source), func(d string) error {
		if err := executable(filepath.Join(d, localName())); err != nil {
			return err
		}
		_, err := os.Stat(filepath.Join(d, "php.ini"))
		return err
	}, func(d string) error {
		path := filepath.Join(d, localName())
		if err := downloadFile(source, path, "PHP server"); err != nil {
			return err
		}
		if err := os.Chmod(path, 0755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(d, "php.ini"), phpINI, 0644)
	})
	if err != nil {
		return nil, err
	}
	return &Runtime{Dir: dir, PHP: filepath.Join(dir, localName()), Composer: filepath.Join(dir, "composer.phar"), INI: dir, cacheRoot: runtimesDir}, nil
}

func fromOverride(p string) (*Runtime, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	if err = executable(p); err != nil {
		return nil, fmt.Errorf("LARASERVE_FRANKENPHP: %w", err)
	}
	dir := filepath.Dir(p)
	return &Runtime{Dir: dir, PHP: p, Composer: filepath.Join(dir, "composer.phar"), INI: dir}, nil
}

func EnsureComposer(rt *Runtime) error {
	if st, err := os.Stat(rt.Composer); err == nil && st.Mode().IsRegular() {
		return nil
	}
	source, err := sourceOverride("COMPOSER", downloadSource{"https://getcomposer.org/download/2.11.1/composer.phar", "6f1b891e1d4927d62740276c540fb490e47689e19e102c49e8da849794882f8c"})
	if err != nil {
		return err
	}
	root := rt.cacheRoot
	if root == "" {
		root = filepath.Dir(rt.Dir)
	}
	dir, err := installCache(root, cacheKey("composer", source), func(d string) error { _, err := os.Stat(filepath.Join(d, "composer.phar")); return err }, func(d string) error { return downloadFile(source, filepath.Join(d, "composer.phar"), "Composer") })
	if err != nil {
		return err
	}
	rt.Composer = filepath.Join(dir, "composer.phar")
	return nil
}
