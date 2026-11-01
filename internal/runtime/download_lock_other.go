//go:build !linux && !darwin

package runtime

import "fmt"

func lockDownload(path string) (func(), error) {
	return nil, fmt.Errorf("automatic runtime installation is supported on Linux and macOS")
}
