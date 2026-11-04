package runtime

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentCachePublishesOnce(t *testing.T) {
	root := t.TempDir()
	var populated atomic.Int32
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := installCache(root, "shared", func(d string) error { _, err := os.Stat(filepath.Join(d, "tool")); return err }, func(d string) error {
				populated.Add(1)
				return os.WriteFile(filepath.Join(d, "tool"), []byte("complete"), 0600)
			})
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if populated.Load() != 1 {
		t.Fatalf("concurrent downloads populated %d times", populated.Load())
	}
}

func TestBrokenReadyCacheIsRepairedWithoutTouchingDatabaseData(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tool")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, ".ready"), []byte("ready"), 0600)
	data := filepath.Join(root, "persistent-database")
	os.WriteFile(data, []byte("keep my data"), 0600)
	_, err := installCache(root, "tool", func(d string) error { _, err := os.Stat(filepath.Join(d, "binary")); return err }, func(d string) error { return os.WriteFile(filepath.Join(d, "binary"), []byte("repaired"), 0700) })
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(data)
	if err != nil || string(b) != "keep my data" {
		t.Fatal("repair touched database state", err)
	}
}
