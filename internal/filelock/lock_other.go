//go:build !linux && !darwin && !windows

package filelock

import "fmt"

func Try(path string) (func(), bool, error) {
	return nil, false, fmt.Errorf("project locks are unsupported on this platform")
}
