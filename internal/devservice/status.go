package devservice

import (
	"encoding/json"
	"path/filepath"

	"laraserve/internal/projectconfig"
)

type Status struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}
type Snapshot struct {
	Generation string   `json:"generation"`
	Services   []Status `json:"services"`
}

func ReadStatus(stateDir, generation, owner string) []Status {
	b, err := projectconfig.ReadRegular(filepath.Join(stateDir, "services.json"), 32<<10)
	if err != nil {
		return nil
	}
	var snapshot Snapshot
	if json.Unmarshal(b, &snapshot) != nil {
		return nil
	}
	if snapshot.Generation != generation {
		return nil
	}
	for i := range snapshot.Services {
		switch owner {
		case "running", "starting":
		case "stopped", "stopping", "crashed", "failed":
			snapshot.Services[i].State = owner
		default:
			snapshot.Services[i].State = "unknown"
		}
	}
	return snapshot.Services
}
