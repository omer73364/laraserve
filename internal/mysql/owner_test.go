package mysql

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveAndUnknownDatabaseLocksArePreserved(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "running.lock")
	os.Mkdir(lock, 0700)
	if err := acquireStateDirectory(context.Background(), lock); err == nil {
		t.Fatal("unknown legacy lock removed")
	}
	if err := recordOwner(lock, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := acquireStateDirectory(ctx, lock); err == nil {
		t.Fatal("live or unverifiable process lock removed")
	}
	if _, err := os.Stat(filepath.Join(lock, "process.json")); err != nil {
		t.Fatal("lost ownership record")
	}
}
func TestExitedDatabaseOwnerLockRecoversWithoutDataDeletion(t *testing.T) {
	base, env := fakeRuntime(t)
	command := exec.Command(filepath.Join(base, "bin", "mysqld"), "--initialize-insecure")
	command.Env = env
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(base, "running.lock")
	os.Mkdir(lock, 0700)
	if err := recordOwner(lock, command.Process.Pid); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(base, "persistent-data")
	os.WriteFile(data, []byte("keep database"), 0600)
	if err := acquireStateDirectory(context.Background(), lock); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(data)
	if err != nil || string(b) != "keep database" {
		t.Fatal("database modified", err)
	}
}
