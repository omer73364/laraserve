package cli

import (
	"fmt"
	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/projects"
	"path/filepath"
)

func projectStore(home *paths.Home) (*projects.Store, error) {
	store := projects.New(home.Root)
	legacy, err := manifest.Load(home.Manifest())
	if err != nil {
		return nil, err
	}
	for _, footprint := range legacy.Projects {
		// Old workspaces lack the original source and cannot safely be imported.
		if footprint.Workspace {
			continue
		}
		source, err := projects.Canonical(footprint.Path)
		if err != nil {
			continue
		}
		p := &projects.Project{ID: paths.ProjectID(source), Name: filepath.Base(source), Source: source, Root: source, Args: []string{source}, HTTPS: true}
		if len(footprint.Domains) > 0 {
			p.Domain = footprint.Domains[len(footprint.Domains)-1]
		}
		if err = store.Import(p); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func projectStateID(m *manifest.Manifest, root string, workspace bool) (string, error) {
	canonical, err := projects.Canonical(root)
	if err != nil {
		return "", err
	}
	matched := ""
	for id, p := range m.Projects {
		if p.Workspace != workspace {
			continue
		}
		previous, err := projects.Canonical(p.Path)
		if err != nil || previous != canonical {
			continue
		}
		if !projects.ValidID(id) {
			return "", fmt.Errorf("invalid legacy project state ID")
		}
		if matched != "" && matched != id {
			return "", fmt.Errorf("multiple legacy states refer to this folder (%s and %s); preserve both and resolve the duplicate before starting", matched, id)
		}
		matched = id
	}
	if matched != "" {
		return matched, nil
	}
	return paths.ProjectID(root), nil
}
