package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestParseTagPageReadsANameOnlyListing covers what the OCI distribution API
// promises and all ghcr.io and dhi.io deliver: tag names, no dates. The empty
// Published map is the signal buildScanTargets acts on (#245).
func TestParseTagPageReadsANameOnlyListing(t *testing.T) {
	body := []byte(`{"name":"astral-sh/ruff","tags":["latest","0.8.1","0.8.2"]}`)

	listing, err := parseTagPage(body)
	if err != nil {
		t.Fatalf("parseTagPage() error = %v", err)
	}

	want := []string{"latest", "0.8.1", "0.8.2"}
	if len(listing.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", listing.Tags, want)
	}
	for i := range want {
		if listing.Tags[i] != want[i] {
			t.Fatalf("tags = %v, want %v", listing.Tags, want)
		}
	}
	if len(listing.Published) != 0 {
		t.Errorf("Published = %v, want empty: this registry publishes no dates", listing.Published)
	}
}

// TestParseTagPageReadsGCRUploadTimes covers gcr.io's extension to the same
// response: a manifest map dating every tag. That date is what makes gcr.io
// candidates promotable at all, since the cooldown is measured against it.
func TestParseTagPageReadsGCRUploadTimes(t *testing.T) {
	body := []byte(`{
	  "name":"kaniko-project/executor",
	  "tags":["v1.23.2","v1.24.0"],
	  "manifest":{
	    "sha256:aaa":{"tag":["v1.23.2"],"timeUploadedMs":"1720568313332"},
	    "sha256:bbb":{"tag":["v1.24.0","latest"],"timeUploadedMs":"1748002111637"}
	  }
	}`)

	listing, err := parseTagPage(body)
	if err != nil {
		t.Fatalf("parseTagPage() error = %v", err)
	}

	want := time.UnixMilli(1748002111637).UTC()
	if got := listing.Published["v1.24.0"]; !got.Equal(want) {
		t.Errorf("Published[v1.24.0] = %s, want %s", got, want)
	}
	// A manifest carrying several tags dates all of them.
	if got := listing.Published["latest"]; !got.Equal(want) {
		t.Errorf("Published[latest] = %s, want %s", got, want)
	}
	if got, want := listing.Published["v1.23.2"], time.UnixMilli(1720568313332).UTC(); !got.Equal(want) {
		t.Errorf("Published[v1.23.2] = %s, want %s", got, want)
	}
}

// TestParseTagPageIgnoresUnusableUploadTimes: gcr.io writes "0" for a manifest
// it has no upload time for. A zero epoch would date the tag to 1970 and hand
// the cooldown a version that is decades old, so it yields no date at all.
func TestParseTagPageIgnoresUnusableUploadTimes(t *testing.T) {
	body := []byte(`{
	  "tags":["v1.0.0","v1.1.0"],
	  "manifest":{
	    "sha256:aaa":{"tag":["v1.0.0"],"timeUploadedMs":"0"},
	    "sha256:bbb":{"tag":["v1.1.0"],"timeUploadedMs":"not-a-number"}
	  }
	}`)

	listing, err := parseTagPage(body)
	if err != nil {
		t.Fatalf("parseTagPage() error = %v", err)
	}
	if len(listing.Published) != 0 {
		t.Errorf("Published = %v, want empty", listing.Published)
	}
}

// TestNextPageURL covers the Link header the paginating registries send. A
// listing read only to its first page would miss the newest versions on
// ghcr.io, which lists oldest first.
func TestNextPageURL(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{
			name: "relative next page is resolved against the registry",
			link: `</v2/astral-sh/ruff/tags/list?last=0.8.0&n=0>; rel="next"`,
			want: "https://ghcr.io/v2/astral-sh/ruff/tags/list?last=0.8.0&n=0",
		},
		{
			name: "absolute next page is followed as sent",
			link: `<https://ghcr.io/v2/astral-sh/ruff/tags/list?last=0.8.0>; rel="next"`,
			want: "https://ghcr.io/v2/astral-sh/ruff/tags/list?last=0.8.0",
		},
		{
			name: "no header means the listing is complete",
			link: "",
			want: "",
		},
		{
			name: "a link that is not a next page is not followed",
			link: `</v2/astral-sh/ruff/tags/list>; rel="prev"`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextPageURL("ghcr.io", tt.link); got != tt.want {
				t.Errorf("nextPageURL(%q) = %q, want %q", tt.link, got, tt.want)
			}
		})
	}
}

// TestListTagsFollowsPaginationBehindABearerChallenge exercises the whole
// listing path — 401 challenge, token, page, cursor, next page — against a
// local server. No registry is contacted.
func TestListTagsFollowsPaginationBehindABearerChallenge(t *testing.T) {
	image, requests := tagListingServer(t, map[string]string{
		"": `{"tags":["0.8.0","0.8.1"]}`,
		// The cursor the first page hands back, followed verbatim.
		"0.8.1": `{"tags":["0.8.2"]}`,
	})

	listing, err := listTags(image)
	if err != nil {
		t.Fatalf("listTags() error = %v", err)
	}

	want := []string{"0.8.0", "0.8.1", "0.8.2"}
	if strings.Join(listing.Tags, ",") != strings.Join(want, ",") {
		t.Errorf("tags = %v, want %v (the second page was not followed)", listing.Tags, want)
	}
	if *requests.tokens == 0 {
		t.Error("the Bearer challenge was never answered")
	}
}

// TestListTagsReportsARegistryRefusal: a repository the registry will not list
// yields an error, so scan-targets keeps scanning the current image rather than
// pretending nothing newer exists.
func TestListTagsReportsARegistryRefusal(t *testing.T) {
	image, _ := tagListingServer(t, nil)

	if _, err := listTags(image); err == nil {
		t.Fatal("listTags() on an unlistable repository should fail, got nil error")
	}
}

// TestListTagsStopsAtThePageCap: a registry that keeps handing out cursors is
// an error rather than a silently truncated listing, which would hide the very
// versions the listing is read for.
func TestListTagsStopsAtThePageCap(t *testing.T) {
	restoreScheme(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `</v2/team/tool/tags/list?last=0.1.0&n=0>; rel="next"`)
		_, _ = w.Write([]byte(`{"tags":["0.1.0"]}`))
	}))
	t.Cleanup(server.Close)

	_, err := listTags(strings.TrimPrefix(server.URL, "http://") + "/team/tool")
	if err == nil {
		t.Fatal("an endless pagination should fail, got nil error")
	}
	if !strings.Contains(err.Error(), "paginated") {
		t.Errorf("error = %v, want it to name the pagination cap", err)
	}
}

// restoreScheme points the registry client at a plain-HTTP test server for the
// duration of one test.
func restoreScheme(t *testing.T) {
	t.Helper()
	original := registryScheme
	registryScheme = "http"
	t.Cleanup(func() { registryScheme = original })
	// registryCredentials reads ~/.docker/config.json; an empty home keeps the
	// test from finding — or shelling out to — a real credential helper.
	t.Setenv("HOME", t.TempDir())
}

type tagListingCounters struct{ tokens *int }

// tagListingServer stands a registry in front of the test: it demands a Bearer
// token, then answers `tags/list` from pages keyed by the `last` cursor. A
// repository with no pages answers 404, as a registry does for one it will not
// list. It returns the image name to ask for.
func tagListingServer(t *testing.T, pages map[string]string) (string, tagListingCounters) {
	t.Helper()
	restoreScheme(t)

	tokens := 0
	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokens++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "test-token"})
			return
		}

		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("Www-Authenticate", fmt.Sprintf(`Bearer realm="%s/token",service="test"`, server.URL))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		page, ok := pages[r.URL.Query().Get("last")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Every page but the last hands back a cursor, the way ghcr.io does:
		// the last tag of the page is where the next one resumes.
		var payload struct {
			Tags []string `json:"tags"`
		}
		_ = json.Unmarshal([]byte(page), &payload)
		if len(payload.Tags) > 0 {
			last := payload.Tags[len(payload.Tags)-1]
			if _, more := pages[last]; more {
				w.Header().Set("Link", fmt.Sprintf(`<%s?last=%s&n=0>; rel="next"`, r.URL.Path, last))
			}
		}
		_, _ = w.Write([]byte(page))
	}))
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://") + "/team/tool", tagListingCounters{tokens: &tokens}
}
