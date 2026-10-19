//go:build !windows

package fileutil

import "os"

func Replace(tempPath, path string) error {
	return os.Rename(tempPath, path)
}
