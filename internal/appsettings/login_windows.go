//go:build windows

package appsettings

import (
	"fmt"
	"golang.org/x/sys/windows/registry"
	"path/filepath"
	"strings"
	"syscall"
)

func platformLogin(home, executable string, enabled bool) (bool, error) {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return true, err
	}
	defer key.Close()
	name := loginName(home)
	if !enabled {
		err = key.DeleteValue(name)
		if err == registry.ErrNotExist {
			err = nil
		}
		return true, err
	}
	if !filepath.IsAbs(executable) || !filepath.IsAbs(home) || strings.ContainsAny(executable+home, "\r\n\x00") {
		return true, fmt.Errorf("startup requires stable absolute executable and data paths")
	}
	command := syscall.EscapeArg(executable) + " __autostart --home " + syscall.EscapeArg(home)
	if len(command) > 260 {
		return true, fmt.Errorf("login startup command exceeds Windows' 260-character Run limit")
	}
	return true, key.SetStringValue(name, command)
}
