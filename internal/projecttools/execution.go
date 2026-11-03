package projecttools

import (
	"encoding/json"
	"os"
	"path/filepath"

	"laraserve/internal/runtime"
	"laraserve/internal/statefile"
)

type Execution struct {
	Generation string
	Root       string
	StateDir   string
	Runtime    runtime.Runtime
	Env        []string
}

func SaveExecution(home, id string, e Execution) error {
	dir := filepath.Join(home, "execution")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return statefile.WriteJSON(filepath.Join(dir, id+".json"), e)
}
func execution(home, id string) (Execution, error) {
	var e Execution
	b, err := os.ReadFile(filepath.Join(home, "execution", id+".json"))
	if err != nil {
		return e, err
	}
	err = json.Unmarshal(b, &e)
	return e, err
}
