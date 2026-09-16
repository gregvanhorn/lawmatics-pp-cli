// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"mime"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFieldAssignments(t *testing.T) {
	t.Parallel()
	values, err := parseFieldAssignments([]string{
		"field-first-name=Ada",
		"field-age=42",
		"field-active=true",
		"field-tags=[\"a\",\"b\"]",
		"field-zip=02134",
		"field-empty=",
		"field-equals=a=b",
	}, "--field", true)
	if err != nil {
		t.Fatalf("parseFieldAssignments: %v", err)
	}
	if values["field-first-name"] != "Ada" {
		t.Errorf("plain text should stay text: %#v", values["field-first-name"])
	}
	if got, ok := values["field-age"].(json.Number); !ok || got.String() != "42" {
		t.Errorf("number should decode as a number: %#v", values["field-age"])
	}
	if values["field-active"] != true {
		t.Errorf("boolean should decode as a boolean: %#v", values["field-active"])
	}
	if got, ok := values["field-tags"].([]any); !ok || len(got) != 2 {
		t.Errorf("array should decode as an array: %#v", values["field-tags"])
	}
	// A leading zero is not a valid JSON number, which is exactly the
	// behaviour a zip code needs.
	if values["field-zip"] != "02134" {
		t.Errorf("leading-zero digits should stay text: %#v", values["field-zip"])
	}
	if values["field-empty"] != "" {
		t.Errorf("empty value should stay an empty string: %#v", values["field-empty"])
	}
	if values["field-equals"] != "a=b" {
		t.Errorf("only the first = separates key from value: %#v", values["field-equals"])
	}
}

func TestParseFieldAssignmentsNoCoercion(t *testing.T) {
	t.Parallel()
	values, err := parseFieldAssignments([]string{"field-phone=2025550101", "field-flag=true"}, "--string", false)
	if err != nil {
		t.Fatalf("parseFieldAssignments: %v", err)
	}
	if values["field-phone"] != "2025550101" || values["field-flag"] != "true" {
		t.Fatalf("--string must never decode: %#v", values)
	}
}

func TestParseFieldAssignmentsRejectsMalformed(t *testing.T) {
	t.Parallel()
	for _, entry := range []string{"no-equals-sign", "=orphan-value"} {
		if _, err := parseFieldAssignments([]string{entry}, "--field", true); err == nil {
			t.Errorf("%q should be rejected", entry)
		}
	}
}

func TestCoerceFieldValueRejectsTrailingContent(t *testing.T) {
	t.Parallel()
	if got := coerceFieldValue("12 monkeys"); got != "12 monkeys" {
		t.Fatalf("got %#v, want the original text", got)
	}
}

func TestBuildSubmitValues(t *testing.T) {
	t.Parallel()
	base := map[string]any{"a": "from-file", "b": "from-file"}
	overrides := map[string]any{"b": "from-flag", "c": "from-flag"}
	values := buildSubmitValues(base, overrides, "")
	if values["a"] != "from-file" || values["b"] != "from-flag" || values["c"] != "from-flag" {
		t.Fatalf("--field should win over the file body: %#v", values)
	}
	if len(base) != 2 {
		t.Fatalf("the caller's base map was mutated: %#v", base)
	}
}

// --wrap has to be idempotent against a body that is already wrapped,
// otherwise re-submitting a saved payload nests it twice and the API sees
// no fields at all.
func TestBuildSubmitValuesUnwrapsWrappedBase(t *testing.T) {
	t.Parallel()
	base := map[string]any{"fields": map[string]any{"a": "1"}}
	values := buildSubmitValues(base, map[string]any{"b": "2"}, "fields")
	if values["a"] != "1" || values["b"] != "2" {
		t.Fatalf("wrapped base was not unwound: %#v", values)
	}
	payload := wrapSubmitValues(values, "fields")
	nested, ok := payload["fields"].(map[string]any)
	if !ok || len(payload) != 1 || nested["a"] != "1" {
		t.Fatalf("wrap produced %#v", payload)
	}
}

func TestWrapSubmitValuesDefaultsToFlatBody(t *testing.T) {
	t.Parallel()
	values := map[string]any{"a": "1"}
	if got := wrapSubmitValues(values, ""); got["a"] != "1" || len(got) != 1 {
		t.Fatalf("without --wrap the body must stay flat: %#v", got)
	}
}

func TestValidateSubmission(t *testing.T) {
	t.Parallel()
	fields := []formField{
		{ID: "field-first-name", SimplifiedID: "first_name", Label: "First name", Required: true},
		{ID: "field-nickname", Label: "Nickname"},
	}

	unknown, missing := validateSubmission(map[string]any{"field-first-name": "Ada"}, fields)
	if len(unknown) != 0 || len(missing) != 0 {
		t.Fatalf("valid submission rejected: unknown=%v missing=%v", unknown, missing)
	}

	// The simplified id is as legitimate a key as the component id.
	if _, missing = validateSubmission(map[string]any{"first_name": "Ada"}, fields); len(missing) != 0 {
		t.Fatalf("simplified id not accepted for a required field: %v", missing)
	}

	unknown, missing = validateSubmission(map[string]any{"field-typo": "x", "field-first-name": "  "}, fields)
	if len(unknown) != 1 || unknown[0] != "field-typo" {
		t.Fatalf("unknown ids: %v", unknown)
	}
	if len(missing) != 1 || missing[0].ID != "field-first-name" {
		t.Fatalf("a whitespace-only value should not satisfy a required field: %v", missing)
	}

	// false and 0 are real answers to a required checkbox or number.
	if _, missing = validateSubmission(map[string]any{"field-first-name": false}, fields); len(missing) != 0 {
		t.Fatalf("false should satisfy a required field: %v", missing)
	}
}

func TestBuildMultipartBody(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "will.txt")
	if err := os.WriteFile(path, []byte("last will"), 0o600); err != nil {
		t.Fatal(err)
	}

	body, contentType, err := buildMultipartBody(map[string]any{
		"field-first-name": "Ada",
		"field-age":        json.Number("42"),
		"field-active":     true,
		"field-tags":       []any{"a", "b"},
	}, []formFileUpload{{Key: "field-upload", Path: path}})
	if err != nil {
		t.Fatalf("buildMultipartBody: %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type %q: %v", contentType, err)
	}
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	form, err := reader.ReadForm(1 << 20)
	if err != nil {
		t.Fatalf("reading back the multipart body: %v", err)
	}
	want := map[string]string{
		"field-first-name": "Ada",
		"field-age":        "42",
		"field-active":     "true",
		"field-tags":       `["a","b"]`,
	}
	for key, value := range want {
		if got := form.Value[key]; len(got) != 1 || got[0] != value {
			t.Errorf("field %s = %v, want %q", key, got, value)
		}
	}
	files := form.File["field-upload"]
	if len(files) != 1 || files[0].Filename != "will.txt" {
		t.Fatalf("file part missing: %+v", files)
	}
	file, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents := make([]byte, 9)
	if _, err := file.Read(contents); err != nil {
		t.Fatal(err)
	}
	if string(contents) != "last will" {
		t.Fatalf("file contents = %q", contents)
	}
}

func TestBuildMultipartBodyReportsMissingFile(t *testing.T) {
	t.Parallel()
	_, _, err := buildMultipartBody(nil, []formFileUpload{{Key: "k", Path: filepath.Join(t.TempDir(), "absent")}})
	if err == nil {
		t.Fatal("expected an error for a missing upload")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("a bad --file path is a usage error; got exit code %d", ExitCode(err))
	}
}

func TestDecodeSubmitJSON(t *testing.T) {
	t.Parallel()
	body, err := decodeSubmitJSON([]byte(`{"field-zip":"02134","field-age":42}`))
	if err != nil {
		t.Fatalf("decodeSubmitJSON: %v", err)
	}
	if body["field-zip"] != "02134" {
		t.Errorf("string preserved: %#v", body["field-zip"])
	}
	if got, ok := body["field-age"].(json.Number); !ok || got.String() != "42" {
		t.Errorf("numbers should round-trip exactly: %#v", body["field-age"])
	}
	if empty, err := decodeSubmitJSON([]byte("  ")); err != nil || len(empty) != 0 {
		t.Errorf("blank input: %v %v", empty, err)
	}
	if _, err := decodeSubmitJSON([]byte("[1,2]")); err == nil {
		t.Error("a JSON array is not a submission body")
	}
}
