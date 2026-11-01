// Package manifest records every change laraserve makes outside its own
// home directory, so that clean/uninstall can reverse all of them.
package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"sort"
)

// CAEntry describes the local CA installed in the OS trust store.
type CAEntry struct {
	SHA256   string   `json:"sha256"`
	SHA1     string   `json:"sha1"`
	Method   string   `json:"method"`             // linux | macos | windows
	Location string   `json:"location,omitempty"` // file placed in system anchors dir (linux)
	Update   []string `json:"update,omitempty"`   // command that refreshes the system store (linux)
	NSSDBs   []string `json:"nss_dbs,omitempty"`  // NSS databases (Firefox/Chrome on Linux)
}

// Project tracks per-project footprint.
type Project struct {
	Path      string   `json:"path"`
	Vendor    bool     `json:"vendor_created"`
	Workspace bool     `json:"workspace"`
	Domains   []string `json:"domains,omitempty"`
}

type Manifest struct {
	path     string
	CA       *CAEntry            `json:"ca,omitempty"`
	Hosts    []string            `json:"hosts,omitempty"`
	Projects map[string]*Project `json:"projects"`
}

func Load(path string) (*Manifest, error) {
	m := &Manifest{path: path, Projects: map[string]*Project{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, m); err != nil {
		return nil, err
	}
	if m.Projects == nil {
		m.Projects = map[string]*Project{}
	}
	return m, nil
}

func (m *Manifest) Save() error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, b, 0o600)
}

func (m *Manifest) Project(id, path string) *Project {
	p, ok := m.Projects[id]
	if !ok {
		p = &Project{Path: path}
		m.Projects[id] = p
	}
	return p
}

func (m *Manifest) AddHost(d string) {
	for _, h := range m.Hosts {
		if h == d {
			return
		}
	}
	m.Hosts = append(m.Hosts, d)
	sort.Strings(m.Hosts)
}

func (m *Manifest) RemoveHost(d string) {
	out := m.Hosts[:0]
	for _, h := range m.Hosts {
		if h != d {
			out = append(out, h)
		}
	}
	m.Hosts = out
}

func (m *Manifest) AddDomain(p *Project, d string) {
	for _, x := range p.Domains {
		if x == d {
			return
		}
	}
	p.Domains = append(p.Domains, d)
}
