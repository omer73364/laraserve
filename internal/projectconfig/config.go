// Package projectconfig defines the shareable project configuration consumed by
// both interfaces. Secrets, absolute paths, credentials and process state are excluded.
package projectconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"laraserve/internal/statefile"
)

type Config struct {
	PHP         string            `json:"php,omitempty"`
	MySQL       string            `json:"mysql,omitempty"`
	Domain      string            `json:"domain,omitempty"`
	HTTPS       *bool             `json:"https,omitempty"`
	Port        *int              `json:"port,omitempty"`
	Database    string            `json:"database,omitempty"` // auto, managed, external, sqlite
	PHPMyAdmin  *bool             `json:"phpmyadmin,omitempty"`
	PHPSettings map[string]string `json:"php_settings,omitempty"`
	PHPRaw      string            `json:"php_ini,omitempty"`
}
type Document struct {
	Config   Config `json:"config"`
	Revision string `json:"revision"`
}

func Digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func Read(root string) (Document, error) {
	b, err := ReadRegular(filepath.Join(root, ".laraserve.json"), 64<<10)
	if os.IsNotExist(err) {
		return Document{Revision: Digest(nil)}, nil
	}
	if err != nil {
		return Document{}, err
	}
	var c Config
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&c); err != nil {
		return Document{}, fmt.Errorf("invalid .laraserve.json: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Document{}, fmt.Errorf("invalid trailing configuration")
	}
	if err = c.Validate(); err != nil {
		return Document{}, err
	}
	return Document{Config: c, Revision: Digest(b)}, nil
}

var domain = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)
var pin = regexp.MustCompile(`^[0-9v.*xX^~<>=!| ,+-]*$`)

func (c Config) Validate() error {
	if len(c.Domain) > 253 || (c.Domain != "" && !domain.MatchString(c.Domain)) || strings.Contains(c.Domain, "..") {
		return fmt.Errorf("invalid local domain")
	}
	for _, part := range strings.Split(c.Domain, ".") {
		if len(part) > 63 || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return fmt.Errorf("invalid domain label")
		}
	}
	if c.Port != nil && (*c.Port < 0 || *c.Port > 65535) {
		return fmt.Errorf("port must be 0 through 65535")
	}
	for _, v := range []string{c.PHP, c.MySQL} {
		if len(v) > 96 || (v != "latest" && !pin.MatchString(v)) {
			return fmt.Errorf("invalid runtime version constraint")
		}
	}
	switch c.Database {
	case "", "auto", "managed", "external", "sqlite":
	default:
		return fmt.Errorf("unknown database mode")
	}
	_, err := c.Directives()
	return err
}

var sizes = regexp.MustCompile(`^(?:-1|[0-9]{1,7}[KMGkmg]?)$`)

func validDirective(key, value string) bool {
	if strings.ContainsAny(value, "\r\n\x00;#\"'") {
		return false
	}
	switch key {
	case "memory_limit", "upload_max_filesize", "post_max_size":
		return sizes.MatchString(value)
	case "max_execution_time", "max_input_time", "max_input_vars":
		n, e := strconv.Atoi(value)
		return e == nil && n >= 0 && n <= 1000000
	case "display_errors", "log_errors", "opcache.validate_timestamps", "opcache.enable", "opcache.enable_cli":
		return value == "0" || value == "1" || value == "On" || value == "Off"
	case "date.timezone":
		_, e := time.LoadLocation(value)
		return e == nil
	case "opcache.revalidate_freq":
		n, e := strconv.Atoi(value)
		return e == nil && n >= 0 && n <= 3600
	}
	return false
}
func (c Config) Directives() (map[string]string, error) {
	if len(c.PHPRaw) > 8192 || len(c.PHPSettings) > 20 {
		return nil, fmt.Errorf("PHP settings too large")
	}
	result := map[string]string{}
	for key, value := range c.PHPSettings {
		if !validDirective(key, value) {
			return nil, fmt.Errorf("invalid or unsupported PHP directive %s", key)
		}
		result[key] = value
	}
	for i, line := range strings.Split(c.PHPRaw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !validDirective(key, value) {
			return nil, fmt.Errorf("invalid or unsupported PHP directive on raw line %d", i+1)
		}
		if _, ok = result[key]; ok {
			return nil, fmt.Errorf("duplicate PHP directive %s", key)
		}
		result[key] = value
	}
	return result, nil
}
func (c Config) INI() (string, error) {
	values, err := c.Directives()
	if err != nil {
		return "", err
	}
	var keys []string
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s = %s\n", k, values[k])
	}
	return b.String(), nil
}
func Save(home, root, id, revision string, c Config) (Document, error) {
	if err := c.Validate(); err != nil {
		return Document{}, err
	}
	current, err := Read(root)
	if err != nil {
		return Document{}, err
	}
	if current.Revision != revision {
		return Document{}, fmt.Errorf("configuration changed; reload before saving")
	}
	file := filepath.Join(root, ".laraserve.json")
	if err = Backup(home, id, "config", file); err != nil {
		return Document{}, err
	}
	if err = statefile.WriteJSON(file, c); err != nil {
		return Document{}, err
	}
	return Read(root)
}

// ReadRegular refuses symlink/FIFO configuration files and caps input size.
func ReadRegular(path string, limit int64) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > limit {
		return nil, fmt.Errorf("expected a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, fmt.Errorf("file changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > int(limit) {
		return nil, fmt.Errorf("file too large")
	}
	return b, err
}
func Backup(home, id, kind, file string) error {
	b, err := ReadRegular(file, 128<<10)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	dir := filepath.Join(home, "history", id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	target := filepath.Join(dir, time.Now().UTC().Format("20060102T150405.000000000")+"-"+kind+".bak")
	return os.WriteFile(target, b, 0600)
}
