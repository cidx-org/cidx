//go:build unix

package executor

import (
	"io/fs"
	"syscall"
)

// fileGroup is the gid that owns the file, when the platform reports one.
func fileGroup(info fs.FileInfo) (int, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(stat.Gid), true
}
