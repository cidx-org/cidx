//go:build !unix

package executor

import "io/fs"

// fileGroup: no numeric group owner on this platform.
func fileGroup(fs.FileInfo) (int, bool) { return 0, false }
