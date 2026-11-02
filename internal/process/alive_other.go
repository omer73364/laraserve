//go:build !linux && !windows

package process

import "fmt"

func Alive(pid int) (bool, error) {
	return true, fmt.Errorf("automatic stale database lock recovery is unavailable on this platform")
}
