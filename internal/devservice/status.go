package devservice

import (
	"encoding/json"
	"os"
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
	for i := range snapshot.Services {
		if snapshot.Generation != generation || (owner != "running" && owner != "starting") {
			snapshot.Services[i].State = "stopped"
			if owner == "crashed" || owner == "failed" {
				snapshot.Services[i].State = owner
			}
		}
	}
	return snapshot.Services
}

// RemoveHotFile only removes the Vite marker written by this process's URL.
// Laravel must not keep referencing a development server after a normal stop.
func RemoveHotFile(root, url string) {
	file := filepath.Join(root, "public", "hot")
	b, err := projectconfig.ReadRegular(file, 4096)
	if err == nil && string(b) == url {
		_ = os.Remove(file)
	}
}
