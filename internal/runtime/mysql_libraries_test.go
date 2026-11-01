package runtime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type supportTransport func(*http.Request) (*http.Response, error)

func (f supportTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func supportDeb(t *testing.T, name string) []byte {
	t.Helper()
	// Minimal amd64 ELF DSO, sufficient for structural package validation.
	library := make([]byte, 64)
	copy(library, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(library[16:], 3)
	binary.LittleEndian.PutUint16(library[18:], 62)
	binary.LittleEndian.PutUint32(library[20:], 1)
	binary.LittleEndian.PutUint16(library[52:], 64)
	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	tw := tar.NewWriter(gz)
	for path, data := range map[string][]byte{"./usr/lib/x86_64-linux-gnu/" + name: library, "./usr/share/doc/copyright": []byte("test license")} {
		if err := tw.WriteHeader(&tar.Header{Name: path, Mode: 0644, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	for _, member := range []struct {
		name string
		data []byte
	}{{"debian-binary", []byte("2.0\n")}, {"data.tar.gz", compressed.Bytes()}} {
		fmt.Fprintf(&deb, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", member.name+"/", "0", "0", "0", "100644", len(member.data))
		deb.Write(member.data)
		if len(member.data)%2 != 0 {
			deb.WriteByte('\n')
		}
	}
	return deb.Bytes()
}

func TestAutoMySQLSupportAndOfflineReuse(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 support packages")
	}
	originalClient, originalLibraries := downloadClient, mysqlSupportLibraries
	t.Cleanup(func() { downloadClient = originalClient; mysqlSupportLibraries = originalLibraries })
	mysqlSupportLibraries = append([]mysqlSupportLibrary(nil), mysqlSupportLibraries...)
	packages := map[string][]byte{}
	for i, library := range mysqlSupportLibraries {
		t.Setenv("LARASERVE_"+library.key+"_URL", "")
		data := supportDeb(t, library.name)
		packages[library.source.URL] = data
		mysqlSupportLibraries[i].source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	calls := 0
	downloadClient = &http.Client{Transport: supportTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		data, ok := packages[r.URL.String()]
		if !ok {
			return nil, fmt.Errorf("unexpected URL")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data)), Request: r}, nil
	})}
	root := t.TempDir()
	for _, name := range []string{"runtime-one", "runtime-one", "runtime-two"} {
		dir := filepath.Join(root, name)
		if err := downloadMySQLLibraries(dir); err != nil {
			t.Fatal(err)
		}
		for _, library := range mysqlSupportLibraries {
			if err := validateSupportLibrary(filepath.Join(dir, "lib", "private", library.name)); err != nil {
				t.Fatal(err)
			}
		}
		// All later runtime installations must work with no network.
		downloadClient = &http.Client{Transport: supportTransport(func(*http.Request) (*http.Response, error) {
			t.Error("offline cache attempted download")
			return nil, fmt.Errorf("offline")
		})}
	}
	if calls != 2 {
		t.Fatalf("downloaded %d packages, want two", calls)
	}
}

func TestSupportPackageExtractionRejectsMalformedInput(t *testing.T) {
	for _, data := range [][]byte{[]byte("not a deb"), []byte("!<arch>\n"), []byte("!<arch>\n" + strings.Repeat("x", 60))} {
		dir := t.TempDir()
		file := filepath.Join(dir, "package.deb")
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := extractDebData(file, dir); err == nil {
			t.Fatal("accepted malformed deb")
		}
	}
}

func TestRealMySQLSupportInstallation(t *testing.T) {
	archive := os.Getenv("LARASERVE_TEST_MYSQL_SUPPORT_ARCHIVE")
	if archive == "" {
		t.Skip("set LARASERVE_TEST_MYSQL_SUPPORT_ARCHIVE to test real downloads and MySQL initialization")
	}
	dir := filepath.Join(t.TempDir(), "mysql")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := extractDistribution(archive, dir); err != nil {
		t.Fatal(err)
	}
	// Deliberately skip system library discovery. Both missing libraries must be
	// downloaded and loaded from private storage, even on a fully provisioned host.
	if err := downloadMySQLLibraries(dir); err != nil {
		t.Fatal(err)
	}
	if err := verifyMySQL(dir, "8.4.13"); err != nil {
		t.Fatal(err)
	}
	args := []string{"--no-defaults", "--basedir=" + dir, "--datadir=" + filepath.Join(dir, "data"), "--initialize-insecure"}
	if os.Geteuid() == 0 {
		args = append(args, "--user=root")
	}
	cmd := exec.Command(filepath.Join(dir, "bin", "mysqld"), args...)
	cmd.Env = MySQLEnv(dir, os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("MySQL initialization with downloaded libraries: %v %s", err, out)
	}
	t.Log("Real MySQL initialized using privately downloaded libaio and libnuma")
}

func TestMySQLSupportRejectsBadChecksumAndRetries(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("Linux amd64 support packages")
	}
	originalClient, originalLibraries := downloadClient, mysqlSupportLibraries
	t.Cleanup(func() { downloadClient = originalClient; mysqlSupportLibraries = originalLibraries })
	library := mysqlSupportLibraries[0]
	data := supportDeb(t, library.name)
	library.source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	mysqlSupportLibraries = []mysqlSupportLibrary{library}
	t.Setenv("LARASERVE_"+library.key+"_URL", "")
	corrupt := true
	downloadClient = &http.Client{Transport: supportTransport(func(r *http.Request) (*http.Response, error) {
		body := data
		if corrupt {
			body = []byte("corrupted package")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: r}, nil
	})}
	dir := filepath.Join(t.TempDir(), "mysql")
	if err := downloadMySQLLibraries(dir); err == nil {
		t.Fatal("accepted checksum mismatch")
	}
	if _, err := os.Stat(filepath.Join(dir, "lib", "private", library.name)); !os.IsNotExist(err) {
		t.Fatal("published corrupted library")
	}
	corrupt = false
	if err := downloadMySQLLibraries(dir); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
}
