package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

// EnsurePHPMyAdmin returns the downloaded, shared phpMyAdmin distribution.
func EnsurePHPMyAdmin(runtimesDir string) (string, error) {
	if dir := os.Getenv("LARASERVE_PHPMYADMIN_DIR"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return abs, validatePHPMyAdmin(abs)
	}

	source, err := sourceOverride("PHPMYADMIN", downloadSource{"https://files.phpmyadmin.net/phpMyAdmin/5.2.3/phpMyAdmin-5.2.3-all-languages.tar.gz", "12ba1c425fa4071abbd4e7668c9ebdeac0b0755a467a6d6d5026122bb47c102b"})
	if err != nil {
		return "", err
	}
	return installCache(runtimesDir, cacheKey("phpmyadmin", source), validatePHPMyAdmin, func(dir string) error {
		archive := filepath.Join(dir, ".archive")
		if err := downloadFile(source, archive, "phpMyAdmin"); err != nil {
			return err
		}
		if err := extractDistribution(archive, dir); err != nil {
			return err
		}
		return os.Remove(archive)
	})
}

func validatePHPMyAdmin(dir string) error {
	for _, name := range []string{"index.php", "vendor/autoload.php", "libraries/classes/Version.php"} {
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(name))); err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("phpMyAdmin distribution is missing %s", name)
		}
	}
	return nil
}
