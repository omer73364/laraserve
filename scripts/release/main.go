// release prepares local artifacts only. It never uploads or publishes them.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var binaries = []string{"laraserve-linux-amd64", "laraserve-linux-arm64", "laraserve-darwin-amd64", "laraserve-darwin-arm64", "laraserve-windows-amd64.exe"}

func main() {
	dir := flag.String("dir", "dist", "build output directory")
	version := flag.String("version", "", "release version")
	archives := flag.Bool("archives", false, "prepare immutable versioned ZIP archives with LICENSE")
	flag.Parse()
	var err error
	if *archives {
		err = createRelease(*dir, *version)
	} else {
		err = checksums(*dir, binaries)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func checksums(dir string, names []string) error {
	var result strings.Builder
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintf(&result, "%s  %s\n", hex.EncodeToString(hash.Sum(nil)), name)
	}
	return os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(result.String()), 0644)
}

func createRelease(dir, version string) error {
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`).MatchString(version) {
		return fmt.Errorf("provide a semantic release version, for example --version 0.1.0")
	}
	destination := filepath.Join(dir, "releases", "v"+version)
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("release %s already exists; keep it as a restore point or choose a new version", version)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), ".release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	notices, err := dependencyNotices()
	if err != nil {
		return err
	}
	var archives []string
	for _, binary := range binaries {
		name := strings.TrimSuffix(binary, ".exe") + "-v" + version + ".zip"
		if err = archiveBinary(filepath.Join(staging, name), filepath.Join(dir, binary), license, notices...); err != nil {
			return err
		}
		archives = append(archives, name)
	}
	if err = checksums(staging, archives); err != nil {
		return err
	}
	if err = os.Rename(staging, destination); err != nil {
		return err
	}
	fmt.Println("Release prepared locally in", destination)
	return nil
}
func archiveBinary(output, binary string, license []byte, notices ...archiveNotice) error {
	source, err := os.Open(binary)
	if err != nil {
		return err
	}
	defer source.Close()
	out, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(out)
	add := func(name string, mode os.FileMode, reader io.Reader) error {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
		h.SetMode(mode)
		entry, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, reader)
		return err
	}
	name := "laraserve"
	if strings.HasSuffix(binary, ".exe") {
		name += ".exe"
	}
	err = add(name, 0755, source)
	if err == nil {
		err = add("LICENSE", 0644, strings.NewReader(string(license)))
	}
	for _, notice := range notices {
		if err == nil {
			err = add(notice.name, 0644, strings.NewReader(string(notice.data)))
		}
	}
	zipErr := zw.Close()
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if zipErr != nil {
		return zipErr
	}
	return closeErr
}

type archiveNotice struct {
	name string
	data []byte
}

func dependencyNotices() ([]archiveNotice, error) {
	output, err := exec.Command("go", "list", "-m", "-json", "all").Output()
	if err != nil {
		return nil, fmt.Errorf("list Go module licenses: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(output)))
	notices := []archiveNotice{}
	var summary strings.Builder
	summary.WriteString("LaraServe client dependencies\n\nSource versions are listed below and pinned by go.mod/go.sum.\nUpstream license texts are included under licenses/.\nDownloaded server runtimes are separate and retain their own upstream licenses.\n\n")
	for decoder.More() {
		var module struct {
			Path, Version, Dir string
			Main               bool
		}
		if err = decoder.Decode(&module); err != nil {
			return nil, err
		}
		if module.Main {
			continue
		}
		fmt.Fprintf(&summary, "%s %s\n", module.Path, module.Version)
		found := false
		for _, pattern := range []string{"LICENSE*", "COPYING*", "NOTICE*", "PATENTS*"} {
			files, _ := filepath.Glob(filepath.Join(module.Dir, pattern))
			for _, file := range files {
				st, err := os.Stat(file)
				if err != nil {
					return nil, err
				}
				if !st.Mode().IsRegular() {
					continue
				}
				b, err := os.ReadFile(file)
				if err != nil {
					return nil, err
				}
				notices = append(notices, archiveNotice{"licenses/" + module.Path + "/" + filepath.Base(file), b})
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("no license text found for %s; download its source before packaging", module.Path)
		}
	}
	goLicense, err := os.ReadFile(filepath.Join(runtime.GOROOT(), "LICENSE"))
	if err != nil {
		return nil, err
	}
	notices = append(notices, archiveNotice{"licenses/go/LICENSE", goLicense}, archiveNotice{"THIRD_PARTY_NOTICES.txt", []byte(summary.String())})
	return notices, nil
}
