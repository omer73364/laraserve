// Package activity records named preparation stages independently of verbose logs.
package activity

import (
	"encoding/json"
	"fmt"
	"laraserve/internal/projectconfig"
	"laraserve/internal/statefile"
	"sync"
	"time"
)

type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}
type Progress struct {
	Steps   []Step `json:"steps"`
	Error   string `json:"error,omitempty"`
	Updated string `json:"updated"`
}

var current struct {
	sync.Mutex
	file     string
	progress Progress
}

func SetFile(file string) {
	current.Lock()
	defer current.Unlock()
	current.file = file
	current.progress = Progress{Steps: []Step{}}
	save()
}
func save() {
	current.progress.Updated = time.Now().UTC().Format(time.RFC3339Nano)
	if current.file != "" {
		_ = statefile.WriteJSON(current.file, current.progress)
	}
}
func Plan(names ...string) {
	current.Lock()
	defer current.Unlock()
	current.progress = Progress{Steps: []Step{}}
	for _, name := range names {
		current.progress.Steps = append(current.progress.Steps, Step{Name: name, Status: "pending"})
	}
	save()
}
func Start(name string) {
	current.Lock()
	defer current.Unlock()
	found := false
	for i := range current.progress.Steps {
		step := &current.progress.Steps[i]
		if step.Status == "running" {
			step.Status = "completed"
		}
		if step.Name == name {
			step.Status = "running"
			found = true
		}
	}
	if !found {
		current.progress.Steps = append(current.progress.Steps, Step{Name: name, Status: "running"})
	}
	fmt.Printf("\n  [step] %s\n", name)
	save()
}
func Detail(text string) {
	current.Lock()
	defer current.Unlock()
	for i := range current.progress.Steps {
		if current.progress.Steps[i].Status == "running" {
			current.progress.Steps[i].Detail = text
		}
	}
	save()
}
func Skip(name, reason string) {
	current.Lock()
	defer current.Unlock()
	for i := range current.progress.Steps {
		if current.progress.Steps[i].Name == name {
			current.progress.Steps[i].Status = "skipped"
			current.progress.Steps[i].Detail = reason
		}
	}
	save()
}
func Finish(err error) {
	current.Lock()
	defer current.Unlock()
	if err != nil {
		current.progress.Error = err.Error()
	}
	for i := range current.progress.Steps {
		step := &current.progress.Steps[i]
		if step.Status == "running" {
			if err != nil {
				step.Status = "failed"
				step.Detail = err.Error()
			} else {
				step.Status = "completed"
			}
		}
	}
	save()
}
func Read(file string) Progress {
	var p Progress
	b, err := projectconfig.ReadRegular(file, 128<<10)
	if err == nil {
		_ = json.Unmarshal(b, &p)
	}
	return p
}
