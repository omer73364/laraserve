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
