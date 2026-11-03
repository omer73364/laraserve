package runtime

import (
	"bufio"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/url"
	"path"
	"strings"
	"sync"
)

// Resolve publisher metadata only on cache misses. Never derive the expected
// checksum from the downloaded artifact or silently accept missing metadata.
func publisherChecksum(raw string) (hash.Hash, string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, "", err
	}
	var sum string
	switch u.Hostname() {
	case "nodejs.org":
		b, e := checksumMetadata(nodeChecksumURL(raw))
		if e != nil {
			return nil, "", e
		}
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == path.Base(u.Path) {
				sum = fields[0]
				break
			}
		}
	case "github.com":
		p := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(p) != 6 || p[2] != "releases" || p[3] != "download" {
			return nil, "", fmt.Errorf("unsupported GitHub release URL")
		}
		endpoint := "https://api.github.com/repos/" + p[0] + "/" + p[1] + "/releases/tags/" + url.PathEscape(p[4])
		b, e := checksumMetadata(endpoint)
		if e != nil {
			return nil, "", e
		}
		var release struct {
			Assets []struct{ Name, Digest string }
		}
		if e = json.Unmarshal(b, &release); e != nil {
			return nil, "", e
		}
		for _, a := range release.Assets {
			if a.Name == p[5] {
				sum = strings.TrimPrefix(a.Digest, "sha256:")
				break
			}
		}
	case "archive.ubuntu.com", "security.ubuntu.com", "ppa.launchpadcontent.net":
		sum, err = packageChecksum(u)
		if err != nil {
			return nil, "", err
		}
	case "www.php.net", "downloads.php.net":
		b, e := checksumMetadata("https://www.php.net/~windows/releases/sha256sum.txt")
		if e != nil {
			return nil, "", e
		}
		for _, line := range strings.Split(string(b), "\n") {
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == path.Base(u.Path) {
				sum = fields[0]
				break
			}
		}
	case "curl.se":
		sum, err = checksumSidecar(raw+".sha256", 64)
	case "getcomposer.org":
		sum, err = checksumSidecar(raw+".sha256sum", 64)
	case "files.phpmyadmin.net":
		sum, err = checksumSidecar(raw+".sha256", 64)
	case "cdn.mysql.com":
		// Oracle publishes MD5 sidecars for these distributions over HTTPS.
		sum, err = checksumSidecar(raw+".md5", 32)
		if err != nil {
			return nil, "", err
		}
		return md5.New(), sum, nil
	default:
		return nil, "", fmt.Errorf("no publisher checksum source for %s; configure an explicit mirror SHA256", u.Hostname())
	}
	if err != nil {
		return nil, "", err
	}
	b, e := hex.DecodeString(sum)
	if e != nil || len(b) != 32 {
		return nil, "", fmt.Errorf("publisher did not provide a SHA256 checksum for %s", path.Base(u.Path))
	}
	return sha256.New(), strings.ToLower(sum), nil
}

func checksumMetadata(raw string) ([]byte, error) {
	resp, err := downloadClient.Get(raw)
	if err != nil {
		return nil, fmt.Errorf("fetch checksum metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("checksum metadata %s: HTTP %d", raw, resp.StatusCode)
	}
	if resp.Request == nil || resp.Request.URL.Scheme != "https" {
		return nil, fmt.Errorf("insecure checksum metadata redirect")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20+1))
	if len(b) > 64<<20 {
		return nil, fmt.Errorf("checksum metadata too large")
	}
	return b, err
}

func checksumSidecar(raw string, size int) (string, error) {
	b, err := checksumMetadata(raw)
	if err != nil {
		return "", err
	}
	// Accept GNU checksum files, plain digests, and BSD MD5 (file) = digest.
	for _, f := range strings.Fields(string(b)) {
		if len(f) == size {
			if _, e := hex.DecodeString(f); e == nil {
				return strings.ToLower(f), nil
			}
		}
	}
	return "", fmt.Errorf("invalid publisher checksum at %s", raw)
}

var packageIndexes = struct {
	sync.Mutex
	entries map[string]map[string]string
}{entries: make(map[string]map[string]string)}

func packageChecksum(u *url.URL) (string, error) {
	marker := strings.Index(u.Path, "/pool/")
	if marker < 0 {
		return "", fmt.Errorf("unsupported package URL")
	}
	base := u.Scheme + "://" + u.Host + u.Path[:marker]
	filename := strings.TrimPrefix(u.Path[marker+1:], "/")
	parts := strings.Split(strings.TrimPrefix(filename, "pool/"), "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid package filename")
	}
	suites := []string{"jammy-updates", "jammy-security", "jammy"}
	if u.Hostname() == "ppa.launchpadcontent.net" {
		suites = []string{"jammy"}
	}
	for _, suite := range suites {
		endpoint := base + "/dists/" + suite + "/" + parts[0] + "/binary-amd64/Packages.gz"
		entries, e := readPackageIndex(endpoint)
		if e != nil {
			return "", e
		}
		if sum := entries[filename]; sum != "" {
			return sum, nil
		}
	}
	return "", fmt.Errorf("package %s is no longer listed by its publisher; runtime catalog needs an updated package version", path.Base(u.Path))
}

func readPackageIndex(endpoint string) (map[string]string, error) {
	packageIndexes.Lock()
	defer packageIndexes.Unlock()
	if entries, ok := packageIndexes.entries[endpoint]; ok {
		return entries, nil
	}
	b, err := checksumMetadata(endpoint)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	entries, err := parsePackageChecksums(io.LimitReader(gz, 256<<20))
	if err != nil {
		return nil, err
	}
	packageIndexes.entries[endpoint] = entries
	return entries, nil
}

func parsePackageChecksums(r io.Reader) (map[string]string, error) {
	entries := map[string]string{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	var filename, sum string
	flush := func() {
		if filename != "" && sum != "" {
			entries[filename] = sum
		}
		filename = ""
		sum = ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
		}
		if strings.HasPrefix(line, "Filename: ") {
			filename = strings.TrimPrefix(line, "Filename: ")
		}
		if strings.HasPrefix(line, "SHA256: ") {
			sum = strings.TrimPrefix(line, "SHA256: ")
		}
	}
	flush()
	return entries, scanner.Err()
}
