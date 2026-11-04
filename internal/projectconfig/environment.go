package projectconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"laraserve/internal/statefile"
)

type Variable struct {
	Key       string `json:"key"`
	Value     string `json:"value"`
	Example   string `json:"example"`
	Secret    bool   `json:"secret"`
	Present   bool   `json:"present"`
	InExample bool   `json:"in_example"`
}
type Environment struct {
	Revision  string     `json:"revision"`
	Variables []Variable `json:"variables"`
}

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func secret(key string) bool {
	key = strings.ToUpper(key)
	// Treat all values as private except a small set of useful noncredential fields.
	switch key {
	case "APP_NAME", "APP_ENV", "APP_DEBUG", "APP_URL", "APP_LOCALE", "APP_TIMEZONE", "LOG_CHANNEL", "LOG_LEVEL", "DB_CONNECTION", "DB_HOST", "DB_PORT", "DB_DATABASE", "CACHE_DRIVER", "CACHE_STORE", "SESSION_DRIVER", "QUEUE_CONNECTION", "MAIL_MAILER", "MAIL_HOST", "MAIL_PORT":
		return false
	}
	return true
}
func readEnv(root, name string) ([]byte, error) {
	b, err := ReadRegular(filepath.Join(root, name), 128<<10)
	if os.IsNotExist(err) {
		return nil, nil
	}
	return b, err
}
func parseEnv(b []byte) (map[string]string, map[string]int, error) {
	values := map[string]string{}
	lines := map[string]int{}
	for i, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) {
			return nil, nil, fmt.Errorf("unsupported .env syntax on line %d; edit this file in your editor", i+1)
		}
		if _, ok = lines[key]; ok {
			return nil, nil, fmt.Errorf("duplicate environment key %s", key)
		}
		value = strings.TrimSpace(value)
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			quote := value[0]
			end := -1
			escaped := false
			for n := 1; n < len(value); n++ {
				if escaped {
					escaped = false
					continue
				}
				if value[n] == '\\' && quote == '"' {
					escaped = true
					continue
				}
				if value[n] == quote {
					end = n
					break
				}
			}
			if end < 0 || (strings.TrimSpace(value[end+1:]) != "" && !strings.HasPrefix(strings.TrimSpace(value[end+1:]), "#")) {
				return nil, nil, fmt.Errorf("unsupported quoted environment value on line %d", i+1)
			}
			value = value[1:end]
			if quote == '"' {
				value = strings.NewReplacer(`\"`, `"`, `\$`, `$`, `\\`, `\`).Replace(value)
			}
		} else if n := strings.Index(value, " #"); n >= 0 {
			value = strings.TrimSpace(value[:n])
		}
		values[key] = value
		lines[key] = i
	}
	return values, lines, nil
}
func ReadEnvironment(root string) (Environment, error) {
	b, err := readEnv(root, ".env")
	if err != nil {
		return Environment{}, err
	}
	values, _, err := parseEnv(b)
	if err != nil {
		return Environment{}, err
	}
	example, err := readEnv(root, ".env.example")
	if err != nil {
		return Environment{}, err
	}
	defaults, _, err := parseEnv(example)
	if err != nil {
		return Environment{}, fmt.Errorf(".env.example cannot be compared safely")
	}
	keys := map[string]bool{}
	for k := range values {
		keys[k] = true
	}
	for k := range defaults {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	result := Environment{Revision: Digest(b), Variables: []Variable{}}
	for _, k := range sorted {
		value, present := values[k]
		ex, inExample := defaults[k]
		private := secret(k)
		if private {
			value = ""
			ex = ""
		}
		result.Variables = append(result.Variables, Variable{Key: k, Value: value, Example: ex, Secret: private, Present: present, InExample: inExample})
	}
	return result, nil
}
func UpdateEnvironment(home, root, id, revision string, changes map[string]string) (Environment, error) {
	if len(changes) > 100 {
		return Environment{}, fmt.Errorf("too many environment changes")
	}
	b, err := readEnv(root, ".env")
	if err != nil {
		return Environment{}, err
	}
	if Digest(b) != revision {
		return Environment{}, fmt.Errorf("environment changed; reload before saving")
	}
	if len(changes) == 0 {
		return ReadEnvironment(root)
	}
	_, indices, err := parseEnv(b)
	if err != nil {
		return Environment{}, err
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	var keys []string
	for key, value := range changes {
		if !envKey.MatchString(key) || len(value) > 4096 || strings.ContainsAny(value, "\r\n\x00") {
			return Environment{}, fmt.Errorf("invalid environment change for %s", key)
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := changes[key]
		line := key + "='" + value + "'"
		if strings.Contains(value, "'") {
			escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`).Replace(value)
			line = key + "=\"" + escaped + "\""
		}
		if n, ok := indices[key]; ok {
			lines[n] = line
		} else {
			lines = append(lines, line)
		}
	}
	ending := "\n"
	if strings.Contains(string(b), "\r\n") {
		ending = "\r\n"
	}
	text := strings.Join(lines, ending)
	if !strings.HasSuffix(text, "\n") {
		text += ending
	}
	if len(text) > 128<<10 {
		return Environment{}, fmt.Errorf("environment file too large")
	}
	file := filepath.Join(root, ".env")
	if err = Backup(home, id, "env", file); err != nil {
		return Environment{}, err
	}
	if err = statefile.WriteBytes(file, []byte(text)); err != nil {
		return Environment{}, err
	}
	return ReadEnvironment(root)
}

// EnvironmentValues is for local runtime preparation only. Unlike the editable
// dashboard document, it is not masked and must never be returned by an API.
func EnvironmentValues(root string) (map[string]string, error) {
	b, e := readEnv(root, ".env")
	if e != nil {
		return nil, e
	}
	values, _, e := parseEnv(b)
	return values, e
}
