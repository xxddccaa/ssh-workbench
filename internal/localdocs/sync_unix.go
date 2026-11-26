//go:build !windows

package localdocs

import "os"

func syncDirectory(directory string) {
	if handle, err := os.Open(directory); err == nil {
		handle.Sync()
		handle.Close()
	}
}
