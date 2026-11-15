package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// socketGroups returns the groups a container needs to use the Unix sockets it
// mounts: the group of each bind-mounted socket, as the host reports it.
//
// A container runs as the host user's uid:gid, and the groups that user holds on
// the host do not follow it in. A Docker socket is `root:docker 0660`, so a tool
// that talks to the daemon (cidx itself, scanning the catalogue) was refused
// with "permission denied" although the very same user runs containers every
// day. Docker's `--group-add` is the documented answer; the group comes from the
// socket, so nothing is configured by hand and nothing is assumed about the name
// or the gid of `docker` on this host.
//
// Group 0 is left out (root's own group grants nothing a socket mode would not),
// and so is a rootless runtime, whose user namespace maps gids differently: the
// host's number would name another group in there.
func socketGroups(volumes []string, host HostIdentity) []string {
	if host.Rootless {
		return nil
	}

	seen := map[int]bool{}
	var gids []int
	for _, volume := range volumes {
		source := bindSource(volume)
		if source == "" {
			continue
		}
		info, err := os.Stat(source)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			continue
		}
		gid, ok := fileGroup(info)
		if !ok || gid == 0 || seen[gid] {
			continue
		}
		seen[gid] = true
		gids = append(gids, gid)
	}

	sort.Ints(gids)
	groups := make([]string, len(gids))
	for i, gid := range gids {
		groups[i] = strconv.Itoa(gid)
	}
	return groups
}

// bindSource is the host path of a `source:target[:options]` bind, or "" when
// the source is not a path (a named volume).
func bindSource(volume string) string {
	source, _, _ := strings.Cut(strings.TrimSpace(volume), ":")
	if !filepath.IsAbs(source) {
		return ""
	}
	return source
}

// ensureBindSources creates the cache directories a container mounts from the
// workspace. Docker would create a missing source itself, as root, and a
// container running as the host user could then not write into it: the cache
// would be a read-only directory and every run would start cold again.
//
// Only sources inside the workspace are touched; anything else is the user's own
// path and is left to the daemon.
func ensureBindSources(volumes []string, workspace string) error {
	if workspace == "" {
		return nil
	}
	root := filepath.Clean(workspace) + string(filepath.Separator)
	for _, volume := range volumes {
		source := bindSource(volume)
		if source == "" || !strings.HasPrefix(filepath.Clean(source)+string(filepath.Separator), root) {
			continue
		}
		if _, err := os.Stat(source); err == nil {
			continue
		}
		if err := os.MkdirAll(source, 0o755); err != nil {
			return fmt.Errorf("failed to create mount source %s: %w", source, err)
		}
	}
	return nil
}
