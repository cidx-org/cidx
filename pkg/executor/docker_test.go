package executor

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/sirupsen/logrus"
)

func TestConfigHash_Deterministic(t *testing.T) {
	image := "alpine:latest"
	command := "echo hello"
	workdir := "/workspace"
	entrypoint := []string{"sh", "-c"}
	volumes := []string{"/src:/app"}
	env := map[string]string{"FOO": "bar", "BAZ": "qux"}

	h1 := configHash(image, command, workdir, entrypoint, volumes, env)
	h2 := configHash(image, command, workdir, entrypoint, volumes, env)

	if h1 != h2 {
		t.Errorf("configHash not deterministic: %s != %s", h1, h2)
	}
}

func TestConfigHash_DifferentInputs(t *testing.T) {
	base := configHash("alpine:latest", "echo", "", nil, nil, nil)

	tests := []struct {
		name       string
		image      string
		command    string
		workdir    string
		entrypoint []string
		volumes    []string
		env        map[string]string
	}{
		{"different image", "ubuntu:latest", "echo", "", nil, nil, nil},
		{"different command", "alpine:latest", "ls", "", nil, nil, nil},
		{"different workdir", "alpine:latest", "echo", "/scan", nil, nil, nil},
		{"different entrypoint", "alpine:latest", "echo", "", []string{"sh", "-c"}, nil, nil},
		{"with volumes", "alpine:latest", "echo", "", nil, []string{"/a:/b"}, nil},
		{"with env", "alpine:latest", "echo", "", nil, nil, map[string]string{"K": "V"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := configHash(tt.image, tt.command, tt.workdir, tt.entrypoint, tt.volumes, tt.env)
			if h == base {
				t.Errorf("expected different hash for %s, got same: %s", tt.name, h)
			}
		})
	}
}

func TestConfigHash_EnvSorting(t *testing.T) {
	env1 := map[string]string{"A": "1", "B": "2", "C": "3"}
	env2 := map[string]string{"C": "3", "A": "1", "B": "2"}

	h1 := configHash("img", "cmd", "", nil, nil, env1)
	h2 := configHash("img", "cmd", "", nil, nil, env2)

	if h1 != h2 {
		t.Errorf("configHash should be order-independent for env: %s != %s", h1, h2)
	}
}

func TestConfigHash_VolumeOrderMatters(t *testing.T) {
	// Volume order changes Docker's mount precedence — treat as a config change.
	h1 := configHash("img", "cmd", "", nil, []string{"/a:/x", "/b:/y"}, nil)
	h2 := configHash("img", "cmd", "", nil, []string{"/b:/y", "/a:/x"}, nil)

	if h1 == h2 {
		t.Errorf("configHash should differ when volume order changes: %s == %s", h1, h2)
	}
}

func TestConfigHash_VolumeWhitespaceNormalized(t *testing.T) {
	// Cosmetic whitespace in cidx.toml volume strings shouldn't trigger a recreate.
	h1 := configHash("img", "cmd", "", nil, []string{"/a:/x"}, nil)
	h2 := configHash("img", "cmd", "", nil, []string{"  /a:/x  "}, nil)

	if h1 != h2 {
		t.Errorf("configHash should ignore surrounding whitespace on volumes: %s != %s", h1, h2)
	}
}

func TestConfigHash_Length(t *testing.T) {
	h := configHash("img", "cmd", "", nil, nil, nil)
	if len(h) != 16 {
		t.Errorf("expected hash length 16, got %d", len(h))
	}
}

func TestDecideRecreate(t *testing.T) {
	tests := []struct {
		name         string
		existingHash string
		newHash      string
		noReuse      string
		wantReason   bool // true if a recreate reason should be returned
		wantContains string
	}{
		{
			name:         "matching hash, no override → reuse",
			existingHash: "abc123",
			newHash:      "abc123",
			noReuse:      "",
			wantReason:   false,
		},
		{
			name:         "hash mismatch → recreate",
			existingHash: "abc123",
			newHash:      "def456",
			noReuse:      "",
			wantReason:   true,
			wantContains: "config changed",
		},
		{
			name:         "missing label (pre-#144 container) → recreate",
			existingHash: "",
			newHash:      "def456",
			noReuse:      "",
			wantReason:   true,
			wantContains: "no cidx.config_hash label",
		},
		{
			name:         "CIDX_NO_REUSE=1 → recreate even if hashes match",
			existingHash: "abc123",
			newHash:      "abc123",
			noReuse:      "1",
			wantReason:   true,
			wantContains: "CIDX_NO_REUSE",
		},
		{
			name:         "CIDX_NO_REUSE takes priority over missing label",
			existingHash: "",
			newHash:      "def456",
			noReuse:      "true",
			wantReason:   true,
			wantContains: "CIDX_NO_REUSE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideRecreate(tt.existingHash, tt.newHash, tt.noReuse)
			if tt.wantReason && got == "" {
				t.Errorf("expected a recreate reason, got empty string")
			}
			if !tt.wantReason && got != "" {
				t.Errorf("expected reuse (empty reason), got: %q", got)
			}
			if tt.wantContains != "" && !strings.Contains(got, tt.wantContains) {
				t.Errorf("reason = %q, expected to contain %q", got, tt.wantContains)
			}
		})
	}
}

func TestParseCommand_Simple(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"single word", "scan", []string{"scan"}},
		{"two words", "trivy scan", []string{"trivy", "scan"}},
		{"multiple args", "trivy fs --severity HIGH .", []string{"trivy", "fs", "--severity", "HIGH", "."}},
		{"leading spaces", "  echo hello  ", []string{"echo", "hello"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommand(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("parseCommand(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommand(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseCommand_ShellCommand(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		script string
	}{
		{"single quotes", "sh -c 'echo hello && echo world'", "echo hello && echo world"},
		{"double quotes", `sh -c "echo hello"`, "echo hello"},
		{"no quotes", "sh -c echo hello", "echo hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommand(tt.input)
			if len(got) != 3 {
				t.Fatalf("expected 3 parts, got %d: %v", len(got), got)
			}
			if got[0] != "sh" || got[1] != "-c" {
				t.Errorf("expected [sh, -c, ...], got %v", got[:2])
			}
			if got[2] != tt.script {
				t.Errorf("script = %q, want %q", got[2], tt.script)
			}
		})
	}
}

func TestExpandVolumes(t *testing.T) {
	t.Setenv("TEST_WORKSPACE", "/projects/myapp")

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"env expansion", []string{"$TEST_WORKSPACE:/app"}, []string{"/projects/myapp:/app"}},
		{"multiple volumes", []string{"$TEST_WORKSPACE:/app", "$TEST_WORKSPACE:/scan"}, []string{"/projects/myapp:/app", "/projects/myapp:/scan"}},
		{"no env var", []string{"/static:/data"}, []string{"/static:/data"}},
		{"empty slice", []string{}, []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandVolumes(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("expandVolumes(%v) length = %d, want %d", tt.in, len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("expandVolumes(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsUnauthorizedError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unauthorized", errString("unauthorized access"), true},
		{"auth required", errString("authentication required"), true},
		{"denied", errString("access denied"), true},
		{"forbidden", errString("403 forbidden"), true},
		{"uppercase", errString("UNAUTHORIZED"), true},
		{"not auth error", errString("connection refused"), false},
		{"timeout", errString("context deadline exceeded"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUnauthorizedError(tt.err)
			if got != tt.want {
				t.Errorf("isUnauthorizedError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// errString is a simple error type for testing
type errString string

func (e errString) Error() string { return string(e) }

func TestExtractRegistry(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  string
	}{
		{"official image", "alpine", "docker.io"},
		{"docker hub user", "myuser/myimage", "docker.io"},
		{"docker hub org", "library/alpine", "docker.io"},
		{"ghcr", "ghcr.io/owner/image", "ghcr.io"},
		{"gcr", "gcr.io/project/image", "gcr.io"},
		{"custom registry", "registry.example.com/image", "registry.example.com"},
		{"registry with port", "localhost:5000/image", "localhost:5000"},
		{"ecr", "123456789.dkr.ecr.us-east-1.amazonaws.com/myimage", "123456789.dkr.ecr.us-east-1.amazonaws.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractRegistry(tt.image)
			if got != tt.want {
				t.Errorf("extractRegistry(%q) = %q, want %q", tt.image, got, tt.want)
			}
		})
	}
}

// Regression test for issue #197. Docker's `name` list filter is a substring
// (regex) match, so filtering on "cidx_p_build" also returns
// "cidx_p_build-musl". The old code took containers[0] — the newest match —
// removed that unrelated container and then tried to create the one it was
// asked for, whose name was still taken, producing a raw daemon Conflict.
// The lookup must match the name exactly.
func TestFindByExactName(t *testing.T) {
	containers := []container.Summary{
		{ID: "newer", Names: []string{"/cidx_p_build-musl"}},
		{ID: "older", Names: []string{"/cidx_p_build"}},
		{ID: "multi", Names: []string{"/other", "/cidx_p_lint"}},
	}

	tests := []struct {
		name   string
		lookup string
		wantID string
	}{
		{"exact match wins over newer substring match", "cidx_p_build", "older"},
		{"longer name still resolves to itself", "cidx_p_build-musl", "newer"},
		{"matches any of the container's names", "cidx_p_lint", "multi"},
		{"prefix of an existing name does not match", "cidx_p_buil", ""},
		{"unknown name returns nil", "cidx_p_trivy", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findByExactName(containers, tt.lookup)
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("expected no match, got %q", got.ID)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected match %q, got nil", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Errorf("findByExactName(%q) = %q, want %q", tt.lookup, got.ID, tt.wantID)
			}
		})
	}
}

// findManagedContainer must select on the ownership label, never on the name:
// names are project-scoped and legacy containers use a different shape.
func TestFindManagedContainer_FiltersByOwnershipLabel(t *testing.T) {
	var gotOpts container.ListOptions
	e := newTestExecutor()
	e.listFn = func(_ context.Context, opts container.ListOptions) ([]container.Summary, error) {
		gotOpts = opts
		return []container.Summary{{ID: "abc", Names: []string{"/cidx_p_trivy"}}}, nil
	}

	found, err := e.findManagedContainer(context.Background(), "cidx_p_trivy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found == nil || found.ID != "abc" {
		t.Fatalf("expected to find container abc, got %v", found)
	}
	if !gotOpts.All {
		t.Error("lookup must include stopped containers (All: true)")
	}
	if !gotOpts.Filters.ExactMatch("label", "managed-by=cidx") {
		t.Errorf("lookup must filter on managed-by=cidx, got filters %v", gotOpts.Filters)
	}
	if gotOpts.Filters.Len() != 1 {
		t.Errorf("lookup must not add a name filter, got filters %v", gotOpts.Filters)
	}
}

// Regression test for issue #197: when the name is held by a container that
// carries our labels, cidx reconciles by removing it instead of surfacing the
// daemon's Conflict error.
func TestReclaimContainerName_RemovesOurOwnContainer(t *testing.T) {
	var removed string
	e := newTestExecutor()
	e.inspectFn = func(_ context.Context, idOrName string) (container.InspectResponse, error) {
		return container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{ID: "deadbeefcafebabe"},
			Config:            &container.Config{Labels: map[string]string{"managed-by": "cidx", "cidx.tool": "trivy"}},
		}, nil
	}
	e.removeFn = func(_ context.Context, id string, opts container.RemoveOptions) error {
		removed = id
		if !opts.Force {
			t.Error("reclaim should force-remove so a running leftover is handled too")
		}
		return nil
	}

	if err := e.reclaimContainerName(context.Background(), "cidx_p_trivy"); err != nil {
		t.Fatalf("expected reclaim to succeed, got: %v", err)
	}
	if removed != "cidx_p_trivy" {
		t.Errorf("expected the conflicting container to be removed, got %q", removed)
	}
}

// A container that is not ours must never be deleted; the error has to tell
// the user what to do, naming `cidx cleanup`.
func TestReclaimContainerName_ForeignContainerIsReported(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
	}{
		{"no labels at all", nil},
		{"someone else's labels", map[string]string{"managed-by": "compose"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newTestExecutor()
			e.inspectFn = func(_ context.Context, _ string) (container.InspectResponse, error) {
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{ID: "7ada1234567890ab"},
					Config:            &container.Config{Labels: tt.labels},
				}, nil
			}
			e.removeFn = func(_ context.Context, id string, _ container.RemoveOptions) error {
				t.Fatalf("must not remove a container cidx does not own (%s)", id)
				return nil
			}

			err := e.reclaimContainerName(context.Background(), "cidx_p_trivy")
			var conflict *NameConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("expected *NameConflictError, got: %v", err)
			}
			if conflict.Name != "cidx_p_trivy" {
				t.Errorf("NameConflictError.Name = %q, want %q", conflict.Name, "cidx_p_trivy")
			}
			if !strings.Contains(err.Error(), "cidx cleanup") {
				t.Errorf("error must point at `cidx cleanup`, got: %v", err)
			}
			if !strings.Contains(err.Error(), "7ada12345678") {
				t.Errorf("error must identify the conflicting container, got: %v", err)
			}
		})
	}
}

func TestReclaimContainerName_InspectFailureIsWrapped(t *testing.T) {
	e := newTestExecutor()
	e.inspectFn = func(_ context.Context, _ string) (container.InspectResponse, error) {
		return container.InspectResponse{}, errString("no such container")
	}

	err := e.reclaimContainerName(context.Background(), "cidx_p_trivy")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "cidx_p_trivy") {
		t.Errorf("error should name the container, got: %v", err)
	}
}

func TestIsNameConflictError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{
			"docker conflict",
			errString(`Error response from daemon: Conflict. The container name "/cidx_p_trivy" is already in use by container "7ada".`),
			true,
		},
		{
			"podman conflict",
			errString(`Error: creating container storage: the container name "cidx_p_trivy" is already in use`),
			true,
		},
		{"unrelated error", errString("connection refused"), false},
		{"image pull error", errString("unauthorized: authentication required"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNameConflictError(tt.err); got != tt.want {
				t.Errorf("isNameConflictError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// newTestExecutor returns a DockerExecutor with a silent logger and no real
// Docker client. Callers set the seams they need.
func newTestExecutor() *DockerExecutor {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &DockerExecutor{logger: logger}
}

// newPullTestExecutor returns a DockerExecutor whose ImagePull calls are
// recorded and answered by the given script of results. Each entry in results
// is consumed in order; the RegistryAuth of every call is appended to *calls.
func newPullTestExecutor(t *testing.T, calls *[]string, results []error) *DockerExecutor {
	t.Helper()
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &DockerExecutor{
		logger: logger,
		pullFn: func(_ context.Context, _ string, opts image.PullOptions) (io.ReadCloser, error) {
			i := len(*calls)
			*calls = append(*calls, opts.RegistryAuth)
			if i >= len(results) {
				t.Fatalf("unexpected ImagePull call #%d", i+1)
			}
			if err := results[i]; err != nil {
				return nil, err
			}
			return io.NopCloser(strings.NewReader("")), nil
		},
	}
}

// Regression test for issue #162: cidx attaches Docker Hub credentials to
// dhi.io pulls that `docker pull` would never send. When the registry rejects
// them, cidx must fall back to an anonymous pull instead of failing.
func TestPullImageWithAuth_AnonymousFallback(t *testing.T) {
	unauthorized := errString("Error response from daemon: unauthorized: authentication required")

	t.Run("rejected credentials fall back to anonymous pull", func(t *testing.T) {
		var calls []string
		e := newPullTestExecutor(t, &calls, []error{unauthorized, nil})

		err := e.pullImageWithAuth(context.Background(), "dhi.io/trivy:0.68", "someauth")
		if err != nil {
			t.Fatalf("expected anonymous fallback to succeed, got: %v", err)
		}
		if len(calls) != 2 {
			t.Fatalf("expected 2 pull attempts, got %d", len(calls))
		}
		if calls[0] != "someauth" {
			t.Errorf("first attempt should use credentials, got RegistryAuth=%q", calls[0])
		}
		if calls[1] != "" {
			t.Errorf("fallback attempt should be anonymous, got RegistryAuth=%q", calls[1])
		}
	})

	t.Run("anonymous pull also rejected returns AuthError", func(t *testing.T) {
		var calls []string
		e := newPullTestExecutor(t, &calls, []error{unauthorized, unauthorized})

		err := e.pullImageWithAuth(context.Background(), "dhi.io/trivy:0.68", "someauth")
		var authErr *AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("expected *AuthError, got: %v", err)
		}
		if authErr.Registry != "dhi.io" {
			t.Errorf("AuthError.Registry = %q, want %q", authErr.Registry, "dhi.io")
		}
		if len(calls) != 2 {
			t.Fatalf("expected 2 pull attempts, got %d", len(calls))
		}
	})

	t.Run("unauthorized without credentials returns AuthError without retry", func(t *testing.T) {
		var calls []string
		e := newPullTestExecutor(t, &calls, []error{unauthorized})

		err := e.pullImageWithAuth(context.Background(), "dhi.io/trivy:0.68", "")
		var authErr *AuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("expected *AuthError, got: %v", err)
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 pull attempt (no retry without credentials), got %d", len(calls))
		}
	})

	t.Run("non-auth error is returned as-is without retry", func(t *testing.T) {
		var calls []string
		netErr := errString("Error response from daemon: dial tcp: lookup dhi.io: no such host")
		e := newPullTestExecutor(t, &calls, []error{netErr})

		err := e.pullImageWithAuth(context.Background(), "dhi.io/trivy:0.68", "someauth")
		if !errors.Is(err, netErr) {
			t.Fatalf("expected original error, got: %v", err)
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 pull attempt, got %d", len(calls))
		}
	})

	t.Run("successful authenticated pull does not retry", func(t *testing.T) {
		var calls []string
		e := newPullTestExecutor(t, &calls, []error{nil})

		err := e.pullImageWithAuth(context.Background(), "dhi.io/trivy:0.68", "someauth")
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
		if len(calls) != 1 {
			t.Fatalf("expected 1 pull attempt, got %d", len(calls))
		}
	})
}
