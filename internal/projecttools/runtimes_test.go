package projecttools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNodeInventoryReadsBothPlatformLayoutsAndKeepsRuntime(t *testing.T) {
	for _, layout := range []string{"lib/node_modules/npm", "node_modules/npm"} {
		t.Run(layout, func(t *testing.T) {
			home := t.TempDir()
			key := "node-22.23.0-linux-amd64-aaaaaaaaaaaaaaaaaaaa"
			dir := filepath.Join(home, "runtimes", key)
			pkg := filepath.Join(dir, filepath.FromSlash(layout))
			if err := os.MkdirAll(pkg, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"version":"10.9.0"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".ready"), []byte("ready"), 0600); err != nil {
				t.Fatal(err)
			}
			inventory, err := RuntimeInventory(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(inventory.Installed) != 1 {
				t.Fatalf("%+v", inventory)
			}
			entry := inventory.Installed[0]
			if entry.Kind != "node" || entry.Version != "22.23.0" || entry.NPM != "10.9.0" || !entry.Protected {
				t.Fatalf("%+v", entry)
			}
		})
	}
}
