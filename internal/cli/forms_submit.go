// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// formFileUpload is a --file KEY=PATH assignment: a form component whose
// value is the contents of a local file rather than a literal.
type formFileUpload struct {
	Key  string
	Path string
}

func newFormsSubmitCmd(flags *rootFlags) *cobra.Command {
	var jsonFile string
	var stdinBody bool
	var fieldAssignments []string
	var stringAssignments []string
	var fileAssignments []string
	var wrapKey string
	var multipartBody bool
	var noValidate bool
	var noAuth bool

	cmd := &cobra.Command{
		Use:   "submit <form>",
		Short: "Submit an entry to a custom form",
		Long: `Submit an entry to a custom form.

Values are keyed by the field ids from 'forms fields <form>'. Supply them as
a JSON body (--json-file or --stdin), as repeated --field key=value pairs, or
both — --field wins on conflict.

--field values are read as JSON when they parse as one (true, 12, ["a"],
{"k":1}) and as plain text otherwise. Use --string to force text, which is
what a zip code, phone number, or any other digit string usually wants.

Before sending, the submission is checked against the form definition:
unknown field ids and missing required fields are rejected. Pass
--no-validate to skip the check.

Lawmatics documents this endpoint as unauthenticated. A configured token is
still sent by default because the same credential governs rate limits and
audit attribution; pass --no-auth to submit anonymously.

Submitting fires the form's automations.`,
		Example: `  lawmatics-pp-cli forms submit "0450" --json-file filled.json --agent
  lawmatics-pp-cli forms submit "0450" \
    --field RmllbGRzOjpTdGFuZGFyZEZpZWxkLUNvbnRhY3QtZmlyc3RfbmFtZQ==="Ada" \
    --string RmllbGRzOjpTdGFuZGFyZEZpZWxkLUNvbnRhY3QtemlwY29kZQ===02134
  cat filled.json | lawmatics-pp-cli forms submit "0450" --stdin --dry-run`,
		Annotations: map[string]string{"pp:endpoint": "forms.submit", "pp:method": "POST", "pp:path": "/forms/{id}/submit"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			if err := formsLiveOnly(flags); err != nil {
				return err
			}
			if jsonFile != "" && stdinBody {
				return usageErr(fmt.Errorf("--json-file and --stdin are mutually exclusive"))
			}

			base, err := readSubmitBase(jsonFile, stdinBody)
			if err != nil {
				return err
			}
			overrides, err := parseFieldAssignments(fieldAssignments, "--field", true)
			if err != nil {
				return usageErr(err)
			}
			literals, err := parseFieldAssignments(stringAssignments, "--string", false)
			if err != nil {
				return usageErr(err)
			}
			for key, value := range literals {
				overrides[key] = value
			}
			uploads, err := parseFileAssignments(fileAssignments)
			if err != nil {
				return usageErr(err)
			}
			if len(base) == 0 && len(overrides) == 0 && len(uploads) == 0 {
				return usageErr(fmt.Errorf("no values to submit: pass --json-file, --stdin, --field, --string, or --file " +
					"(an empty submission is rejected by the API with HTTP 422)"))
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			if noAuth && c.Config != nil {
				// Clearing the in-memory credential is how a single
				// invocation opts out of auth; the config file is untouched.
				c.Config.AuthHeaderVal = ""
				c.Config.AccessToken = ""
			}
			form, err := resolveFormTarget(c, flags, args[0])
			if err != nil {
				return err
			}

			values := buildSubmitValues(base, overrides, wrapKey)
			if !noValidate && !flags.dryRun {
				if err := validateAgainstForm(cmd, c, form, values, uploads); err != nil {
					return err
				}
			}
			payload := wrapSubmitValues(values, wrapKey)

			path := formPath(form.ID, "submit")
			var data json.RawMessage
			var statusCode int
			if multipartBody || len(uploads) > 0 {
				body, contentType, buildErr := buildMultipartBody(values, uploads)
				if buildErr != nil {
					return buildErr
				}
				data, statusCode, err = c.PostRaw(path, nil, body, contentType, nil)
			} else {
				data, statusCode, err = c.PostWithParams(path, map[string]string{}, payload)
			}
			if err != nil {
				return classifyAPIError(err, flags)
			}

			if flags.asJSON || (!isTerminal(cmd.OutOrStdout()) && !flags.csv && !flags.quiet && !flags.plain) {
				if flags.quiet {
					return nil
				}
				envelope := map[string]any{
					"action":      "post",
					"resource":    "forms",
					"path":        path,
					"status":      statusCode,
					"success":     statusCode >= 200 && statusCode < 300,
					"form_id":     form.ID,
					"field_count": len(values),
				}
				if form.Name != "" {
					envelope["form_name"] = form.Name
				}
				if flags.dryRun {
					envelope["dry_run"] = true
					envelope["status"] = 0
					envelope["success"] = false
				}
				if len(data) > 0 {
					var parsed any
					if json.Unmarshal(data, &parsed) == nil {
						envelope["data"] = parsed
					}
				}
				envelopeJSON, marshalErr := json.Marshal(envelope)
				if marshalErr != nil {
					return marshalErr
				}
				return printOutput(cmd.OutOrStdout(), json.RawMessage(envelopeJSON), true)
			}
			return printOutputWithFlags(cmd.OutOrStdout(), data, flags)
		},
	}
	cmd.Flags().StringVar(&jsonFile, "json-file", "", "Read the submission body from a JSON file")
	cmd.Flags().BoolVar(&stdinBody, "stdin", false, "Read the submission body as JSON from stdin")
	cmd.Flags().StringArrayVar(&fieldAssignments, "field", nil, "Field value as id=value, JSON-decoded when it parses as JSON (repeatable)")
	cmd.Flags().StringArrayVar(&stringAssignments, "string", nil, "Field value as id=value, always sent as text (repeatable)")
	cmd.Flags().StringArrayVar(&fileAssignments, "file", nil, "File upload as id=path; implies --multipart (repeatable)")
	cmd.Flags().StringVar(&wrapKey, "wrap", "", `Nest the values under a top-level key (e.g. --wrap fields)`)
	cmd.Flags().BoolVar(&multipartBody, "multipart", false, "Send multipart/form-data instead of JSON")
	cmd.Flags().BoolVar(&noValidate, "no-validate", false, "Skip checking the submission against the form definition")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "Submit without the configured OAuth token")
	return cmd
}

// readSubmitBase loads the JSON body supplied by --json-file or --stdin.
// Numbers are kept as json.Number so a value like "02134" that the caller
// already wrote as a string is never reformatted.
func readSubmitBase(jsonFile string, stdinBody bool) (map[string]any, error) {
	var raw []byte
	var err error
	switch {
	case jsonFile != "":
		raw, err = os.ReadFile(jsonFile)
		if err != nil {
			return nil, usageErr(fmt.Errorf("reading %s: %w", jsonFile, err))
		}
	case stdinBody:
		raw, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
	default:
		return map[string]any{}, nil
	}
	return decodeSubmitJSON(raw)
}

func decodeSubmitJSON(raw []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return map[string]any{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var body map[string]any
	if err := decoder.Decode(&body); err != nil {
		return nil, usageErr(fmt.Errorf("parsing submission JSON: %w", err))
	}
	if body == nil {
		return map[string]any{}, nil
	}
	return body, nil
}

func errInvalidAssignment(flagName, entry string) error {
	return fmt.Errorf("invalid %s %q: expected key=value", flagName, entry)
}

// parseFieldAssignments parses repeated key=value flags into submission
// values. When coerce is set, a value that is valid JSON is decoded as JSON
// so booleans, numbers, arrays, and objects survive the shell.
func parseFieldAssignments(entries []string, flagName string, coerce bool) (map[string]any, error) {
	out := map[string]any{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			return nil, errInvalidAssignment(flagName, entry)
		}
		if coerce {
			out[key] = coerceFieldValue(value)
			continue
		}
		out[key] = value
	}
	return out, nil
}

// coerceFieldValue decodes a flag value as JSON when it is valid JSON, and
// returns it as text otherwise. Leading-zero digit strings ("02134") are not
// valid JSON numbers, so they survive as text without needing --string.
func coerceFieldValue(raw string) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return raw
	}
	// Reject trailing content ("12 monkeys" decodes 12 then stops).
	if _, err := decoder.Token(); err != io.EOF {
		return raw
	}
	return decoded
}

func parseFileAssignments(entries []string) ([]formFileUpload, error) {
	uploads := make([]formFileUpload, 0, len(entries))
	for _, entry := range entries {
		key, path, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("invalid --file %q: expected key=path", entry)
		}
		uploads = append(uploads, formFileUpload{Key: key, Path: path})
	}
	return uploads, nil
}

// buildSubmitValues merges the base body with flag overrides. When wrapKey is
// set and the base body is already wrapped under that key, the wrapper is
// unwound first so --field assignments land next to the existing values
// rather than beside the wrapper.
func buildSubmitValues(base, overrides map[string]any, wrapKey string) map[string]any {
	if wrapKey != "" && len(base) == 1 {
		if nested, ok := base[wrapKey].(map[string]any); ok {
			base = nested
		}
	}
	values := make(map[string]any, len(base)+len(overrides))
	for key, value := range base {
		values[key] = value
	}
	for key, value := range overrides {
		values[key] = value
	}
	return values
}

func wrapSubmitValues(values map[string]any, wrapKey string) map[string]any {
	if wrapKey == "" {
		return values
	}
	return map[string]any{wrapKey: values}
}

// validateAgainstForm rejects a submission that the form definition cannot
// accept. A schema that cannot be fetched degrades to a warning: the API is
// the authority on validity, and blocking a submission because the (optional)
// pre-check failed would be worse than letting the API answer.
func validateAgainstForm(cmd *cobra.Command, c formsFetcher, form formSummary, values map[string]any, uploads []formFileUpload) error {
	data, err := c.GetWithHeaders(formPath(form.ID), map[string]string{"fields": "all"}, nil)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not load the form definition to validate the submission (%v); sending anyway\n", err)
		return nil
	}
	fields, err := flattenFormFields(data, false)
	if err != nil || len(fields) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: the form definition contained no fillable fields; skipping validation")
		return nil
	}
	submitted := make(map[string]any, len(values)+len(uploads))
	for key, value := range values {
		submitted[key] = value
	}
	for _, upload := range uploads {
		submitted[upload.Key] = upload.Path
	}
	unknown, missing := validateSubmission(submitted, fields)
	if len(unknown) > 0 {
		return usageErr(fmt.Errorf("unknown field id(s) for form %s: %s\nrun 'lawmatics-pp-cli forms fields %q' for valid ids, or pass --no-validate",
			form.ID, strings.Join(unknown, ", "), form.ID))
	}
	if len(missing) > 0 {
		labels := make([]string, 0, len(missing))
		for _, field := range missing {
			labels = append(labels, fmt.Sprintf("%s (%s)", field.Label, field.ID))
		}
		return usageErr(fmt.Errorf("missing required field(s): %s\npass a value for each, or --no-validate to submit anyway",
			strings.Join(labels, ", ")))
	}
	return nil
}

// validateSubmission compares a submission against a form's fillable fields.
// Keys match on either the Lawmatics component id or the simplified id, since
// both appear in the schema and either is a reasonable thing to send.
func validateSubmission(values map[string]any, fields []formField) (unknown []string, missing []formField) {
	known := make(map[string]bool, len(fields)*2)
	for _, field := range fields {
		known[field.ID] = true
		if field.SimplifiedID != "" {
			known[field.SimplifiedID] = true
		}
	}
	for key := range values {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)

	for _, field := range fields {
		if !field.Required {
			continue
		}
		if isBlankSubmissionValue(values[field.ID]) && isBlankSubmissionValue(values[field.SimplifiedID]) {
			missing = append(missing, field)
		}
	}
	return unknown, missing
}

func isBlankSubmissionValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	default:
		return false
	}
}

// buildMultipartBody encodes the submission as multipart/form-data, reading
// each --file assignment from disk. Non-text values are encoded as JSON so a
// list or nested block survives the round trip.
func buildMultipartBody(values map[string]any, uploads []formFileUpload) ([]byte, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := writer.WriteField(key, multipartFieldValue(values[key])); err != nil {
			return nil, "", err
		}
	}

	for _, upload := range uploads {
		contents, err := os.ReadFile(upload.Path)
		if err != nil {
			return nil, "", usageErr(fmt.Errorf("reading %s: %w", upload.Path, err))
		}
		part, err := writer.CreateFormFile(upload.Key, filepath.Base(upload.Path))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(contents); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

func multipartFieldValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(encoded)
	}
}
