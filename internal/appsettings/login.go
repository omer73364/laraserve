package appsettings

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"laraserve/internal/paths"
	"laraserve/internal/statefile"
)

func loginName(home string) string { return "laraserve-" + paths.ProjectID(home) }
func LoginPath(home string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		dir, err := os.UserConfigDir()
		return filepath.Join(dir, "autostart", loginName(home)+".desktop"), err
	case "darwin":
		dir, err := os.UserHomeDir()
		return filepath.Join(dir, "Library", "LaunchAgents", loginName(home)+".plist"), err
	case "windows":
		return `HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run\` + loginName(home), nil
	}
	return "", fmt.Errorf("login startup is unavailable on %s", runtime.GOOS)
}
func desktopQuote(s string) string {
	// Desktop Exec is not a shell. Escape its reserved characters, then the
	// Desktop Entry string escaping layer. Percent signs must not become field codes.
	s = strings.ReplaceAll(s, "%", "%%")
	for _, c := range []string{"\\", "\"", "`", "$"} {
		s = strings.ReplaceAll(s, c, "\\"+c)
	}
	return strings.ReplaceAll("\""+s+"\"", "\\", "\\\\")
}
func LoginContent(platform, executable, home string) (string, error) {
	if !filepath.IsAbs(executable) || !filepath.IsAbs(home) || strings.ContainsAny(executable+home, "\r\n\x00") {
		return "", fmt.Errorf("startup requires stable absolute executable and data paths")
	}
	switch platform {
	case "linux":
		return "[Desktop Entry]\nType=Application\nName=LaraServe\nComment=Start your local development workspace\nExec=" + desktopQuote(executable) + " __autostart --home " + desktopQuote(home) + "\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", nil
	case "darwin":
		esc := func(s string) string { var b strings.Builder; _ = xml.EscapeText(&b, []byte(s)); return b.String() }
		return `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict><key>Label</key><string>` + loginName(home) + `</string><key>ProgramArguments</key><array><string>` + esc(executable) + `</string><string>__autostart</string><string>--home</string><string>` + esc(home) + `</string></array><key>RunAtLoad</key><true/></dict></plist>`, nil
	case "windows":
		return "", fmt.Errorf("Windows uses the current-user Run registry value")
	}
	return "", fmt.Errorf("unsupported startup platform")
}
func SetLogin(home, executable string, enabled bool) error {
	if handled, err := platformLogin(home, executable, enabled); handled {
		return err
	}
	path, err := LoginPath(home)
	if err != nil {
		return err
	}
	if !enabled {
		err = os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content, err := LoginContent(runtime.GOOS, executable, home)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return statefile.WriteBytes(path, []byte(content))
}
