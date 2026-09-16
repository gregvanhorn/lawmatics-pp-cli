// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const form450UUID = "38d8ec11-87cd-41d6-b983-f025cfec8926"

// formsTestServer stands in for the Lawmatics forms API: a paginated form
// list, a schema that only carries rows when fields=all is sent, and a
// submit endpoint that records what it received.
type formsTestServer struct {
	*httptest.Server
	listRequests   int
	schemaRequests []string
	submissions    []map[string]any
	submitHeaders  []http.Header
	submitStatus   int
}

func newFormsTestServer(t *testing.T) *formsTestServer {
	t.Helper()
	fake := &formsTestServer{submitStatus: http.StatusOK}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/forms":
			fake.listRequests++
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "2" {
				_, _ = w.Write([]byte(`{"data":[
                  {"id":"aaaaaaaa-0000-0000-0000-000000000003","type":"custom_form","attributes":{"name":"Client Feedback"}}
                ],"meta":{"current_page":2,"total_pages":2}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[
              {"id":"` + form450UUID + `","type":"custom_form","attributes":{"name":"0450 - EP Consult + Design Decisions"}},
              {"id":"aaaaaaaa-0000-0000-0000-000000000001","type":"custom_form","attributes":{"name":"0451 - EP Consult Follow Up"}}
            ],"meta":{"current_page":1,"total_pages":2}}`))

		case r.Method == http.MethodGet && r.URL.Path == "/forms/"+form450UUID:
			fake.schemaRequests = append(fake.schemaRequests, r.URL.Query().Get("fields"))
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("fields") == "" {
				_, _ = w.Write([]byte(`{"data":{"id":"` + form450UUID + `","attributes":{"name":"0450 - EP Consult + Design Decisions"}}}`))
				return
			}
			_, _ = w.Write([]byte(form450Schema))

		case r.Method == http.MethodGet && r.URL.Path == "/forms/"+form450UUID+"/entries":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"entry-1","type":"custom_form_entry"}],"meta":{"current_page":1,"total_pages":1}}`))

		case r.Method == http.MethodPost && r.URL.Path == "/forms/"+form450UUID+"/submit":
			body := map[string]any{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			fake.submissions = append(fake.submissions, body)
			fake.submitHeaders = append(fake.submitHeaders, r.Header.Clone())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(fake.submitStatus)
			_, _ = w.Write([]byte(`{"data":{"id":"entry-99","type":"custom_form_entry"}}`))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(fake.Close)
	return fake
}

// runForms executes the CLI in-process and returns stdout.
func runForms(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newRootCmd(&rootFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append(args, "--no-cache"))
	err := cmd.Execute()
	return out.String(), err
}

func setupFormsEnv(t *testing.T, server *formsTestServer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("LAWMATICS_ACCESS_TOKEN", "test-only")
	t.Setenv("LAWMATICS_BASE_URL", server.URL)
}

// The headline flow: name the form the way a human does, get back the
// fillable keys, then submit a body keyed by those ids.
func TestFormsFieldsThenSubmit(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)

	out, err := runForms(t, "forms", "fields", "Form 450", "--agent")
	if err != nil {
		t.Fatalf("forms fields: %v", err)
	}
	var fields []map[string]any
	if err := json.Unmarshal([]byte(out), &fields); err != nil {
		t.Fatalf("parsing %s: %v", out, err)
	}
	if len(fields) != 4 || fields[0]["id"] != "field-first-name" || fields[0]["required"] != true {
		t.Fatalf("unexpected fields: %s", out)
	}
	if len(server.schemaRequests) != 1 || server.schemaRequests[0] != "all" {
		t.Fatalf("the schema must be requested with fields=all: %v", server.schemaRequests)
	}
	if server.listRequests != 2 {
		t.Fatalf("name resolution should page through the form list, got %d requests", server.listRequests)
	}

	template, err := runForms(t, "forms", "fields", "0450", "--template", "--json")
	if err != nil {
		t.Fatalf("forms fields --template: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(template), &body); err != nil {
		t.Fatalf("parsing %s: %v", template, err)
	}
	if len(body) != 4 || body["field-marital-status"] != "Single" {
		t.Fatalf("unexpected template: %s", template)
	}

	body["field-first-name"] = "Ada"
	filled := filepath.Join(t.TempDir(), "filled.json")
	encoded, _ := json.Marshal(body)
	if err := os.WriteFile(filled, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err = runForms(t, "forms", "submit", "450", "--json-file", filled,
		"--field", "field-marital-status=Married", "--agent")
	if err != nil {
		t.Fatalf("forms submit: %v", err)
	}
	if len(server.submissions) != 1 {
		t.Fatalf("expected exactly one submission, got %d", len(server.submissions))
	}
	submitted := server.submissions[0]
	if submitted["field-first-name"] != "Ada" {
		t.Errorf("body value lost: %#v", submitted)
	}
	if submitted["field-marital-status"] != "Married" {
		t.Errorf("--field should win over the file body: %#v", submitted)
	}
	if len(submitted) != 4 {
		t.Errorf("submitted %d keys, want the form's 4: %#v", len(submitted), submitted)
	}
	var envelope struct {
		Success bool   `json:"success"`
		Status  int    `json:"status"`
		FormID  string `json:"form_id"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("parsing %s: %v", out, err)
	}
	if !envelope.Success || envelope.Status != 200 || envelope.FormID != form450UUID {
		t.Fatalf("unexpected envelope: %s", out)
	}
}

// Validation is the difference between a typo failing locally with a
// readable message and a 422 from the API after the automations fired.
func TestFormsSubmitValidatesAgainstTheForm(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)

	_, err := runForms(t, "forms", "submit", form450UUID, "--field", "field-typo=x", "--agent")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("an unknown field id should be a usage error, got %v", err)
	}
	if !strings.Contains(err.Error(), "field-typo") {
		t.Errorf("the error should name the bad id: %v", err)
	}

	_, err = runForms(t, "forms", "submit", form450UUID, "--field", "field-marital-status=Married", "--agent")
	if err == nil || !strings.Contains(err.Error(), "First name") {
		t.Fatalf("a missing required field should be reported by label, got %v", err)
	}

	if len(server.submissions) != 0 {
		t.Fatalf("nothing should reach the API when validation fails: %+v", server.submissions)
	}

	// --no-validate is the escape hatch when the schema and the endpoint
	// disagree; the API becomes the only authority.
	if _, err := runForms(t, "forms", "submit", form450UUID,
		"--field", "field-typo=x", "--no-validate", "--agent"); err != nil {
		t.Fatalf("--no-validate: %v", err)
	}
	if len(server.submissions) != 1 {
		t.Fatalf("--no-validate should submit anyway: %+v", server.submissions)
	}
}

func TestFormsSubmitRequiresValues(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	_, err := runForms(t, "forms", "submit", form450UUID, "--agent")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("an empty submission should be refused locally, got %v", err)
	}
	if len(server.submissions) != 0 {
		t.Fatal("an empty submission reached the API")
	}
}

func TestFormsSubmitNoAuthOmitsTheToken(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	if _, err := runForms(t, "forms", "submit", form450UUID,
		"--field", "field-first-name=Ada", "--no-auth", "--agent"); err != nil {
		t.Fatalf("forms submit --no-auth: %v", err)
	}
	if got := server.submitHeaders[0].Get("Authorization"); got != "" {
		t.Fatalf("--no-auth still sent %q", got)
	}

	if _, err := runForms(t, "forms", "submit", form450UUID,
		"--field", "field-first-name=Ada", "--agent"); err != nil {
		t.Fatalf("forms submit: %v", err)
	}
	if got := server.submitHeaders[1].Get("Authorization"); got != "Bearer test-only" {
		t.Fatalf("a configured token should be sent by default, got %q", got)
	}
}

func TestFormsSubmitDryRunSendsNothing(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	if _, err := runForms(t, "forms", "submit", "0450",
		"--field", "field-first-name=Ada", "--dry-run", "--agent"); err != nil {
		t.Fatalf("forms submit --dry-run: %v", err)
	}
	if len(server.submissions) != 0 || server.listRequests != 0 {
		t.Fatalf("--dry-run touched the API: %d submissions, %d list requests",
			len(server.submissions), server.listRequests)
	}
}

// A uuid is unambiguous by construction, so resolving one must not cost a
// list call — the account has hundreds of forms across several pages.
func TestFormsGetByUUIDSkipsNameResolution(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	out, err := runForms(t, "forms", "get", form450UUID, "--agent")
	if err != nil {
		t.Fatalf("forms get: %v", err)
	}
	if server.listRequests != 0 {
		t.Fatalf("resolving a uuid cost %d list requests", server.listRequests)
	}
	if len(server.schemaRequests) != 1 || server.schemaRequests[0] != "all" {
		t.Fatalf("fields=all should be the default: %v", server.schemaRequests)
	}
	if !strings.Contains(out, "0450 - EP Consult") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestFormsGetAmbiguousReferenceFailsClosed(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	_, err := runForms(t, "forms", "get", "EP Consult", "--agent")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("an ambiguous name should be a usage error, got %v", err)
	}
	if len(server.schemaRequests) != 0 {
		t.Fatal("an ambiguous reference should not fetch a form")
	}
}

func TestFormsGetUnknownReferenceIsNotFound(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	_, err := runForms(t, "forms", "get", "no such form", "--agent")
	if err == nil || ExitCode(err) != 3 {
		t.Fatalf("an unknown name should exit 3 (not found), got %v", err)
	}
}

func TestFormsListAndEntries(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)

	out, err := runForms(t, "forms", "list", "--all", "--agent")
	if err != nil {
		t.Fatalf("forms list: %v", err)
	}
	var listed struct {
		Results []formSummary `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("parsing %s: %v", out, err)
	}
	if len(listed.Results) != 3 {
		t.Fatalf("--all should return every page: %s", out)
	}

	out, err = runForms(t, "forms", "list", "--filter", "0450", "--agent")
	if err != nil {
		t.Fatalf("forms list --filter: %v", err)
	}
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("parsing %s: %v", out, err)
	}
	if len(listed.Results) != 1 || listed.Results[0].ID != form450UUID {
		t.Fatalf("--filter did not narrow the list: %s", out)
	}

	out, err = runForms(t, "forms", "entries", "0450", "--agent")
	if err != nil {
		t.Fatalf("forms entries: %v", err)
	}
	if !strings.Contains(out, "entry-1") {
		t.Fatalf("unexpected entries output: %s", out)
	}
}

// Forms are not part of the sync surface, so --data-source local would
// silently return nothing; it has to be refused instead.
func TestFormsRejectLocalDataSource(t *testing.T) {
	server := newFormsTestServer(t)
	setupFormsEnv(t, server)
	_, err := runForms(t, "forms", "list", "--data-source", "local", "--agent")
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("want a usage error, got %v", err)
	}
}

// `api` is where an agent audits the endpoint surface, so a
// hand-maintained interface has to show up there alongside the generated
// ones even though it is also a first-class visible command.
func TestAPIIndexIncludesForms(t *testing.T) {
	out, err := runForms(t, "api", "--json")
	if err != nil {
		t.Fatalf("api: %v", err)
	}
	var index struct {
		Interfaces []struct {
			Name string `json:"name"`
		} `json:"interfaces"`
	}
	if err := json.Unmarshal([]byte(out), &index); err != nil {
		t.Fatalf("parsing %s: %v", out, err)
	}
	found := map[string]bool{}
	for _, iface := range index.Interfaces {
		found[iface.Name] = true
	}
	for _, want := range []string{"forms", "campaigns", "sources"} {
		if !found[want] {
			t.Errorf("api index is missing %q", want)
		}
	}

	out, err = runForms(t, "api", "forms", "--json")
	if err != nil {
		t.Fatalf("api forms: %v", err)
	}
	for _, method := range []string{"list", "get", "fields", "entries", "submit"} {
		if !strings.Contains(out, `"`+method+`"`) {
			t.Errorf("api forms is missing the %q method: %s", method, out)
		}
	}
}
