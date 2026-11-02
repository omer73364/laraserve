package phpmyadmin

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrivateWorkspace(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "index.php"), []byte("<?php echo 'phpMyAdmin';"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	password := "secret's\\password"
	w, err := Prepare(source, state, "https://localhost:8444", map[string]string{
		"DB_SOCKET": "/private/mysql.sock", "DB_DATABASE": "laraserve", "DB_USERNAME": "laraserve", "DB_PASSWORD": password,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	u, err := url.Parse(w.LoginURL)
	if err != nil || len(u.Query().Get("token")) != 64 || strings.Contains(w.LoginURL, password) {
		t.Fatal("invalid private sign-in URL")
	}
	for _, name := range []string{"config.inc.php", "laraserve-login.php"} {
		st, err := os.Stat(filepath.Join(w.Root, name))
		if err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
			t.Fatal("generated credential file is not private")
		}
		if php, err := exec.LookPath("php"); err == nil {
			if out, err := exec.Command(php, "-l", filepath.Join(w.Root, name)).CombinedOutput(); err != nil {
				t.Fatalf("invalid generated PHP: %v %s", err, out)
			}
		}
	}
	b, _ := os.ReadFile(filepath.Join(w.Root, "laraserve-login.php"))
	for _, want := range []string{"hash_equals(", "session_regenerate_id(true)", "'secure' => true", "'httponly' => true", phpString(password)} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("missing sign-in protection %s", want)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "config.inc.php")); !os.IsNotExist(err) {
		t.Fatal("shared runtime was modified")
	}
	w.Close()
	if _, err := os.Stat(w.dir); !os.IsNotExist(err) {
		t.Fatal("temporary credentials were not removed")
	}
}
