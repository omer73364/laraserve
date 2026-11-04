// Package elevate uses the operating system's authentication UI. Passwords never
// pass through the dashboard, logs, preferences, or process arguments.
package elevate

import (
	"fmt"
	"golang.org/x/term"
	"laraserve/internal/activity"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func Command(name string, args ...string) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" && name == "cp" && len(args) == 2 {
		activity.Detail("Approve the Windows administrator authorization dialog to continue")
		return windowsCopy(args[0], args[1]), nil
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return exec.Command(name, args...), nil
	}
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return exec.Command("sudo", append([]string{"--", name}, args...)...), nil
	}
	if os.Getenv("SUDO_ASKPASS") != "" {
		return exec.Command("sudo", append([]string{"-A", "--", name}, args...)...), nil
	}
	if runtime.GOOS == "darwin" {
		command := quote(name)
		for _, arg := range args {
			command += " " + quote(arg)
		}
		activity.Detail("Approve the macOS administrator authorization dialog to continue")
		return exec.Command("osascript", "-e", "do shell script "+appleString(command)+" with administrator privileges"), nil
	}
	if _, err := exec.LookPath("pkexec"); err == nil && (os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "") {
		activity.Detail("Approve the system authorization dialog to continue")
		return exec.Command("pkexec", append([]string{name}, args...)...), nil
	}
	// Cached sudo credentials need no password dialog and work in headless sessions.
	if err := exec.Command("sudo", "-n", "true").Run(); err == nil {
		return exec.Command("sudo", append([]string{"-n", "--", name}, args...)...), nil
	}
	return nil, fmt.Errorf("administrator authorization is required. Enable your desktop authentication agent (polkit/pkexec), configure SUDO_ASKPASS, or run the setup once from a terminal. Your password is never stored by LaraServe")
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
func appleString(s string) string {
	return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r").Replace(s) + "\""
}
