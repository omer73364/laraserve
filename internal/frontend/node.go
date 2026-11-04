package frontend

import (
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
	"laraserve/internal/versions"
	"os"
	"path/filepath"
	"strings"
)

func Select(cache, root string) (*runtime.NodeRuntime, error) {
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	available, err := runtime.NodeVersions(cache)
	if err != nil {
		return nil, err
	}
	lockFile := filepath.Join(cache, "node-project-"+paths.ProjectID(root)+".json")
	lock, err := versions.ReadLock(lockFile)
	if err != nil {
		return nil, err
	}
	selected, err := versions.ProjectNode(root, available, "", lock.Node, runtime.NodeLTSVersions(cache)...)
	if err != nil {
		return nil, err
	}
	node, err := runtime.EnsureNode(cache, selected)
	if err != nil {
		return nil, err
	}
	if err = versions.WriteLock(lockFile, versions.Pins{Node: selected}); err != nil {
		return nil, err
	}
	return node, nil
}
func Environment(node *runtime.NodeRuntime, base []string) []string {
	result := make([]string, 0, len(base)+1)
	path := filepath.Dir(node.Node)
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "PATH") {
			path += string(os.PathListSeparator) + value
			continue
		}
		result = append(result, entry)
	}
	return append(result, "PATH="+path)
}
