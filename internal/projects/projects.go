// Package projects stores private, per-project registry records, independent of
// the legacy footprint manifest. Separate records avoid lost concurrent writes.
package projects

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"laraserve/internal/filelock"
	"laraserve/internal/paths"
	"laraserve/internal/statefile"
)

type Project struct {
	Removed  bool     `json:"removed,omitempty"`
	Schema   int      `json:"schema"`
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`
	Root     string   `json:"root"`
	Args     []string `json:"serve_args,omitempty"`
	Domain   string   `json:"domain,omitempty"`
	Port     int      `json:"port,omitempty"`
	HTTPS    bool     `json:"https"`
	PHP      string   `json:"php,omitempty"`
	MySQL    string   `json:"mysql,omitempty"`
	Database string   `json:"database,omitempty"`
	Updated  string   `json:"updated_utc"`
}

type Store struct{ Dir string }

var idPattern = regexp.MustCompile(`^[a-f0-9]{12}$`)

func New(home string) *Store { return &Store{Dir: filepath.Join(home, "registry")} }
func Canonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		real = strings.ToLower(real)
	}
	return filepath.Clean(real), nil
}
func Identity(root string) (string, error) {
	canonical, err := Canonical(root)
	if err != nil {
		return "", err
	}
	return paths.ProjectID(canonical), nil
}
func (s *Store) File(id string) string { return filepath.Join(s.Dir, id+".json") }
func (s *Store) Save(p *Project) error {
	if !idPattern.MatchString(p.ID) {
		return fmt.Errorf("invalid project identifier")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	release, err := filelock.Acquire(context.Background(), filepath.Join(s.Dir, p.ID+".record.lock"))
	if err != nil {
		return err
	}
	defer release()
	return s.save(p)
}
func (s *Store) save(p *Project) error {
	p.Schema = 1
	p.Updated = time.Now().UTC().Format(time.RFC3339)
	return WriteJSON(s.File(p.ID), p)
}
func (s *Store) Load(id string) (*Project, error) {
	if !idPattern.MatchString(id) {
		return nil, fmt.Errorf("invalid project identifier")
	}
	b, err := os.ReadFile(s.File(id))
	if err != nil {
		return nil, err
	}
	var p Project
	if err = json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	if p.Schema < 0 || p.Schema > 1 {
		return nil, fmt.Errorf("unsupported project registry schema")
	}
	if p.ID != id {
		return nil, fmt.Errorf("registry project identity does not match filename")
	}
	return &p, nil
}
func (s *Store) List() ([]*Project, error) { return s.list(false) }
func (s *Store) All() ([]*Project, error)  { return s.list(true) }
func (s *Store) list(includeRemoved bool) ([]*Project, error) {
	files, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result []*Project
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			id := strings.TrimSuffix(file.Name(), ".json")
			if !idPattern.MatchString(id) {
				continue
			}
			p, err := s.Load(id)
			if err != nil {
				return nil, fmt.Errorf("read project %s: %w", id, err)
			}
			if includeRemoved || !p.Removed {
				result = append(result, p)
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}
func (s *Store) Find(target string) (*Project, error) {
	if idPattern.MatchString(target) {
		if p, err := s.Load(target); err == nil && !p.Removed {
			return p, nil
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	canonical, err := Canonical(target)
	list, listErr := s.List()
	if listErr != nil {
		return nil, listErr
	}
	var matches []*Project
	for _, p := range list {
		if err == nil && (p.Root == canonical || p.Source == canonical) {
			return p, nil
		}
		if p.Name == target {
			matches = append(matches, p)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("project name %q is ambiguous; use its ID or full path", target)
	}
	return nil, fmt.Errorf("project %q is not registered; run serve or start with its path", target)
}

func WriteJSON(path string, value any) error { return statefile.WriteJSON(path, value) }

func (s *Store) Import(p *Project) error {
	if !idPattern.MatchString(p.ID) {
		return fmt.Errorf("invalid project identifier")
	}
	if err := os.MkdirAll(s.Dir, 0700); err != nil {
		return err
	}
	release, err := filelock.Acquire(context.Background(), filepath.Join(s.Dir, p.ID+".record.lock"))
	if err != nil {
		return err
	}
	defer release()
	if _, err = s.Load(p.ID); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return s.save(p)
}

func ValidID(id string) bool { return idPattern.MatchString(id) }

// Archive keeps a reversible registration tombstone so legacy footprint imports
// cannot silently re-add a removed project. No source or private data is deleted.
func (s *Store) Archive(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid project identifier")
	}
	release, err := filelock.Acquire(context.Background(), filepath.Join(s.Dir, id+".record.lock"))
	if err != nil {
		return err
	}
	defer release()
	p, err := s.Load(id)
	if err != nil {
		return err
	}
	p.Removed = true
	return s.save(p)
}

// ReusableArgs excludes one-shot consent and schema-changing options from all
// dashboard/login restarts. Project files still control the reusable defaults.
func ReusableArgs(p *Project) []string {
	args := []string{p.Source}
	for i := 1; i < len(p.Args); i++ {
		arg := p.Args[i]
		key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		switch key {
		case "migrate", "yes", "y", "allow-insecure-dependencies":
			if !strings.Contains(arg, "=") && i+1 < len(p.Args) && (p.Args[i+1] == "true" || p.Args[i+1] == "false") {
				i++
			}
			continue
		}
		args = append(args, arg)
	}
	return args
}
