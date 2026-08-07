package guards

// The guards in this package are about the repository rather than about any
// package of it: no source may point at a removed command (#174), every git
// invocation pins its locale (#364), a checking phase never writes (#272), the
// audit carries the runner's credentials (#284), a PR action names what it
// commits (#323).
//
// They live together because that is what they have in common, and they live
// under internal/ so the module root holds no Go files at all — the tree it
// walks is the repository, not a package.

// projectRoot is the repository, seen from the directory `go test` runs this
// package in. Same convention as pkg/config's test_scope_guard_test.go.
const projectRoot = "../.."
