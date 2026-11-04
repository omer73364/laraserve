package prepare

import (
	"fmt"
	"laraserve/internal/projectconfig"
	"os"
	"path/filepath"
)

// EnsureEnvironment copies the example once, without overwriting local secrets.
func EnsureEnvironment(root string) error {
	target := filepath.Join(root, ".env")
	if _, err := os.Lstat(target); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err := projectconfig.ReadRegular(filepath.Join(root, ".env.example"), 1<<20)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read .env.example: %w", err)
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create .env: %w", err)
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
