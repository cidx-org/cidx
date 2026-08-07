package guards

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scannerImages are the two scanners the audit and the monitor invoke. A
// `docker run` naming one of them and pointing it at the matrix image is a scan,
// and a scan has to be able to authenticate.
var scannerImages = []string{"aquasec/trivy:", "anchore/grype:"}

// TestScansCarryTheRunnerCredentials is the standing guard behind issue #284.
//
// `docker/login-action` writes its credentials into the *runner's*
// ~/.docker/config.json. The scanners run in containers, which see neither that
// file nor the daemon: unless the config is handed to them explicitly they
// re-pull every image anonymously. That passes on public registries and answers
// 401 on dhi.io, so the five hardened images produced no findings at all — and
// no findings is not the same as no vulnerabilities. `cidx security vuln prune`
// is fail-closed by design, so it pinned 122 exceptions in `unknown`, unable to
// show any of them gone.
//
// The regression is invisible: the workflow stays green, the artifacts still
// upload, and only a count buried in a report says a quarter of the catalogue
// was skipped. So the invariant is pinned here rather than left to review.
func TestScansCarryTheRunnerCredentials(t *testing.T) {
	for _, workflow := range []string{
		filepath.Join(projectRoot, ".github/workflows/security-audit.yml"),
		filepath.Join(projectRoot, ".github/workflows/container-monitor.yml"),
	} {
		content, err := os.ReadFile(workflow)
		if err != nil {
			t.Fatalf("failed to read %s: %v", workflow, err)
		}

		commands := dockerRunCommands(string(content))
		if len(commands) == 0 {
			t.Fatalf("%s: no `docker run` command found — the guard would pass vacuously", workflow)
		}

		scans := 0
		for _, command := range commands {
			// Only the commands that scan a catalogue image. The database
			// downloads also run a scanner container, but they talk to the
			// scanner's own distribution registry, which wants no credentials
			// of ours.
			if !namesAScanner(command) || !strings.Contains(command, `"$IMAGE"`) {
				continue
			}
			scans++

			if !strings.Contains(command, "DOCKER_CONFIG") && !strings.Contains(command, "DOCKER_AUTH[@]") {
				t.Errorf("%s: this scan cannot authenticate — it passes the runner's registry\n"+
					"credentials to neither scanner, so a private registry answers 401 and the\n"+
					"image silently produces no findings (#284):\n\n%s\n\n"+
					"Mount the docker config read-only and point DOCKER_CONFIG at it:\n"+
					`  -v "$HOME/.docker:/docker-config:ro" -e DOCKER_CONFIG=/docker-config`,
					workflow, command)
			}
		}

		if scans == 0 {
			t.Errorf("%s: no scan of a matrix image found — the guard would pass vacuously", workflow)
		}

		// The audit reaches the same flags through a shell array, so the array
		// itself has to carry them.
		for _, definition := range shellArrayDefinitions(string(content), "DOCKER_AUTH") {
			if !strings.Contains(definition, "DOCKER_CONFIG") {
				t.Errorf("%s: DOCKER_AUTH is expanded into the scan commands but defines no\n"+
					"DOCKER_CONFIG, so it hands the scanner nothing (#284):\n\n%s", workflow, definition)
			}
		}
	}
}

// dockerRunCommands returns each `docker run` invocation as a single string,
// following backslash continuations so the flags and the image that belong to
// one command are weighed together.
func dockerRunCommands(workflow string) []string {
	var commands []string

	lines := strings.Split(workflow, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.Contains(lines[i], "docker run") {
			continue
		}

		command := strings.TrimSpace(lines[i])
		for strings.HasSuffix(command, `\`) && i+1 < len(lines) {
			i++
			command = strings.TrimSuffix(command, `\`) + " " + strings.TrimSpace(lines[i])
		}
		commands = append(commands, command)
	}

	return commands
}

// shellArrayDefinitions returns every `NAME=(...)` assignment found, joined the
// same way, so a definition spread over several lines is read whole.
func shellArrayDefinitions(workflow, name string) []string {
	var definitions []string

	lines := strings.Split(workflow, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.Contains(lines[i], name+"=(") {
			continue
		}

		definition := strings.TrimSpace(lines[i])
		for !strings.Contains(definition, ")") && i+1 < len(lines) {
			i++
			definition += " " + strings.TrimSpace(lines[i])
		}
		definitions = append(definitions, definition)
	}

	return definitions
}

func namesAScanner(command string) bool {
	for _, image := range scannerImages {
		if strings.Contains(command, image) {
			return true
		}
	}
	return false
}
