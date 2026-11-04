// Package appsettings stores private client preferences, separate from app files.
package appsettings

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"laraserve/internal/filelock"
	"laraserve/internal/projectconfig"
	"laraserve/internal/statefile"
)

type Config struct {
	LaunchAtLogin bool            `json:"launch_at_login"`
	OpenDashboard bool            `json:"open_dashboard"`
	CheckUpdates  bool            `json:"check_updates"`
	Projects      map[string]bool `json:"projects"`
}
type Document struct {
	Config   Config `json:"config"`
	Revision string `json:"revision"`
}

func Read(home string) (Document, error) {
	c := Config{OpenDashboard: true, Projects: map[string]bool{}}
	b, err := projectconfig.ReadRegular(filepath.Join(home, "preferences.json"), 128<<10)
	if os.IsNotExist(err) {
		return Document{c, projectconfig.Digest(nil)}, nil
	}
	if err != nil {
		return Document{}, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return Document{}, err
	}
	if c.Projects == nil {
		c.Projects = map[string]bool{}
	}
	return Document{c, projectconfig.Digest(b)}, nil
}
func Change(ctx context.Context, home string, fn func(*Config) error) (Document, error) {
	if err := os.MkdirAll(home, 0700); err != nil {
		return Document{}, err
	}
	release, err := filelock.Acquire(ctx, filepath.Join(home, "preferences.lock"))
	if err != nil {
		return Document{}, err
	}
	defer release()
	doc, err := Read(home)
	if err != nil {
		return doc, err
	}
	if err = fn(&doc.Config); err != nil {
		return doc, err
	}
	if err = statefile.WriteJSON(filepath.Join(home, "preferences.json"), doc.Config); err != nil {
		return doc, err
	}
	return Read(home)
}
func Save(ctx context.Context, home, executable, revision string, next Config) (Document, error) {
	release, err := filelock.Acquire(ctx, filepath.Join(home, "preferences.lock"))
	if err != nil {
		return Document{}, err
	}
	defer release()
	doc, err := Read(home)
	if err != nil {
		return doc, err
	}
	if doc.Revision != revision {
		return doc, fmt.Errorf("settings changed elsewhere; reload before saving")
	}
	previous := doc.Config
	if err = SetLogin(home, executable, next.LaunchAtLogin); err != nil {
		return doc, err
	}
	doc.Config.LaunchAtLogin = next.LaunchAtLogin
	doc.Config.OpenDashboard = next.OpenDashboard
	doc.Config.CheckUpdates = next.CheckUpdates
	if err = statefile.WriteJSON(filepath.Join(home, "preferences.json"), doc.Config); err != nil {
		_ = SetLogin(home, executable, previous.LaunchAtLogin)
		return doc, err
	}
	return Read(home)
}
