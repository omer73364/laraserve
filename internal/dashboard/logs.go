package dashboard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"laraserve/internal/manifest"
	"laraserve/internal/projects"
)

const logLimit = 256 << 10

type LogResult struct {
	Source    string `json:"source"`
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
	Note      string `json:"note"`
}

func (s *Server) logs(p *projects.Project, source, query string) (LogResult, error) {
	result := LogResult{Source: source, Note: "Recent log tail only. Logs can contain application secrets; keep this session private."}
	if len(query) > 256 {
		return result, fmt.Errorf("search is limited to 256 characters")
	}
	stateID, err := s.privateID(p)
	if err != nil {
		return result, err
	}
	var files []string
	var base string
	switch source {
	case "startup":
		base = s.home
		files = []string{filepath.Join(s.home, "control", p.ID+"-startup.log")}
	case "server", "php":
		base = s.home
		files = []string{filepath.Join(s.home, "projects", stateID, "logs", "server.log")}
		result.Note += " PHP and web server output share this stream."
	case "vite", "queue", "scheduler", "redis", "mailpit":
		base = s.home
		files = []string{filepath.Join(s.home, "projects", stateID, "logs", source+".log")}
	case "mysql":
		base = s.home
		if p.MySQL != "" && !strings.ContainsAny(p.MySQL, "/\\") {
			root := filepath.Join(s.home, "databases", stateID)
			file := filepath.Join(root, "mysql-"+p.MySQL, "server.log")
			if p.MySQL == "8.4.13" {
				if st, e := os.Stat(filepath.Join(root, "data")); e == nil && st.IsDir() {
					file = filepath.Join(root, "server.log")
				}
			}
			files = []string{file}
		}
	case "laravel":
		base = p.Root
		logDir := filepath.Join(p.Root, "storage", "logs")
		entries, e := os.ReadDir(logDir)
		if e != nil && !os.IsNotExist(e) {
			return result, fmt.Errorf("cannot read Laravel log directory")
		}
		type candidate struct {
			path string
			time int64
		}
		var candidates []candidate
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
				continue
			}
			info, e := entry.Info()
			if e == nil {
				candidates = append(candidates, candidate{filepath.Join(logDir, entry.Name()), info.ModTime().UnixNano()})
			}
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].time > candidates[j].time })
		if len(candidates) > 8 {
			candidates = candidates[:8]
			result.Truncated = true
		}
		for i := len(candidates) - 1; i >= 0; i-- {
			files = append(files, candidates[i].path)
		}
	default:
		return result, fmt.Errorf("unknown log source")
	}
	budget := logLimit
	var chunks []string
	for i := len(files) - 1; i >= 0 && budget > 0; i-- {
		text, cut, e := readTail(base, files[i], budget)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return result, fmt.Errorf("cannot safely read selected log")
		}
		result.Truncated = result.Truncated || cut
		budget -= len(text)
		chunks = append([]string{text}, chunks...)
	}
	text := strings.Join(chunks, "\n")
	// Render plain text, strip terminal control sequences, and bound scanning.
	text = cleanLog(text)
	if len(text) > logLimit {
		text = strings.ToValidUTF8(text[len(text)-logLimit:], "")
		result.Truncated = true
	}
	if query != "" {
		var matched []string
		for _, line := range strings.Split(text, "\n") {
			if strings.Contains(strings.ToLower(line), strings.ToLower(query)) {
				matched = append(matched, line)
			}
		}
		text = strings.Join(matched, "\n")
	}
	result.Text = text
	return result, nil
}
func (s *Server) privateID(p *projects.Project) (string, error) {
	m, err := manifest.Load(filepath.Join(s.home, "manifest.json"))
	if err != nil {
		return "", fmt.Errorf("cannot read project footprint")
	}
	root, err := projects.Canonical(p.Root)
	if err != nil {
		return p.ID, nil
	}
	id := ""
	for key, entry := range m.Projects {
		path, e := projects.Canonical(entry.Path)
		if e != nil || path != root {
			continue
		}
		if !projects.ValidID(key) {
			return "", fmt.Errorf("invalid project footprint")
		}
		if id != "" && id != key {
			return "", fmt.Errorf("multiple private states match this project")
		}
		id = key
	}
	if id == "" {
		id = p.ID
	}
	return id, nil
}
func readTail(base, path string, limit int) (string, bool, error) {
	// Resolve both paths and reject symlinks escaping the allowlisted tree.
	root, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", false, err
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, err
	}
	rel, err := filepath.Rel(root, real)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("log escapes its root")
	}
	before, err := os.Stat(real)
	if err != nil {
		return "", false, err
	}
	if !before.Mode().IsRegular() {
		return "", false, fmt.Errorf("log is not a regular file")
	}
	file, err := os.Open(real)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return "", false, err
	}
	if !stat.Mode().IsRegular() || !os.SameFile(before, stat) {
		return "", false, fmt.Errorf("log is not a regular file")
	}
	start := stat.Size() - int64(limit)
	cut := start > 0
	if start < 0 {
		start = 0
	}
	if _, err = file.Seek(start, io.SeekStart); err != nil {
		return "", false, err
	}
	b, err := io.ReadAll(io.LimitReader(file, int64(limit)))
	if cut {
		if i := strings.IndexByte(string(b), '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return string(b), cut, err
}
func cleanLog(text string) string {
	var b strings.Builder
	escaped := false
	for _, r := range text {
		if r == 27 {
			escaped = true
			continue
		}
		if escaped {
			if r >= '@' && r <= '~' && r != '[' {
				escaped = false
			}
			continue
		}
		if !unicode.IsControl(r) || r == '\n' || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
