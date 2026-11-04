package projecttools

import (
	"encoding/json"
	"laraserve/internal/projectconfig"
	"os"
	"sort"
	"strings"
)

// ListTasks returns bounded metadata only. Output is read for a selected task.
func ListTasks(home, projectID string) ([]Task, error) {
	entries, err := os.ReadDir(taskDir(home))
	if os.IsNotExist(err) {
		return []Task{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Task{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") || strings.Contains(entry.Name(), "-") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if len(id) != 12 {
			continue
		}
		b, e := projectconfig.ReadRegular(taskFile(home, id), 16<<10)
		if e != nil {
			continue
		}
		var task Task
		if json.Unmarshal(b, &task) != nil || task.ID != id || projectID != "" && task.ProjectID != projectID {
			continue
		}
		task.Output = ""
		task.Progress.Steps = nil
		result = append(result, task)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Started > result[j].Started })
	if len(result) > 50 {
		result = result[:50]
	}
	return result, nil
}
