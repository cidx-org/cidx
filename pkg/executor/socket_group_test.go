//go:build unix

package executor

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cidx-org/cidx/v3/pkg/config"
)

// listen makes a Unix socket in a short temp path (socket paths are length-bound).
func listen(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Skipf("cannot create a unix socket here: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return path
}

func TestSocketGroupsNamesTheGroupOfAMountedSocket(t *testing.T) {
	sock := listen(t)
	info, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	gid, _ := fileGroup(info)

	got := socketGroups([]string{"/work:/work", sock + ":/var/run/docker.sock:ro", "named-volume:/x"}, HostIdentity{UID: 1001, GID: gid})
	if gid == 0 {
		if len(got) != 0 {
			t.Fatalf("group 0 must not be added, got %v", got)
		}
		return
	}
	if len(got) != 1 || got[0] != strconv.Itoa(gid) {
		t.Fatalf("groups = %v, want [%d]", got, gid)
	}
}

func TestSocketGroupsIgnoresPlainFilesRootlessAndMissingSources(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "plain")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	host := HostIdentity{UID: 1, GID: 1}
	if got := socketGroups([]string{file + ":/a", dir + ":/b", "/nonexistent/x.sock:/c"}, host); len(got) != 0 {
		t.Fatalf("groups = %v, want none", got)
	}

	sock := listen(t)
	if got := socketGroups([]string{sock + ":/s"}, HostIdentity{UID: 1, GID: 1, Rootless: true}); len(got) != 0 {
		t.Fatalf("rootless runtime must not get host gids, got %v", got)
	}
}

func TestReuseKeyChangesWithTheSocketGroupOnly(t *testing.T) {
	sock := listen(t)
	info, _ := os.Stat(sock)
	gid, _ := fileGroup(info)
	c := &config.ContainerConfig{Image: "img"}
	host := HostIdentity{UID: 1, GID: 1}

	plain := ReuseKey(c, "cmd", []string{"/work:/work"}, host)
	withSocket := ReuseKey(c, "cmd", []string{"/work:/work", sock + ":/s"}, host)
	if gid != 0 && plain == withSocket {
		t.Error("a container created without the socket group must not be reused for one that needs it")
	}
}

func TestEnsureBindSourcesCreatesOnlyInsideTheWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other")
	inside := filepath.Join(ws, ".cidx", "go", "mod")

	if err := ensureBindSources([]string{inside + ":/c", outside + ":/o"}, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(inside); err != nil {
		t.Errorf("cache directory not created: %v", err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Error("a path outside the workspace must be left to the daemon")
	}
}
