// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"testing"
)

// form450Schema mirrors the shape Lawmatics returns for
// GET /v1/forms/{uuid}?fields=all: a JSON:API envelope whose attributes
// carry rows of components. It includes the cases that make flattening
// non-trivial — a layout component, a nested relationship block, and a
// component repeated across conditional rows.
const form450Schema = `{
  "data": {
    "id": "38d8ec11-87cd-41d6-b983-f025cfec8926",
    "type": "custom_form",
    "attributes": {
      "name": "0450 - EP Consult + Design Decisions",
      "rows": [
        {"components": [
          {"component_type": "page_divider", "field_type": "string", "label": "Matter + Team",
           "required": false, "id": "divider-1", "default_value": null}
        ]},
        {"components": [
          {"component_type": "instructions", "label": "<h2>Quick Reference:</h2><p>Client:&nbsp;<span>|full-name|</span></p>",
           "required": false, "id": "instructions-1"}
        ]},
        {"components": [
          {"component_type": "field", "field_type": "string", "label": "First name", "required": true,
           "id": "field-first-name", "simplified_id": "first_name", "default_value": null},
          {"component_type": "field", "field_type": "list", "label": "Marital Status", "required": false,
           "id": "field-marital-status", "simplified_id": "marital_status",
           "list_options": ["Single", "Married"], "default_value": "Single"}
        ]},
        {"components": [
          {"component_type": "field", "field_type": "relationship_block", "label": "Spouse/Partner",
           "required": false, "id": "block-spouse", "default_value": null,
           "rows": [{"components": [
             {"component_type": "general_field", "field_type": "boolean", "label": "Show firm team instructions?",
              "required": false, "id": "general-show-instructions", "default_value": false}
           ]}]}
        ]},
        {"components": [
          {"component_type": "field", "field_type": "string", "label": "First name", "required": true,
           "id": "field-first-name", "simplified_id": "first_name", "default_value": null}
        ]}
      ]
    }
  }
}`

func TestFlattenFormFields(t *testing.T) {
	t.Parallel()
	fields, err := flattenFormFields(json.RawMessage(form450Schema), false)
	if err != nil {
		t.Fatalf("flattenFormFields: %v", err)
	}

	wantIDs := []string{"field-first-name", "field-marital-status", "block-spouse", "general-show-instructions"}
	if len(fields) != len(wantIDs) {
		t.Fatalf("got %d fields, want %d: %+v", len(fields), len(wantIDs), fields)
	}
	for i, want := range wantIDs {
		if fields[i].ID != want {
			t.Errorf("field %d: got id %q, want %q", i, fields[i].ID, want)
		}
	}

	first := fields[0]
	if first.Label != "First name" || !first.Required || first.FieldType != "string" ||
		first.ComponentType != "field" || first.SimplifiedID != "first_name" {
		t.Errorf("first field projected wrong: %+v", first)
	}
	if got := fields[1].Options; len(got) != 2 || got[0] != "Single" {
		t.Errorf("list options lost: %+v", fields[1])
	}
	if fields[1].DefaultValue != "Single" {
		t.Errorf("default value lost: %+v", fields[1])
	}
	if fields[3].ComponentType != "general_field" {
		t.Errorf("nested general_field not reached: %+v", fields[3])
	}
}

// Layout components are what an agent rebuilding a form needs, and noise
// for an agent filling one in, so --all-components has to be the only way
// to see them.
func TestFlattenFormFieldsIncludeAll(t *testing.T) {
	t.Parallel()
	fields, err := flattenFormFields(json.RawMessage(form450Schema), true)
	if err != nil {
		t.Fatalf("flattenFormFields: %v", err)
	}
	if len(fields) != 6 {
		t.Fatalf("got %d components, want 6: %+v", len(fields), fields)
	}
	if fields[0].ComponentType != "page_divider" {
		t.Errorf("layout component missing: %+v", fields[0])
	}
	if got, want := fields[1].Label, "Quick Reference: Client: |full-name|"; got != want {
		t.Errorf("html label not flattened: got %q, want %q", got, want)
	}
}

// Flattening must be stable: an agent that diffs two runs of
// `forms fields` should see field order changes only when the form changed.
func TestFlattenFormFieldsIsDeterministic(t *testing.T) {
	t.Parallel()
	first, err := flattenFormFields(json.RawMessage(form450Schema), true)
	if err != nil {
		t.Fatalf("flattenFormFields: %v", err)
	}
	for i := 0; i < 20; i++ {
		again, err := flattenFormFields(json.RawMessage(form450Schema), true)
		if err != nil {
			t.Fatalf("flattenFormFields: %v", err)
		}
		for j := range first {
			if first[j].ID != again[j].ID {
				t.Fatalf("run %d diverged at %d: %q vs %q", i, j, first[j].ID, again[j].ID)
			}
		}
	}
}

func TestFlattenFormFieldsRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := flattenFormFields(json.RawMessage(`not json`), false); err == nil {
		t.Fatal("expected an error for a non-JSON schema")
	}
}

func TestCleanFormLabel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"First name": "First name",
		"<h2>Quick Reference:</h2><p><strong>Client(s):</strong><br>Client:&nbsp;Ada</p>": "Quick Reference: Client(s): Client: Ada",
		"  spaced   out  ": "spaced out",
		"":                 "",
	}
	for input, want := range cases {
		if got := cleanFormLabel(input); got != want {
			t.Errorf("cleanFormLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFilterFormFields(t *testing.T) {
	t.Parallel()
	fields, err := flattenFormFields(json.RawMessage(form450Schema), false)
	if err != nil {
		t.Fatalf("flattenFormFields: %v", err)
	}
	if got := filterFormFields(fields, "", true); len(got) != 1 || got[0].ID != "field-first-name" {
		t.Errorf("--required-only: %+v", got)
	}
	if got := filterFormFields(fields, "MARITAL", false); len(got) != 1 || got[0].ID != "field-marital-status" {
		t.Errorf("label filter is not case-insensitive: %+v", got)
	}
	if got := filterFormFields(fields, "general-show", false); len(got) != 1 {
		t.Errorf("filter should match on id too: %+v", got)
	}
	if got := filterFormFields(fields, "nothing-matches", false); len(got) != 0 {
		t.Errorf("expected no matches, got %+v", got)
	}
}

// The template is the handoff between `forms fields` and `forms submit`:
// every fillable id must be present, keyed exactly as the submit body
// expects, carrying the form's own default where it declares one.
func TestBuildFieldTemplate(t *testing.T) {
	t.Parallel()
	fields, err := flattenFormFields(json.RawMessage(form450Schema), false)
	if err != nil {
		t.Fatalf("flattenFormFields: %v", err)
	}
	template := buildFieldTemplate(fields)
	if len(template) != len(fields) {
		t.Fatalf("template has %d keys, want %d", len(template), len(fields))
	}
	if template["field-first-name"] != "" {
		t.Errorf("unset string field should be empty, got %#v", template["field-first-name"])
	}
	if template["field-marital-status"] != "Single" {
		t.Errorf("declared default lost: %#v", template["field-marital-status"])
	}
	if template["general-show-instructions"] != false {
		t.Errorf("boolean default lost: %#v", template["general-show-instructions"])
	}
}

func TestParseFormList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"json:api envelope", `{"data":[{"id":"u1","type":"custom_form","attributes":{"name":"0450 - EP Consult","created_at":"2026-01-01"}}]}`},
		{"bare array", `[{"id":"u1","name":"0450 - EP Consult","created_at":"2026-01-01"}]`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			forms, err := parseFormList(json.RawMessage(tc.input))
			if err != nil {
				t.Fatalf("parseFormList: %v", err)
			}
			if len(forms) != 1 || forms[0].ID != "u1" || forms[0].Name != "0450 - EP Consult" ||
				forms[0].CreatedAt != "2026-01-01" {
				t.Fatalf("wrong projection: %+v", forms)
			}
		})
	}
}
