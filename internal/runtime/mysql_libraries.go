package runtime

import (
	"debug/elf"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
)

type mysqlSupportLibrary struct {
	name, key, version string
	source             downloadSource
}

// These small official Ubuntu packages have an older glibc baseline than either
// supported MySQL runtime. Only their libraries are loaded; maintainer scripts
// are never executed. Copyright notices remain in the dependency cache.
var mysqlSupportLibraries = []mysqlSupportLibrary{
	{"libaio.so.1", "LIBAIO", "0.3.112-5", downloadSource{
		"https://archive.ubuntu.com/ubuntu/pool/main/liba/libaio/libaio1_0.3.112-5_amd64.deb",
		"publisher",
	}},
	{"libnuma.so.1", "LIBNUMA", "2.0.12-1", downloadSource{
		"https://archive.ubuntu.com/ubuntu/pool/main/n/numactl/libnuma1_2.0.12-1_amd64.deb",
		"publisher",
	}},
}

func downloadMySQLLibraries(dir string) error {
	if goruntime.GOOS != "linux" || goruntime.GOARCH != "amd64" {
		return nil
	}
	cache := filepath.Join(filepath.Dir(dir), "mysql-support")
	for _, library := range mysqlSupportLibraries {
		target := filepath.Join(dir, "lib", "private", library.name)
		if _, err := os.Stat(target); err == nil {
			continue
		}
		source, err := sourceOverride(library.key, library.source)
		if err != nil {
			return err
		}
		validate := func(root string) error {
			return validateSupportLibrary(filepath.Join(root, "usr", "lib", "x86_64-linux-gnu", library.name))
		}
		dep, err := installCache(cache, cacheKey(strings.ToLower(library.key)+"-"+library.version+"-linux-amd64", source), validate, func(root string) error {
			archive := filepath.Join(root, ".package.deb")
			if err := downloadFile(source, archive, "MySQL support: "+library.name); err != nil {
				return err
			}
			if err := extractDebData(archive, root); err != nil {
				return err
			}
			return os.Remove(archive)
		})
		if err != nil {
			return fmt.Errorf("automatically installing %s: %w", library.name, err)
		}
		data, err := os.ReadFile(filepath.Join(dep, "usr", "lib", "x86_64-linux-gnu", library.name))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := writeMySQLLibrary(target, data); err != nil {
			return err
		}
	}
	return nil
}

func validateSupportLibrary(path string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if f.Machine != elf.EM_X86_64 || f.Class != elf.ELFCLASS64 || f.Type != elf.ET_DYN {
		return fmt.Errorf("MySQL support package does not contain an amd64 shared library: %s", path)
	}
	return nil
}

// Debian packages are ar containers. Read just the bounded data archive, then
// use the existing traversal-safe tar extractor. No dpkg, ar, or xz executable
// is required on the client, and package install scripts are not run.
func extractDebData(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	magic := make([]byte, 8)
	if _, err := io.ReadFull(f, magic); err != nil || string(magic) != "!<arch>\n" {
		return fmt.Errorf("invalid Debian package header")
	}
	for {
		header := make([]byte, 60)
		if _, err := io.ReadFull(f, header); err != nil {
			return fmt.Errorf("Debian package has no supported data archive: %w", err)
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 || size > 64<<20 || string(header[58:60]) != "`\n" {
			return fmt.Errorf("invalid Debian package member")
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(header[:16])), "/")
		if name == "data.tar.xz" || name == "data.tar.gz" || name == "data.tar.zst" {
			tmp, err := os.CreateTemp(dir, ".deb-data-*")
			if err != nil {
				return err
			}
			defer os.Remove(tmp.Name())
			_, copyErr := io.CopyN(tmp, f, size)
			closeErr := tmp.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			return extractRuntimeArchive(tmp.Name(), dir, false, true)
		}
		if _, err := io.CopyN(io.Discard, f, size+size%2); err != nil {
			return fmt.Errorf("truncated Debian package: %w", err)
		}
	}
}
