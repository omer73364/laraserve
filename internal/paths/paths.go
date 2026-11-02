// Package paths defines the single directory laraserve owns on the host.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

type Home struct{ Root string }

// New returns $LARASERVE_HOME or ~/.laraserve and creates it.
func New() (*Home, error) {
	root := os.Getenv("LARASERVE_HOME")
	if root == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(h, ".laraserve")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Home{Root: root}, nil
}

func (h *Home) Runtimes() string   { return filepath.Join(h.Root, "runtimes") }
func (h *Home) CA() string         { return filepath.Join(h.Root, "ca") }
func (h *Home) Projects() string   { return filepath.Join(h.Root, "projects") }
func (h *Home) Workspaces() string { return filepath.Join(h.Root, "workspaces") }
func (h *Home) Manifest() string   { return filepath.Join(h.Root, "manifest.json") }

// ProjectID is a stable short id for an absolute project path.
func ProjectID(abs string) string {
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12]
}
