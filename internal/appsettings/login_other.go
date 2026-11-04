//go:build !windows

package appsettings

func platformLogin(home, executable string, enabled bool) (bool, error) { return false, nil }
