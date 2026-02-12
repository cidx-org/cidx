package executor

import (
	"os"
	"testing"
)

func TestConfigHash_Deterministic(t *testing.T) {
	image := "alpine:latest"
	command := "echo hello"
	volumes := []string{"/src:/app"}
	env := map[string]string{"FOO": "bar", "BAZ": "qux"}

	h1 := configHash(image, command, volumes, env)
	h2 := configHash(image, command, volumes, env)

	if h1 != h2 {
		t.Errorf("configHash not deterministic: %s != %s", h1, h2)
	}
}

func TestConfigHash_DifferentInputs(t *testing.T) {
	base := configHash("alpine:latest", "echo", nil, nil)

	tests := []struct {
		name    string
		image   string
		command string
		volumes []string
		env     map[string]string
	}{
		{"different image", "ubuntu:latest", "echo", nil, nil},
		{"different command", "alpine:latest", "ls", nil, nil},
		{"with volumes", "alpine:latest", "echo", []string{"/a:/b"}, nil},
		{"with env", "alpine:latest", "echo", nil, map[string]string{"K": "V"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := configHash(tt.image, tt.command, tt.volumes, tt.env)
			if h == base {
				t.Errorf("expected different hash for %s, got same: %s", tt.name, h)
			}
		})
	}
}

func TestConfigHash_EnvSorting(t *testing.T) {
	env1 := map[string]string{"A": "1", "B": "2", "C": "3"}
	env2 := map[string]string{"C": "3", "A": "1", "B": "2"}

	h1 := configHash("img", "cmd", nil, env1)
	h2 := configHash("img", "cmd", nil, env2)

	if h1 != h2 {
		t.Errorf("configHash should be order-independent for env: %s != %s", h1, h2)
	}
}

func TestConfigHash_Length(t *testing.T) {
	h := configHash("img", "cmd", nil, nil)
	if len(h) != 16 {
		t.Errorf("expected hash length 16, got %d", len(h))
	}
}

func TestParseCommand_Simple(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
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
	os.Setenv("TEST_WORKSPACE", "/projects/myapp")
	defer os.Unsetenv("TEST_WORKSPACE")

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

func TestExpandCommand(t *testing.T) {
	tests := []struct {
		name    string
		command string
		env     map[string]string
		want    string
	}{
		{
			"simple substitution",
			"trivy fs --severity ${SEVERITY} .",
			map[string]string{"SEVERITY": "HIGH"},
			"trivy fs --severity HIGH .",
		},
		{
			"no substitution",
			"trivy scan .",
			map[string]string{"UNUSED": "val"},
			"trivy scan .",
		},
		{
			"multiple substitutions",
			"${TOOL} ${ACTION}",
			map[string]string{"TOOL": "trivy", "ACTION": "scan"},
			"trivy scan",
		},
		{
			"shell command preserves structure",
			"sh -c 'echo ${MSG}'",
			map[string]string{"MSG": "hello"},
			"sh -c 'echo hello'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandCommand(tt.command, tt.env)
			if got != tt.want {
				t.Errorf("expandCommand(%q, %v) = %q, want %q", tt.command, tt.env, got, tt.want)
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
