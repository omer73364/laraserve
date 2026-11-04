package prepare

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentCopyPreservesExistingSecrets(t *testing.T) {
	root := t.TempDir()
	example := filepath.Join(root, ".env.example")
	target := filepath.Join(root, ".env")
	os.WriteFile(example, []byte("APP_NAME=Example\n"), 0600)
	if e := EnsureEnvironment(root); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(target)
	if string(b) != "APP_NAME=Example\n" {
		t.Fatal(string(b))
	}
	os.WriteFile(target, []byte("APP_KEY=keep-secret\n"), 0600)
	if e := EnsureEnvironment(root); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(target)
	if string(b) != "APP_KEY=keep-secret\n" {
		t.Fatal("existing environment overwritten")
	}
}
