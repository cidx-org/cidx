package executor

import "testing"

func TestParseBackendType(t *testing.T) {
	tests := []struct {
		input string
		want  BackendType
	}{
		{"docker", BackendDocker},
		{"podman", BackendPodman},
		{"kubernetes", BackendKubernetes},
		{"k8s", BackendKubernetes},
		{"auto", BackendAuto},
		{"", BackendAuto},
		{"unknown", BackendAuto},
		{"Docker", BackendAuto}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseBackendType(tt.input)
			if got != tt.want {
				t.Errorf("ParseBackendType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBackendType_String(t *testing.T) {
	tests := []struct {
		backend BackendType
		want    string
	}{
		{BackendAuto, "auto"},
		{BackendDocker, "docker"},
		{BackendPodman, "podman"},
		{BackendKubernetes, "kubernetes"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.backend.String()
			if got != tt.want {
				t.Errorf("BackendType(%q).String() = %q, want %q", tt.backend, got, tt.want)
			}
		})
	}
}
