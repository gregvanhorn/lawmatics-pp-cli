// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// uuidPattern matches the form identifiers Lawmatics uses. A reference that
// matches is used verbatim, so an agent holding a uuid never pays for a list
// call to resolve it.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func looksLikeFormUUID(ref string) bool {
	return uuidPattern.MatchString(strings.TrimSpace(ref))
}

// formNotFoundError reports that no form matched the reference.
type formNotFoundError struct {
	Ref string
}

func (e *formNotFoundError) Error() string {
	return fmt.Sprintf("no form matches %q; run 'lawmatics-pp-cli forms list' to see available forms", e.Ref)
}

// formAmbiguousError reports that a reference matched more than one form.
// Resolution fails closed: acting on the wrong form would submit client data
// into the wrong intake pipeline.
type formAmbiguousError struct {
	Ref        string
	Candidates []formSummary
}

func (e *formAmbiguousError) Error() string {
	names := make([]string, 0, len(e.Candidates))
	for i, candidate := range e.Candidates {
		if i == 10 {
			names = append(names, fmt.Sprintf("… and %d more", len(e.Candidates)-i))
			break
		}
		names = append(names, fmt.Sprintf("%s (%s)", candidate.Name, candidate.ID))
	}
	return fmt.Sprintf("%q matches %d forms: %s; pass the uuid or the exact name",
		e.Ref, len(e.Candidates), strings.Join(names, ", "))
}

// resolveFormRef maps a user-supplied reference to exactly one form.
// Matching runs in descending confidence and stops at the first tier that
// produces any match, so a weaker tier can never override a stronger one:
//
//  1. exact name (case-insensitive)
//  2. form number — "450", "Form 450", "#450" all match "0450 - EP Consult…"
//  3. substring of the name
//  4. every query word appears in the name, in any order
//
// A tier that produces more than one match is ambiguous and fails.
func resolveFormRef(ref string, forms []formSummary) (formSummary, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return formSummary{}, &formNotFoundError{Ref: ref}
	}
	for _, form := range forms {
		if form.ID == trimmed {
			return form, nil
		}
	}

	normalizedRef := normalizeFormText(trimmed)
	tiers := [][]formSummary{
		matchForms(forms, func(form formSummary) bool {
			return normalizeFormText(form.Name) == normalizedRef
		}),
	}
	if code := formNumberQuery(normalizedRef); code != "" {
		tiers = append(tiers, matchForms(forms, func(form formSummary) bool {
			return formHasNumber(form.Name, code)
		}))
	}
	tiers = append(tiers,
		matchForms(forms, func(form formSummary) bool {
			return normalizedRef != "" && strings.Contains(normalizeFormText(form.Name), normalizedRef)
		}),
		matchForms(forms, func(form formSummary) bool {
			return containsAllWords(normalizeFormText(form.Name), strings.Fields(normalizedRef))
		}),
	)

	for _, matches := range tiers {
		switch len(matches) {
		case 0:
			continue
		case 1:
			return matches[0], nil
		default:
			return formSummary{}, &formAmbiguousError{Ref: ref, Candidates: matches}
		}
	}
	return formSummary{}, &formNotFoundError{Ref: ref}
}

func matchForms(forms []formSummary, pred func(formSummary) bool) []formSummary {
	var matches []formSummary
	for _, form := range forms {
		if pred(form) {
			matches = append(matches, form)
		}
	}
	return matches
}

// normalizeFormText lowercases and reduces every run of non-alphanumeric
// characters to a single space, so "0450 - EP Consult + Design Decisions"
// and "0450 ep consult design decisions" compare equal.
func normalizeFormText(s string) string {
	var b strings.Builder
	lastWasSpace := true
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasSpace = false
			continue
		}
		if !lastWasSpace {
			b.WriteRune(' ')
			lastWasSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// formNumberFillers are words callers put around a form number that carry no
// matching signal of their own.
var formNumberFillers = map[string]bool{"form": true, "no": true, "num": true, "number": true, "nwo": true}

// formNumberQuery returns the bare form number a normalized reference asks
// for ("form 450" → "450"), or "" when the reference is not a form number.
func formNumberQuery(normalized string) string {
	var digits string
	for _, token := range strings.Fields(normalized) {
		if formNumberFillers[token] {
			continue
		}
		if !isDigits(token) || digits != "" {
			return ""
		}
		digits = token
	}
	return strings.TrimLeft(digits, "0")
}

// formHasNumber reports whether any number in the form's name equals code,
// ignoring leading zeros so "450" matches the "0450 - …" naming convention.
func formHasNumber(name, code string) bool {
	for _, token := range strings.Fields(normalizeFormText(name)) {
		if isDigits(token) && strings.TrimLeft(token, "0") == code {
			return true
		}
	}
	return false
}

func containsAllWords(haystack string, words []string) bool {
	if len(words) == 0 {
		return false
	}
	fields := strings.Fields(haystack)
	for _, word := range words {
		found := false
		for _, field := range fields {
			if field == word {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
