package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/projecttools"
)

type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}
type folderListing struct {
	Path      string        `json:"path"`
	Parent    string        `json:"parent"`
	Home      string        `json:"home"`
	Separator string        `json:"separator"`
	Folders   []folderEntry `json:"folders"`
	Truncated bool          `json:"truncated"`
}

// Folder navigation is a deliberately initiated, authenticated, CSRF-protected
// POST. Only directory names are returned; file contents are never exposed.
func browseFolders(path string, hidden bool) (folderListing, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return folderListing{}, err
	}
	if path == "" {
		path = home
	}
	if !filepath.IsAbs(path) {
		return folderListing{}, fmt.Errorf("enter an absolute folder path")
	}
	root, err := projects.Canonical(path)
	if err != nil {
		return folderListing{}, fmt.Errorf("cannot open folder: %w", err)
	}
	f, err := os.Open(root)
	if err != nil {
		return folderListing{}, err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil || !stat.IsDir() {
		return folderListing{}, fmt.Errorf("choose a folder, not a file")
	}
	result := folderListing{Path: root, Parent: filepath.Dir(root), Home: home, Separator: string(filepath.Separator), Folders: []folderEntry{}}
	// Bound both scanning and the response, even for an unusually large directory.
	for scanned := 0; ; {
		entries, readErr := f.ReadDir(256)
		for _, entry := range entries {
			scanned++
			if entry.IsDir() && (hidden || !strings.HasPrefix(entry.Name(), ".")) {
				if len(result.Folders) == 300 {
					result.Truncated = true
					break
				}
				result.Folders = append(result.Folders, folderEntry{Name: entry.Name(), Path: filepath.Join(root, entry.Name())})
			}
		}
		if result.Truncated {
			break
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return folderListing{}, readErr
		}
		if scanned >= 8192 {
			result.Truncated = true
			break
		}
	}
	sort.Slice(result.Folders, func(i, j int) bool {
		return strings.ToLower(result.Folders[i].Name) < strings.ToLower(result.Folders[j].Name)
	})
	return result, nil
}

// Framework metadata is read without booting the app or reading its environment.
// A locked version is distinguished from the fallback Composer constraint.
func laravelVersion(root string) (string, string) {
	if b, err := projectconfig.ReadRegular(filepath.Join(root, "composer.lock"), 4<<20); err == nil {
		var lock struct {
			Packages []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"packages"`
		}
		if json.Unmarshal(b, &lock) == nil {
			for _, p := range lock.Packages {
				if p.Name == "laravel/framework" && p.Version != "" {
					return p.Version, "composer.lock"
				}
			}
		}
	}
	if b, err := projectconfig.ReadRegular(filepath.Join(root, "composer.json"), 1<<20); err == nil {
		var composer struct {
			Require map[string]string `json:"require"`
		}
		if json.Unmarshal(b, &composer) == nil && composer.Require["laravel/framework"] != "" {
			return composer.Require["laravel/framework"], "composer.json constraint"
		}
	}
	return "", "unavailable"
}

func (s *Server) workspaceTools(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/folders":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Path   string `json:"path"`
			Hidden bool   `json:"hidden"`
		}
		if err := decode(w, r, &input); err != nil {
			http.Error(w, "invalid folder request", 400)
			return true
		}
		result, err := browseFolders(input.Path, input.Hidden)
		if err != nil {
			http.Error(w, err.Error(), 400)
		} else {
			respond(w, result)
		}
		return true
	case "/api/projects/add":
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return true
		}
		var input struct {
			Path string `json:"path"`
		}
		if err := decode(w, r, &input); err != nil {
			http.Error(w, "invalid project folder", 400)
			return true
		}
		p, err := projecttools.RegisterExisting(s.home, input.Path)
		if err != nil {
			http.Error(w, err.Error(), 400)
		} else {
			respond(w, p)
		}
		return true
	}
	return false
}
