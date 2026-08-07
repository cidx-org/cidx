package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/cidx-org/cidx/v3/pkg/registry"
)

// The catalogue is pinned by digest (`image:tag@sha256:...`, issue #242): the
// tag stays readable for humans, the digest makes the reference immutable.
// Everything in this file exists to keep that form parseable and reproducible.

// parseImageRef splits an image reference into repository, tag and digest.
//
// A reference carries a tag, a digest, or both. Two colons must not be mistaken
// for the tag separator: the one inside `@sha256:...`, and the one in a
// registry port (`registry:5000/repo`). An untagged, undigested reference
// defaults to "latest", as Docker does; a digest-only reference has no tag at
// all, and saying so is more honest than inventing "latest".
func parseImageRef(image string) (name, tag, digest string) {
	if idx := strings.Index(image, "@"); idx != -1 {
		digest = image[idx+1:]
		image = image[:idx]
	}

	if idx := strings.LastIndex(image, ":"); idx != -1 {
		// A "/" after the colon means it was a registry port, not a tag.
		if afterColon := image[idx+1:]; !strings.Contains(afterColon, "/") {
			return image[:idx], afterColon, digest
		}
	}

	if digest != "" {
		return image, "", digest
	}
	return image, "latest", digest
}

// refWithoutDigest strips the digest from a reference, leaving `repo:tag`.
//
// Used wherever a reference is matched against data keyed by version rather
// than by content — `known-vulnerabilities.toml` records a CVE against
// `commitlint/commitlint:21.0.0`, and re-pinning that tag to a new digest must
// not silently orphan the exception.
func refWithoutDigest(image string) string {
	if idx := strings.Index(image, "@"); idx != -1 {
		return image[:idx]
	}
	return image
}

// imageRepository strips both tag and digest, leaving the repository a
// reference belongs to.
//
// This is the key `known-vulnerabilities.toml` records exceptions under: an
// accepted CVE is a judgement about a package in an image, in our usage, and
// none of that changes when the tag moves (#238). `rust:1.97.0` and
// `rust:1.97.0-slim` are the same repository, and an exception written for one
// covers the other — which is what the catalogue actually needs, since it runs
// both.
func imageRepository(image string) string {
	name, _, _ := parseImageRef(image)
	return name
}

// manifestAcceptTypes lists the manifest media types requested when resolving a
// digest, index types first. The order is load-bearing: a registry answers with
// the first type it can produce, and pinning a per-platform manifest digest
// instead of the index digest would break every runner on another architecture.
var manifestAcceptTypes = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

// digestPattern matches the digest form the catalogue is pinned in.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// errImageNotFound reports a reference the registry says does not exist, as
// opposed to one it merely would not serve us. The difference decides whether
// `preset scan-targets` calls a catalogue image gone (#245): a 401 from a
// registry we hold no credentials for is a lookup failure, not a deletion.
var errImageNotFound = errors.New("not found in the registry")

// resolveDigest returns the digest a reference currently resolves to, so a
// promoted version can be written back as `image:tag@sha256:...` rather than as
// a mutable tag.
//
// reference is a tag or a digest — the manifest endpoint accepts either, which
// is what lets the same call verify that the digest the catalogue is pinned to
// still exists.
//
// It speaks the registry v2 API directly instead of shelling out to Docker:
// the commands that need this (`preset scan-targets`) are metadata commands and
// must work on a runner that has no daemon.
func resolveDigest(image, reference string) (string, error) {
	reg, repo := parseRegistry(image)
	manifestURL := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", registryScheme, registryHost(reg), repo, reference)

	resp, err := registryDo(http.MethodHead, manifestURL, manifestAcceptTypes, reg)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("%s: %w", imageRef(image, reference), errImageNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry %s returned HTTP %d for %s", reg, resp.StatusCode, imageRef(image, reference))
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if !digestPattern.MatchString(digest) {
		return "", fmt.Errorf("registry %s returned no usable digest for %s (got %q)", reg, imageRef(image, reference), digest)
	}
	return digest, nil
}

// registryHost maps a registry name to the host serving its v2 API. Docker Hub
// is the one place where the two differ.
func registryHost(reg string) string {
	if reg == "docker.io" {
		return "registry-1.docker.io"
	}
	return reg
}

// imageRef rebuilds a printable reference, joining a digest with `@` and a tag
// with `:` so an error message reads the way the catalogue is written.
func imageRef(image, reference string) string {
	if strings.HasPrefix(reference, "sha256:") {
		return image + "@" + reference
	}
	return image + ":" + reference
}

// registryDo performs a registry v2 request, answering the Bearer challenge the
// registry sends when it wants one — which is most of them, including for
// public images where the token is anonymous.
//
// This is the single entry point to the v2 API: the manifest HEAD behind
// digest pinning (#242) and the tag listing behind update detection (#245) are
// the same endpoint family behind the same authentication, and one client is
// enough for both. The caller closes the response body.
func registryDo(method, requestURL, accept, reg string) (*http.Response, error) {
	resp, err := registryRequest(method, requestURL, accept, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	challenge := resp.Header.Get("Www-Authenticate")
	_ = resp.Body.Close()

	token, err := fetchRegistryToken(challenge, reg)
	if err != nil {
		return nil, err
	}
	return registryRequest(method, requestURL, accept, "Bearer "+token)
}

// registryRequest issues one request, carrying the media types the endpoint
// answers in.
func registryRequest(method, requestURL, accept, authorization string) (*http.Response, error) {
	req, err := http.NewRequest(method, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build registry request: %w", err)
	}
	req.Header.Set("Accept", accept)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach registry: %w", err)
	}
	return resp, nil
}

// fetchRegistryToken answers a `WWW-Authenticate: Bearer realm=...` challenge.
//
// Credentials are looked up exactly as `docker pull` does — for the registry
// itself, with the Docker Hub fallback DHI accepts (issue #162) — so a private
// registry resolves for whoever ran `cidx registry login`, and a public one
// resolves anonymously for everyone else.
func fetchRegistryToken(challenge, reg string) (string, error) {
	params := parseAuthChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry %s did not advertise a token endpoint", reg)
	}

	tokenURL, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("registry %s advertised an unusable token endpoint %q: %w", reg, realm, err)
	}
	query := tokenURL.Query()
	for _, key := range []string{"service", "scope"} {
		if value := params[key]; value != "" {
			query.Set(key, value)
		}
	}
	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to build token request: %w", err)
	}
	if creds := registryCredentials(reg); creds != nil {
		req.SetBasicAuth(creds.Username, creds.Secret)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach token endpoint of %s: %w", reg, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint of %s returned HTTP %d", reg, resp.StatusCode)
	}

	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to decode token from %s: %w", reg, err)
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("token endpoint of %s returned an empty token", reg)
}

// registryCredentials returns the stored credentials for a registry, or nil.
func registryCredentials(reg string) *registry.Credentials {
	manager := registry.NewManager()

	if reg == "docker.io" {
		return manager.GetDockerHubCredentials()
	}

	creds := manager.GetRegistryCredentials(reg)
	if creds == nil && reg == registry.DHIRegistry {
		creds = manager.GetDockerHubCredentials()
	}
	return creds
}

// parseAuthChallenge extracts the key="value" pairs of a Bearer challenge.
func parseAuthChallenge(challenge string) map[string]string {
	params := make(map[string]string)

	rest := strings.TrimSpace(challenge)
	if !strings.HasPrefix(strings.ToLower(rest), "bearer ") {
		return params
	}
	rest = rest[len("bearer "):]

	for _, field := range splitChallengeFields(rest) {
		key, value, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		params[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return params
}

// splitChallengeFields splits on the commas that separate parameters, ignoring
// the ones inside quoted values — a scope like
// `scope="repository:a:pull,repository:b:pull"` is a single field.
func splitChallengeFields(s string) []string {
	var fields []string
	var current strings.Builder
	inQuotes := false

	for _, r := range s {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case r == ',' && !inQuotes:
			fields = append(fields, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		fields = append(fields, current.String())
	}
	return fields
}
