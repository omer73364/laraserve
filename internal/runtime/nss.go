package runtime

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

//go:embed nss_packages.json
var nssPackagesJSON []byte

// EnsureNSS provides a private Linux NSS certutil, never the unrelated Windows
// program. Importing certificates remains the trust package's explicit action.
func EnsureNSS(root string) (string, error) {
	if goruntime.GOOS != "linux" || goruntime.GOARCH != "amd64" {
		return "", fmt.Errorf("automatic NSS tools currently support Linux amd64 (glibc 2.35+); install your distribution's libnss3-tools or nss-tools on this platform")
	}
	var packages []legacyPackage
	if err := json.Unmarshal(nssPackagesJSON, &packages); err != nil {
		return "", err
	}
	// Mirrors participate in the key so changing a checksum never reuses a
	// package installation produced from different source inputs.
	for i, p := range packages {
		kind := "NSS_" + strings.ToUpper(strings.ReplaceAll(p.Name, "-", "_"))
		source, err := sourceOverride(kind, p.Source)
		if err != nil {
			return "", err
		}
		packages[i].Source = source
	}
	encoded, _ := json.Marshal(packages)
	digest := sha256.Sum256(encoded)
	key := "nss-linux-amd64-" + fmt.Sprintf("%x", digest[:10])
	dir, err := installCache(root, key, func(dir string) error {
		for _, name := range []string{"bin/certutil", "usr/bin/certutil", "usr/lib/x86_64-linux-gnu/libnss3.so", "usr/lib/x86_64-linux-gnu/libnspr4.so"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
		return executable(filepath.Join(dir, "usr/bin/certutil"))
	}, func(dir string) error {
		for _, p := range packages {
			archive := filepath.Join(dir, ".package.deb")
			if err := downloadFile(p.Source, archive, "Browser trust: "+p.Name); err != nil {
				return err
			}
			if err := extractDebData(archive, dir); err != nil {
				return err
			}
			if err := os.Remove(archive); err != nil {
				return err
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			return err
		}
		script := `#!/bin/sh
root=${0%/bin/certutil}
export LD_LIBRARY_PATH="$root/usr/lib/x86_64-linux-gnu:$root/lib/x86_64-linux-gnu"
exec "$root/usr/bin/certutil" "$@"
`
		return os.WriteFile(filepath.Join(dir, "bin/certutil"), []byte(script), 0700)
	})
	if err != nil {
		return "", err
	}
	tool := filepath.Join(dir, "bin", "certutil")
	out, err := exec.Command(tool, "-H").CombinedOutput()
	if !strings.Contains(string(out), "-A") || !strings.Contains(string(out), "-L") {
		return "", fmt.Errorf("private NSS tools could not run: %v: %s", err, out)
	}
	return tool, nil
}
