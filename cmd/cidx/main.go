// Command cidx is the CI with Declarative eXecution CLI.
//
// This file is deliberately the whole of package main: the command tree and
// every handler live in internal/commands, a package the godog suite at the
// repository root can import. It used to live here, in a second `package main`
// that nothing could import, so the suite mirrored the tree by hand and the
// copy drifted (issue #317).
package main

import (
	"fmt"
	"os"

	"github.com/cidx-org/cidx/v3/internal/commands"
)

// Version is set via ldflags during build (`-X main.Version=2.1.2`). It stays
// in package main because that is the symbol the Makefile, the release
// workflow and the go-build preset all target.
var Version = "dev"

func main() {
	if err := commands.Run(Version, os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
