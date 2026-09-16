package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lawmatics-pp-cli/internal/store"
)

func TestContactsFindByNameLocalAndLive(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LAWMATICS_ACCESS_TOKEN", "test-only")
	db, err := store.Open(defaultDBPath("lawmatics-pp-cli"))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = db.UpsertBatch("contacts", []json.RawMessage{json.RawMessage(`{"id":"25","attributes":{"first_name":"Ada","last_name":"O'Neil/Smith","phone":"202-555-0101"}}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("contacts", "", 1); err != nil {
		t.Fatal(err)
	}
	db.Close()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != "GET" || r.URL.EscapedPath() != "/contacts/find_by_name/Ada%20O%27Neil%2FSmith" || r.URL.RawQuery != "" {
			t.Errorf("wrong request: %s %s", r.Method, r.URL.String())
		}
		fmt.Fprint(w, `{"data":{"id":"25","attributes":{"first_name":"Ada"}}}`)
	}))
	defer server.Close()
	t.Setenv("LAWMATICS_BASE_URL", server.URL)
	for _, source := range []string{"local", "live"} {
		cmd := newRootCmd(&rootFlags{})
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs([]string{"contacts", "find-by-name", "--name", "Ada O'Neil/Smith", "--data-source", source, "--agent", "--no-cache"})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Data json.RawMessage `json:"results"`
			Meta DataProvenance  `json:"meta"`
		}
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Meta.Source != source {
			t.Fatalf("wrong provenance: %s", output.String())
		}
		if source == "local" {
			if requests != 0 {
				t.Fatal("local lookup contacted API")
			}
			var matches []store.ContactMatch
			if err := json.Unmarshal(result.Data, &matches); err != nil {
				t.Fatal(err)
			}
			if len(matches) != 1 || len(matches[0].PhoneNumbers) != 1 || matches[0].PhoneNumbers[0] != "202-555-0101" || result.Meta.SyncedAt == nil || matches[0].PhoneNumbersSyncedAt != nil {
				t.Fatalf("local compact output lost phones/freshness: %s", output.String())
			}
		}
	}
	if requests != 1 {
		t.Fatalf("got %d requests", requests)
	}
	// Selection must still win over the local projection in agent mode.
	cmd := newRootCmd(&rootFlags{})
	var selected bytes.Buffer
	cmd.SetOut(&selected)
	cmd.SetArgs([]string{"contacts", "find-by-name", "--name", "Ada", "--data-source", "local", "--agent", "--select", "id,phone_numbers"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(selected.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Results) != 1 || len(projection.Results[0]) != 2 || projection.Results[0]["id"] != "25" || projection.Results[0]["phone_numbers"] == nil {
		t.Fatalf("wrong selection: %s", selected.String())
	}

	server.Close()
	cmd = newRootCmd(&rootFlags{})
	var fallback bytes.Buffer
	cmd.SetOut(&fallback)
	cmd.SetArgs([]string{"contacts", "find-by-name", "--name", "Ada O'Neil/Smith", "--agent", "--no-cache"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fallback.String(), `"api_unreachable"`) || !strings.Contains(fallback.String(), `"202-555-0101"`) {
		t.Fatalf("wrong fallback: %s", fallback.String())
	}

	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer denied.Close()
	t.Setenv("LAWMATICS_BASE_URL", denied.URL)
	cmd = newRootCmd(&rootFlags{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"contacts", "find-by-name", "--name", "Ada", "--agent", "--no-cache"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("HTTP 404 must not be hidden by local fallback")
	}
	cmd = newRootCmd(&rootFlags{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"contacts", "find-by-name", "--name", "  ", "--data-source", "local", "--agent"})
	if err := cmd.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("blank input: %v", err)
	}
}
