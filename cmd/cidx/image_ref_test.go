package main

import "testing"

// TestParseImageRef covers the reference forms the catalogue and user configs
// can hold, now that every built-in image is pinned `image:tag@sha256:...`
// (#242). The digest's own colon must never be read as the tag separator, and
// neither must a registry port.
func TestParseImageRef(t *testing.T) {
	const digest = "sha256:31684fd957f78ddebd0fc003f2790da08bd7a41cfe1146912da43a114fbeebf9"

	tests := []struct {
		name       string
		ref        string
		wantName   string
		wantTag    string
		wantDigest string
	}{
		{
			name:     "tag only",
			ref:      "tmknom/prettier:3.6.2",
			wantName: "tmknom/prettier",
			wantTag:  "3.6.2",
		},
		{
			name:     "no tag defaults to latest",
			ref:      "alpine",
			wantName: "alpine",
			wantTag:  "latest",
		},
		{
			name:       "tag and digest",
			ref:        "tmknom/prettier:3.6.2@" + digest,
			wantName:   "tmknom/prettier",
			wantTag:    "3.6.2",
			wantDigest: digest,
		},
		{
			name:       "digest without tag has no tag",
			ref:        "tmknom/prettier@" + digest,
			wantName:   "tmknom/prettier",
			wantTag:    "",
			wantDigest: digest,
		},
		{
			name:       "registry host with tag and digest",
			ref:        "ghcr.io/ansible/community-ansible-dev-tools:v25.12.0@" + digest,
			wantName:   "ghcr.io/ansible/community-ansible-dev-tools",
			wantTag:    "v25.12.0",
			wantDigest: digest,
		},
		{
			name:       "registry port with tag and digest",
			ref:        "registry.internal:5000/team/tool:1.2.3@" + digest,
			wantName:   "registry.internal:5000/team/tool",
			wantTag:    "1.2.3",
			wantDigest: digest,
		},
		{
			name:     "registry port without tag",
			ref:      "registry.internal:5000/team/tool",
			wantName: "registry.internal:5000/team/tool",
			wantTag:  "latest",
		},
		{
			name:       "registry port with digest but no tag",
			ref:        "registry.internal:5000/team/tool@" + digest,
			wantName:   "registry.internal:5000/team/tool",
			wantTag:    "",
			wantDigest: digest,
		},
		{
			name:     "variant tag is preserved",
			ref:      "dhi.io/golang:1.23-alpine3.21-dev",
			wantName: "dhi.io/golang",
			wantTag:  "1.23-alpine3.21-dev",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, tag, dgst := parseImageRef(tt.ref)
			if name != tt.wantName {
				t.Errorf("parseImageRef(%q) name = %q, want %q", tt.ref, name, tt.wantName)
			}
			if tag != tt.wantTag {
				t.Errorf("parseImageRef(%q) tag = %q, want %q", tt.ref, tag, tt.wantTag)
			}
			if dgst != tt.wantDigest {
				t.Errorf("parseImageRef(%q) digest = %q, want %q", tt.ref, dgst, tt.wantDigest)
			}
		})
	}
}

// TestParseImageRefFeedsRegistryLookup pins the contract between parseImageRef
// and parseRegistry: the name handed over must never carry a digest, or the
// registry host would be resolved from a string containing `@sha256:`.
func TestParseImageRefFeedsRegistryLookup(t *testing.T) {
	tests := []struct {
		ref          string
		wantRegistry string
		wantRepo     string
	}{
		{"rust:1.95.0@sha256:" + zeroDigest, "docker.io", "library/rust"},
		{"securego/gosec:2.24.6@sha256:" + zeroDigest, "docker.io", "securego/gosec"},
		{"ghcr.io/astral-sh/ruff:0.8.2@sha256:" + zeroDigest, "ghcr.io", "astral-sh/ruff"},
		{"gcr.io/kaniko-project/executor:v1.24.0@sha256:" + zeroDigest, "gcr.io", "kaniko-project/executor"},
		{"dhi.io/trivy:0.68@sha256:" + zeroDigest, "dhi.io", "trivy"},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			name, _, _ := parseImageRef(tt.ref)
			reg, repo := parseRegistry(name)
			if reg != tt.wantRegistry || repo != tt.wantRepo {
				t.Errorf("parseRegistry(%q) = (%q, %q), want (%q, %q)", name, reg, repo, tt.wantRegistry, tt.wantRepo)
			}
		})
	}
}

const zeroDigest = "0000000000000000000000000000000000000000000000000000000000000000"

// TestRefWithoutDigest guards the lookup key used against
// known-vulnerabilities.toml, which records exceptions per `repo:tag`.
func TestRefWithoutDigest(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"commitlint/commitlint:21.0.0@sha256:" + zeroDigest, "commitlint/commitlint:21.0.0"},
		{"commitlint/commitlint:21.0.0", "commitlint/commitlint:21.0.0"},
		{"alpine", "alpine"},
		{"registry.internal:5000/team/tool:1.2.3@sha256:" + zeroDigest, "registry.internal:5000/team/tool:1.2.3"},
	}

	for _, tt := range tests {
		if got := refWithoutDigest(tt.ref); got != tt.want {
			t.Errorf("refWithoutDigest(%q) = %q, want %q", tt.ref, got, tt.want)
		}
	}
}

// TestGetLatestTagRejectsDigestOnlyReference: a reference with no tag has no
// version to compare, and reporting that is better than querying the registry
// for an empty string.
func TestGetLatestTagRejectsDigestOnlyReference(t *testing.T) {
	if _, _, err := getLatestTag("tmknom/prettier", ""); err == nil {
		t.Fatal("getLatestTag with an empty tag should fail, got nil error")
	}
}

// TestParseAuthChallenge covers the WWW-Authenticate parsing that digest
// resolution relies on. A scope value may itself contain commas and colons.
func TestParseAuthChallenge(t *testing.T) {
	challenge := `Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/rust:pull,repository:library/alpine:pull"`

	params := parseAuthChallenge(challenge)

	if got, want := params["realm"], "https://auth.docker.io/token"; got != want {
		t.Errorf("realm = %q, want %q", got, want)
	}
	if got, want := params["service"], "registry.docker.io"; got != want {
		t.Errorf("service = %q, want %q", got, want)
	}
	if got, want := params["scope"], "repository:library/rust:pull,repository:library/alpine:pull"; got != want {
		t.Errorf("scope = %q, want %q", got, want)
	}
}

// TestParseAuthChallengeIgnoresNonBearer: a registry answering with a scheme we
// cannot satisfy must yield no parameters rather than a half-parsed realm.
func TestParseAuthChallengeIgnoresNonBearer(t *testing.T) {
	if params := parseAuthChallenge(`Basic realm="registry"`); len(params) != 0 {
		t.Errorf("parseAuthChallenge(Basic ...) = %v, want empty", params)
	}
}
