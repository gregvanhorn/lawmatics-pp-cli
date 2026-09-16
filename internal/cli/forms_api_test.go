// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
)

// fakeFetcher replays canned pages and records the query parameters each
// page was requested with.
type fakeFetcher struct {
	pages    []string
	requests []map[string]string
}

func (f *fakeFetcher) GetWithHeaders(path string, params map[string]string, headers map[string]string) (json.RawMessage, error) {
	f.requests = append(f.requests, copyParams(params))
	if len(f.requests) > len(f.pages) {
		return nil, fmt.Errorf("unexpected request %d for %s", len(f.requests), path)
	}
	return json.RawMessage(f.pages[len(f.requests)-1]), nil
}

func TestFetchFormCollectionFollowsLinksNext(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{pages: []string{
		`{"data":[{"id":"1"},{"id":"2"}],"links":{"next":"https://api.lawmatics.com/v1/forms?page=2&per_page=2"}}`,
		`{"data":[{"id":"3"}],"links":{"next":null}}`,
	}}
	items, pages, err := fetchFormCollection(fetcher, "/forms", map[string]string{"per_page": "2"}, true)
	if err != nil {
		t.Fatalf("fetchFormCollection: %v", err)
	}
	if len(items) != 3 || pages != 2 {
		t.Fatalf("got %d items across %d pages", len(items), pages)
	}
	if got := fetcher.requests[1]["page"]; got != "2" {
		t.Fatalf("second request did not follow links.next: %v", fetcher.requests[1])
	}
	if got := fetcher.requests[1]["per_page"]; got != "2" {
		t.Fatalf("caller params were dropped on page 2: %v", fetcher.requests[1])
	}
}

func TestFetchFormCollectionFollowsMetaPageCounts(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{pages: []string{
		`{"data":[{"id":"1"}],"meta":{"current_page":1,"total_pages":3}}`,
		`{"data":[{"id":"2"}],"meta":{"current_page":2,"total_pages":3}}`,
		`{"data":[{"id":"3"}],"meta":{"current_page":3,"total_pages":3}}`,
	}}
	items, pages, err := fetchFormCollection(fetcher, "/forms", nil, true)
	if err != nil {
		t.Fatalf("fetchFormCollection: %v", err)
	}
	if len(items) != 3 || pages != 3 {
		t.Fatalf("got %d items across %d pages", len(items), pages)
	}
}

// Without --all only the first page is fetched — the truncation warning on
// stderr is what tells an agent it is looking at a partial result.
func TestFetchFormCollectionStopsAtFirstPageByDefault(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{pages: []string{
		`{"data":[{"id":"1"}],"meta":{"current_page":1,"total_pages":2}}`,
	}}
	items, pages, err := fetchFormCollection(fetcher, "/forms", nil, false)
	if err != nil {
		t.Fatalf("fetchFormCollection: %v", err)
	}
	if len(items) != 1 || pages != 1 {
		t.Fatalf("got %d items across %d pages", len(items), pages)
	}
}

// A pagination shape this code cannot advance must terminate rather than
// re-request the same page forever.
func TestFetchFormCollectionTerminatesOnUnchangedCursor(t *testing.T) {
	t.Parallel()
	fetcher := &fakeFetcher{pages: []string{
		`{"data":[{"id":"1"}],"links":{"next":"https://api.lawmatics.com/v1/forms"},"meta":{"current_page":1,"total_pages":1}}`,
	}}
	items, pages, err := fetchFormCollection(fetcher, "/forms", nil, true)
	if err != nil {
		t.Fatalf("fetchFormCollection: %v", err)
	}
	if len(items) != 1 || pages != 1 {
		t.Fatalf("got %d items across %d pages", len(items), pages)
	}
}

func TestNextFormPageParamsHonorsHrefObjects(t *testing.T) {
	t.Parallel()
	next := nextFormPageParams(json.RawMessage(`{"links":{"next":{"href":"/v1/forms?page=4"}}}`), map[string]string{})
	if next == nil || next["page"] != "4" {
		t.Fatalf("got %v", next)
	}
}

func TestNextFormPageParamsReturnsNilOnLastPage(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		`{"data":[],"links":{"next":null}}`,
		`{"data":[],"meta":{"current_page":3,"total_pages":3}}`,
		`{"data":[]}`,
		`[]`,
	} {
		if next := nextFormPageParams(json.RawMessage(body), map[string]string{}); next != nil {
			t.Errorf("%s: want nil, got %v", body, next)
		}
	}
}
