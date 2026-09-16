// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"lawmatics-pp-cli/internal/client"
)

const (
	formsBasePath = "/forms"
	// formsMaxPages bounds --all so a pagination shape this code does not
	// understand degrades into a truncated result instead of an endless loop.
	formsMaxPages = 500
)

// formsFetcher is the slice of the API client the forms commands read
// through. Declaring it here keeps pagination unit-testable without an HTTP
// server.
type formsFetcher interface {
	GetWithHeaders(path string, params map[string]string, headers map[string]string) (json.RawMessage, error)
}

// formsLiveOnly rejects --data-source local for the forms commands. Forms are
// not part of the sync surface, so a local read would silently return
// nothing rather than the stale-but-honest data the flag promises.
func formsLiveOnly(flags *rootFlags) error {
	if flags != nil && flags.dataSource == "local" {
		return usageErr(fmt.Errorf("forms are read live from the API; --data-source local is not supported"))
	}
	return nil
}

// fetchFormCollection reads a forms collection endpoint, following
// pagination when fetchAll is set. It returns the concatenated items and the
// number of pages fetched.
func fetchFormCollection(c formsFetcher, path string, params map[string]string, fetchAll bool) ([]json.RawMessage, int, error) {
	pageParams := map[string]string{}
	for k, v := range params {
		if v != "" {
			pageParams[k] = v
		}
	}

	var all []json.RawMessage
	pages := 0
	for {
		pages++
		data, err := c.GetWithHeaders(path, pageParams, nil)
		if err != nil {
			return nil, pages, err
		}
		items, err := formItems(data)
		if err != nil {
			return nil, pages, err
		}
		all = append(all, items...)

		next := nextFormPageParams(data, pageParams)
		if !fetchAll {
			if next != nil {
				emitFormsTruncationWarning()
			}
			return all, pages, nil
		}
		if next == nil || len(items) == 0 || pages >= formsMaxPages {
			return all, pages, nil
		}
		pageParams = next
		emitFormsPageEvent(pages + 1)
	}
}

// nextFormPageParams derives the query parameters for the page after the one
// in data, or nil when data is the last page. Lawmatics returns a JSON:API
// `links.next` URL on some collections and `meta.current_page`/
// `meta.total_pages` on others, so both are honored.
func nextFormPageParams(data json.RawMessage, current map[string]string) map[string]string {
	var envelope struct {
		Links struct {
			Next json.RawMessage `json:"next"`
		} `json:"links"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil
	}

	if nextURL := stringOrHref(envelope.Links.Next); nextURL != "" {
		if parsed, err := url.Parse(nextURL); err == nil {
			next := copyParams(current)
			for key, values := range parsed.Query() {
				if len(values) > 0 {
					next[key] = values[0]
				}
			}
			if !sameParams(next, current) {
				return next
			}
		}
	}

	currentPage := intFromMeta(envelope.Meta, "current_page", "page", "page_number")
	totalPages := intFromMeta(envelope.Meta, "total_pages", "page_count", "last_page")
	if currentPage > 0 && totalPages > currentPage {
		next := copyParams(current)
		next["page"] = strconv.Itoa(currentPage + 1)
		return next
	}
	return nil
}

// stringOrHref reads a JSON:API link, which is either a URL string or an
// object carrying an href.
func stringOrHref(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return asString
	}
	var asObject struct {
		Href string `json:"href"`
	}
	if json.Unmarshal(raw, &asObject) == nil {
		return asObject.Href
	}
	return ""
}

func intFromMeta(meta map[string]any, keys ...string) int {
	for _, key := range keys {
		switch v := meta[key].(type) {
		case float64:
			return int(v)
		case string:
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

func copyParams(params map[string]string) map[string]string {
	out := make(map[string]string, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	return out
}

func sameParams(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func emitFormsPageEvent(page int) {
	if humanFriendly {
		fmt.Fprintf(os.Stderr, "fetching page %d...\n", page)
		return
	}
	fmt.Fprintf(os.Stderr, `{"event":"page_fetch","page":%d}`+"\n", page)
}

func emitFormsTruncationWarning() {
	if humanFriendly {
		fmt.Fprintln(os.Stderr, "warning: results truncated; more pages available. Re-run with --all to fetch every page.")
		return
	}
	fmt.Fprintln(os.Stderr, `{"event":"truncated","hint":"pass --all to fetch every page"}`)
}

// listForms returns every form the account exposes, for name resolution and
// for `forms list`.
func listForms(c formsFetcher, params map[string]string, fetchAll bool) ([]formSummary, []json.RawMessage, error) {
	items, _, err := fetchFormCollection(c, formsBasePath, params, fetchAll)
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, nil, err
	}
	forms, err := parseFormList(raw)
	if err != nil {
		return nil, nil, err
	}
	return forms, items, nil
}

// resolveFormTarget turns a CLI argument into the form it names. UUIDs skip
// the network entirely; anything else is matched against the account's form
// list by resolveFormRef.
func resolveFormTarget(c *client.Client, flags *rootFlags, ref string) (formSummary, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return formSummary{}, usageErr(fmt.Errorf("a form uuid or name is required"))
	}
	if looksLikeFormUUID(trimmed) {
		return formSummary{ID: trimmed}, nil
	}
	if flags != nil && flags.dryRun {
		// --dry-run never reaches the API, so there is no form list to match
		// against; preview the request with the reference as given.
		fmt.Fprintf(os.Stderr, "dry run: skipping name resolution for %q\n", trimmed)
		return formSummary{ID: trimmed, Name: trimmed}, nil
	}
	forms, _, err := listForms(c, map[string]string{}, true)
	if err != nil {
		return formSummary{}, classifyAPIError(err, flags)
	}
	form, err := resolveFormRef(trimmed, forms)
	if err != nil {
		var ambiguous *formAmbiguousError
		if As(err, &ambiguous) {
			return formSummary{}, usageErr(err)
		}
		return formSummary{}, notFoundErr(err)
	}
	return form, nil
}

// formPath builds a path under a specific form, percent-encoding the id.
func formPath(formID string, suffix ...string) string {
	path := formsBasePath + "/" + url.PathEscape(formID)
	for _, part := range suffix {
		path += "/" + part
	}
	return path
}

// printFormsPayload renders a read response the same way generated endpoint
// commands do: provenance-wrapped JSON for agents and piped consumers, a
// table for humans at a terminal.
func printFormsPayload(cmd *cobra.Command, flags *rootFlags, data json.RawMessage) error {
	prov := attachFreshness(DataProvenance{Source: "live", ResourceType: "forms"}, flags)
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		var countItems []json.RawMessage
		if json.Unmarshal(data, &countItems) != nil {
			countItems = []json.RawMessage{data}
		}
		printProvenance(cmd, len(countItems), prov)
	}
	if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain) {
		filtered := data
		if flags.selectFields != "" {
			filtered = filterFields(filtered, flags.selectFields)
		} else if flags.compact {
			filtered = compactFields(filtered)
		}
		wrapped, err := wrapWithProvenance(filtered, prov)
		if err != nil {
			return err
		}
		return printOutput(cmd.OutOrStdout(), wrapped, true)
	}
	if wantsHumanTable(cmd.OutOrStdout(), flags) {
		var items []map[string]any
		if json.Unmarshal(data, &items) == nil && len(items) > 0 {
			return printAutoTable(cmd.OutOrStdout(), items)
		}
	}
	return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
}
