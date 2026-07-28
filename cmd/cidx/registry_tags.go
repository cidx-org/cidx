package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Tag listing over the registry v2 API (issue #245).
//
// Update detection used to reach Docker Hub and Quay.io only, which left 9 of
// the 21 catalogue images — every Docker Hardened Image among them — without a
// candidate of any kind. `GET /v2/<repo>/tags/list` is the same endpoint family
// as the manifest HEAD that pins digests, behind the same Bearer challenge, so
// it goes through registryDo rather than a second client.

// tagListing is everything a registry will say about the tags of a repository:
// their names always, and — of the registries the catalogue pulls from, gcr.io
// alone — when each one was uploaded.
type tagListing struct {
	Tags []string

	// Published maps a tag to the moment the registry received it. It is empty
	// on a registry that publishes no dates, and a missing entry must never be
	// read as "old enough": the promotion cooldown is fail-closed (#246).
	Published map[string]time.Time
}

// maxTagPages bounds the pagination loop. ghcr.io answers 100 tags a page, so
// this allows 5000 — far past anything the catalogue pulls from. Hitting it is
// an error rather than a silent truncation: ghcr.io lists oldest first, so a
// truncated listing would drop exactly the versions we came for.
const maxTagPages = 50

// registryScheme is https everywhere a registry is actually contacted. It is a
// variable so the tests can stand a real listing — pagination, Bearer challenge
// and all — in front of an httptest server instead of a network.
var registryScheme = "https"

// listTags returns every tag a repository holds, following the pagination the
// registry asks for.
func listTags(image string) (tagListing, error) {
	reg, repo := parseRegistry(image)

	listing := tagListing{Published: make(map[string]time.Time)}
	next := fmt.Sprintf("%s://%s/v2/%s/tags/list", registryScheme, registryHost(reg), repo)

	for page := 0; next != ""; page++ {
		if page == maxTagPages {
			return tagListing{}, fmt.Errorf("registry %s paginated the tags of %s past %d pages", reg, repo, maxTagPages)
		}

		body, link, err := fetchTagPage(next, reg)
		if err != nil {
			return tagListing{}, err
		}

		parsed, err := parseTagPage(body)
		if err != nil {
			return tagListing{}, fmt.Errorf("failed to read the tag listing of %s from %s: %w", repo, reg, err)
		}

		listing.Tags = append(listing.Tags, parsed.Tags...)
		for tag, uploaded := range parsed.Published {
			listing.Published[tag] = uploaded
		}

		next = nextPageURL(reg, link)
	}

	return listing, nil
}

// fetchTagPage retrieves one page of the listing and the Link header pointing
// at the next, if the registry sent one.
func fetchTagPage(pageURL, reg string) ([]byte, string, error) {
	resp, err := registryDo(http.MethodGet, pageURL, "application/json", reg)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("registry %s returned HTTP %d listing tags", reg, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read the tag listing from %s: %w", reg, err)
	}
	return body, resp.Header.Get("Link"), nil
}

// parseTagPage decodes one page of `tags/list`.
//
// `manifest` is gcr.io's extension to the response: a map of manifest digest to
// the tags pointing at it and the moment it was uploaded. Registries that do
// not send it decode to an empty map, so one decoder serves all of them.
func parseTagPage(body []byte) (tagListing, error) {
	var payload struct {
		Tags     []string `json:"tags"`
		Manifest map[string]struct {
			Tag            []string `json:"tag"`
			TimeUploadedMs string   `json:"timeUploadedMs"`
		} `json:"manifest"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return tagListing{}, err
	}

	listing := tagListing{Tags: payload.Tags, Published: make(map[string]time.Time)}
	for _, manifest := range payload.Manifest {
		// timeUploadedMs is when the registry received the manifest, which is
		// the moment the version became pullable — not the `created` of the
		// config blob, which is a build date the policy rejects (#246).
		milliseconds, err := strconv.ParseInt(manifest.TimeUploadedMs, 10, 64)
		if err != nil || milliseconds <= 0 {
			continue
		}
		uploaded := time.UnixMilli(milliseconds).UTC()
		for _, tag := range manifest.Tag {
			listing.Published[tag] = uploaded
		}
	}
	return listing, nil
}

// nextPageURL reads the `Link: </v2/...>; rel="next"` header a paginating
// registry sends.
//
// The path is followed verbatim, cursor and all: the registry chose where the
// page break falls, and rebuilding the query ourselves would be guessing at it.
func nextPageURL(reg, link string) string {
	if !strings.Contains(link, "next") {
		return ""
	}

	start := strings.Index(link, "<")
	end := strings.Index(link, ">")
	if start == -1 || end <= start {
		return ""
	}

	target := link[start+1 : end]
	if strings.HasPrefix(target, "http") {
		return target
	}
	return registryScheme + "://" + registryHost(reg) + target
}
