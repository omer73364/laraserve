// Package phpmyadmin prepares a private phpMyAdmin workspace for one run.
package phpmyadmin

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Workspace struct {
	Root     string
	LoginURL string
	dir      string
}

func (w *Workspace) Close() { _ = os.RemoveAll(w.dir) }

func phpString(s string) string {
	return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s) + "'"
}

// Prepare never writes credentials into the shared runtime or Laravel tree.
func Prepare(source, stateDir, baseURL string, db map[string]string) (*Workspace, error) {
	dir, err := os.MkdirTemp(stateDir, "phpmyadmin-")
	if err != nil {
		return nil, err
	}
	w := &Workspace{dir: dir, Root: filepath.Join(dir, "public")}
	ok := false
	defer func() {
		if !ok {
			w.Close()
		}
	}()
	if err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(w.Root, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("unsupported phpMyAdmin source file %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o600)
	}); err != nil {
		return nil, err
	}
	for _, name := range []string{"tmp", "sessions"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o700); err != nil {
			return nil, err
		}
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(random)
	session := "LaraServePMA" + token[:16]
	secure := "false"
	if strings.HasPrefix(baseURL, "https://") {
		secure = "true"
	}
	config := "<?php\n" +
		"$cfg['blowfish_secret'] = " + phpString(token[:32]) + ";\n" +
		"$cfg['PmaAbsoluteUri'] = " + phpString(baseURL+"/") + ";\n" +
		"$cfg['TempDir'] = " + phpString(filepath.Join(dir, "tmp")) + ";\n" +
		"$cfg['SessionSavePath'] = " + phpString(filepath.Join(dir, "sessions")) + ";\n" +
		"$cfg['AllowArbitraryServer'] = false;\n" +
		"$cfg['Servers'][1] = [\n" +
		" 'auth_type' => 'signon',\n" +
		" 'SignonSession' => " + phpString(session) + ",\n" +
		" 'SignonURL' => 'laraserve-login.php',\n" +
		" 'LogoutURL' => 'laraserve-login.php',\n" +
		" 'host' => 'localhost',\n" +
		" 'socket' => " + phpString(db["DB_SOCKET"]) + ",\n" +
		" 'only_db' => " + phpString(db["DB_DATABASE"]) + ",\n" +
		" 'AllowNoPassword' => false,\n];\n"
	login := "<?php\n" +
		"header('Cache-Control: no-store');\nheader('Referrer-Policy: no-referrer');\n" +
		"if (!isset($_GET['token']) || !is_string($_GET['token']) || !hash_equals(" + phpString(token) + ", $_GET['token'])) {\n" +
		" http_response_code(403); exit('Open the phpMyAdmin sign-in link printed by LaraServe.');\n}\n" +
		"ini_set('session.use_strict_mode', '1');\n" +
		"session_save_path(" + phpString(filepath.Join(dir, "sessions")) + ");\n" +
		"session_name(" + phpString(session) + ");\n" +
		"session_set_cookie_params(['path' => '/', 'secure' => " + secure + ", 'httponly' => true, 'samesite' => 'Strict']);\n" +
		"session_start();\nsession_regenerate_id(true);\n" +
		"$_SESSION['PMA_single_signon_user'] = " + phpString(db["DB_USERNAME"]) + ";\n" +
		"$_SESSION['PMA_single_signon_password'] = " + phpString(db["DB_PASSWORD"]) + ";\n" +
		"$_SESSION['PMA_single_signon_HMAC_secret'] = bin2hex(random_bytes(32));\n" +
		"session_write_close();\nheader('Location: " + strings.ReplaceAll(baseURL, "'", "\\'") + "/index.php');\n"
	for name, content := range map[string]string{"config.inc.php": config, "laraserve-login.php": login} {
		if err := os.WriteFile(filepath.Join(w.Root, name), []byte(content), 0o600); err != nil {
			return nil, err
		}
	}
	w.LoginURL = baseURL + "/laraserve-login.php?token=" + token
	ok = true
	return w, nil
}
