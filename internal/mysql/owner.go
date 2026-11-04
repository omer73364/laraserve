package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"laraserve/internal/process"
	"laraserve/internal/statefile"
)

type databaseOwner struct {
	PID int `json:"pid"`
}

func recordOwner(lock string, pid int) error {
	return statefile.WriteJSON(filepath.Join(lock, "process.json"), databaseOwner{pid})
}

// Only called while holding the database's OS owner lock. A recorded PID is
// checked for liveness, never killed. Unknown/legacy locks remain untouched.
func acquireStateDirectory(ctx context.Context, lock string) error {
	if err := os.Mkdir(lock, 0700); err == nil {
		return nil
	} else if !os.IsExist(err) {
		return err
	}
	b, err := os.ReadFile(filepath.Join(lock, "process.json"))
	if err != nil {
		return fmt.Errorf("database has an unverified legacy or incomplete lock at %s; verify the old server has stopped before removing this lock", lock)
	}
	var owner databaseOwner
	if err = json.Unmarshal(b, &owner); err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		alive, err := process.Alive(owner.PID)
		if err != nil {
			return fmt.Errorf("cannot verify previous database owner; lock retained: %w", err)
		}
		if !alive {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("previous database owner is still shutting down or its PID was reused; lock retained")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if err = os.Remove(filepath.Join(lock, "process.json")); err != nil {
		return err
	}
	// Refuse unknown files rather than recursively removing the directory.
	if err = os.Remove(lock); err != nil {
		return err
	}
	return os.Mkdir(lock, 0700)
}
func releaseStateDirectory(lock string) {
	_ = os.Remove(filepath.Join(lock, "process.json"))
	_ = os.Remove(lock)
}
