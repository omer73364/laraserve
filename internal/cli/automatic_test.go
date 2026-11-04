package cli

import (
	"laraserve/internal/projectconfig"
	"os"
	"path/filepath"
	"testing"
)

func TestAutomaticServicesRespectsRemoteInfrastructureAndOptOut(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ".env"), []byte("CACHE_STORE=redis\nREDIS_HOST=127.0.0.1\nMAIL_MAILER=smtp\nMAIL_HOST=mailpit\nMAIL_PORT=1025\n"), 0600)
	var c projectconfig.Config
	automaticServices(root, &c)
	if !c.Services.Redis || !c.Services.Mailpit || c.Services.Queue {
		t.Fatal(c)
	}
	os.WriteFile(filepath.Join(root, ".env"), []byte("CACHE_STORE=redis\nREDIS_HOST=production.example\nMAIL_MAILER=smtp\nMAIL_HOST=smtp.example\nMAIL_PORT=1025\n"), 0600)
	c = projectconfig.Config{}
	automaticServices(root, &c)
	if c.Services.Redis || c.Services.Mailpit {
		t.Fatal("remote connection replaced")
	}
	off := false
	c = projectconfig.Config{Services: projectconfig.Services{AutoDetect: &off}}
	automaticServices(root, &c)
	if c.Services.Redis || c.Services.Mailpit {
		t.Fatal("opt-out ignored")
	}
}
