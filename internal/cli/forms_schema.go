// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
)

// formSummary is the projection of a Lawmatics custom form used for listing
// and for name resolution. Lawmatics serves forms as JSON:API resources
// ({"id":…,"attributes":{"name":…}}) but the same fields also appear flat on
// some responses, so both shapes are accepted.
type formSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// formField is one fillable component of a custom form: the unit a caller
// needs in order to build a submission body. ID is the Lawmatics component
// id used as the submit payload key; SimplifiedID is the human-readable
// alias Lawmatics returns for standard fields (first_name, practice_area).
type formField struct {
	ID            string `json:"id"`
	SimplifiedID  string `json:"simplified_id,omitempty"`
	Label         string `json:"label"`
	FieldType     string `json:"field_type"`
	Required      bool   `json:"required"`
	ComponentType string `json:"component_type"`
	DefaultValue  any    `json:"default_value,omitempty"`
	Options       []any  `json:"options,omitempty"`
}

// fillableComponentTypes are the component types a caller can submit a value
// for. Everything else in a form's rows is layout or copy: page_divider,
// instructions, signature blocks, and so on.
var fillableComponentTypes = map[string]bool{
	"field":         true,
	"general_field": true,
}

// componentChildKeys are traversed first, and in this order, when walking a
// form definition. Remaining keys are visited in sorted order so flattening
// is deterministic despite Go's randomized map iteration.
var componentChildKeys = []string{"rows", "components", "columns", "children", "fields"}

// parseFormList extracts form summaries from a list response. It accepts a
// bare array, a JSON:API envelope ({"data":[…]}), or a single form object.
func parseFormList(data json.RawMessage) ([]formSummary, error) {
	items, err := formItems(data)
	if err != nil {
		return nil, err
	}
	forms := make([]formSummary, 0, len(items))
	for _, item := range items {
		if summary, ok := formSummaryFromRaw(item); ok {
			forms = append(forms, summary)
		}
	}
	return forms, nil
}

// formItems unwraps the collection shapes Lawmatics list endpoints return.
func formItems(data json.RawMessage) ([]json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		return arr, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, fmt.Errorf("parsing forms response: %w", err)
	}
	for _, key := range []string{"data", "results", "items", "forms"} {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var nested []json.RawMessage
		if err := json.Unmarshal(raw, &nested); err == nil {
			return nested, nil
		}
		// A single-object "data" is a detail response, not a collection.
		return []json.RawMessage{raw}, nil
	}
	return []json.RawMessage{data}, nil
}

func formSummaryFromRaw(raw json.RawMessage) (formSummary, bool) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return formSummary{}, false
	}
	summary := formSummary{ID: stringField(obj, "id")}
	attrs, _ := obj["attributes"].(map[string]any)
	pick := func(key string) string {
		if v := stringField(obj, key); v != "" {
			return v
		}
		if attrs != nil {
			return stringField(attrs, key)
		}
		return ""
	}
	summary.Name = pick("name")
	if summary.Name == "" {
		summary.Name = pick("title")
	}
	summary.CreatedAt = pick("created_at")
	summary.UpdatedAt = pick("updated_at")
	if summary.ID == "" && summary.Name == "" {
		return formSummary{}, false
	}
	return summary, true
}

// formDetailSummary reads the id/name off a `forms get` response so other
// commands can report which form they acted on.
func formDetailSummary(data json.RawMessage) formSummary {
	items, err := formItems(data)
	if err != nil || len(items) == 0 {
		return formSummary{}
	}
	summary, _ := formSummaryFromRaw(items[0])
	return summary
}

// flattenFormFields walks a `forms get …?fields=all` response and returns
// every fillable component in form order. Components are deduplicated by id:
// Lawmatics repeats the same component across conditional rows, and a
// submission body can only carry one value per id.
//
// When includeAll is true, layout components (page_divider, instructions, …)
// are returned as well, which is what a caller rebuilding a form needs.
func flattenFormFields(data json.RawMessage, includeAll bool) ([]formField, error) {
	var root any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing form schema: %w", err)
	}
	var components []map[string]any
	collectComponents(root, &components)

	fields := make([]formField, 0, len(components))
	seen := make(map[string]bool, len(components))
	for _, component := range components {
		componentType := stringField(component, "component_type")
		if !includeAll && !fillableComponentTypes[componentType] {
			continue
		}
		id := stringField(component, "id")
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		field := formField{
			ID:            id,
			SimplifiedID:  stringField(component, "simplified_id"),
			Label:         cleanFormLabel(stringField(component, "label")),
			FieldType:     stringField(component, "field_type"),
			Required:      boolField(component, "required"),
			ComponentType: componentType,
		}
		if v, ok := component["default_value"]; ok && v != nil {
			field.DefaultValue = v
		}
		if opts, ok := component["list_options"].([]any); ok && len(opts) > 0 {
			field.Options = opts
		}
		fields = append(fields, field)
	}
	return fields, nil
}

// collectComponents walks an arbitrary decoded JSON tree and appends every
// object that carries a component_type. Traversal order is deterministic:
// arrays keep their index order, and object keys are visited in
// componentChildKeys order first, then alphabetically.
func collectComponents(node any, out *[]map[string]any) {
	switch value := node.(type) {
	case []any:
		for _, item := range value {
			collectComponents(item, out)
		}
	case map[string]any:
		if _, ok := value["component_type"].(string); ok {
			*out = append(*out, value)
		}
		visited := make(map[string]bool, len(componentChildKeys))
		for _, key := range componentChildKeys {
			visited[key] = true
			if child, ok := value[key]; ok {
				collectComponents(child, out)
			}
		}
		rest := make([]string, 0, len(value))
		for key := range value {
			if !visited[key] {
				rest = append(rest, key)
			}
		}
		sort.Strings(rest)
		for _, key := range rest {
			collectComponents(value[key], out)
		}
	}
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// cleanFormLabel renders a component label as plain text. Lawmatics stores
// rich-text labels as HTML, which is unreadable in a table and useless as a
// lookup key.
func cleanFormLabel(label string) string {
	if label == "" {
		return ""
	}
	if strings.ContainsAny(label, "<&") {
		label = htmlTagPattern.ReplaceAllString(label, " ")
		label = html.UnescapeString(label)
		label = strings.ReplaceAll(label, "\u00a0", " ")
	}
	return strings.Join(strings.Fields(label), " ")
}

// filterFormFields applies the `forms fields` display filters: a
// case-insensitive substring match against label/id/simplified id, and a
// required-only toggle.
func filterFormFields(fields []formField, query string, requiredOnly bool) []formField {
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]formField, 0, len(fields))
	for _, field := range fields {
		if requiredOnly && !field.Required {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(field.Label + " " + field.ID + " " + field.SimplifiedID + " " + field.FieldType)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, field)
	}
	return out
}

// buildFieldTemplate returns a submit-ready body skeleton keyed by field id.
// Values are the form's own defaults where it declares them, so a caller can
// fill in only what it wants to change and pipe the result straight into
// `forms submit --json-file`.
func buildFieldTemplate(fields []formField) map[string]any {
	template := make(map[string]any, len(fields))
	for _, field := range fields {
		if field.DefaultValue != nil {
			template[field.ID] = field.DefaultValue
			continue
		}
		template[field.ID] = templateZeroValue(field.FieldType)
	}
	return template
}

func templateZeroValue(fieldType string) any {
	switch fieldType {
	case "boolean", "checkbox":
		return false
	default:
		return ""
	}
}

func stringField(obj map[string]any, key string) string {
	if v, ok := obj[key].(string); ok {
		return v
	}
	return ""
}

func boolField(obj map[string]any, key string) bool {
	switch v := obj[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}
