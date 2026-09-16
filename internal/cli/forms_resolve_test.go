// Copyright 2026 gregvanhorn. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

var resolveTestForms = []formSummary{
	{ID: "38d8ec11-87cd-41d6-b983-f025cfec8926", Name: "0450 - EP Consult + Design Decisions"},
	{ID: "aaaaaaaa-0000-0000-0000-000000000001", Name: "0451 - EP Consult Follow Up"},
	{ID: "aaaaaaaa-0000-0000-0000-000000000002", Name: "1450 - Probate Intake"},
	{ID: "aaaaaaaa-0000-0000-0000-000000000003", Name: "Client Feedback"},
}

func TestResolveFormRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ref  string
		want string
	}{
		{"uuid", "38d8ec11-87cd-41d6-b983-f025cfec8926", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"exact name", "0450 - EP Consult + Design Decisions", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"exact name, different punctuation and case", "0450 ep consult design decisions", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"form number with leading zero", "0450", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"form number without leading zero", "450", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"form number with the word form", "Form 450", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"form number that is its own prefix", "1450", "aaaaaaaa-0000-0000-0000-000000000002"},
		{"substring of the name", "Design Decisions", "38d8ec11-87cd-41d6-b983-f025cfec8926"},
		{"words out of order", "Feedback Client", "aaaaaaaa-0000-0000-0000-000000000003"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			form, err := resolveFormRef(tc.ref, resolveTestForms)
			if err != nil {
				t.Fatalf("resolveFormRef(%q): %v", tc.ref, err)
			}
			if form.ID != tc.want {
				t.Fatalf("resolveFormRef(%q) = %s (%s), want %s", tc.ref, form.ID, form.Name, tc.want)
			}
		})
	}
}

// An ambiguous reference must fail rather than guess: picking the wrong
// form would file a client's intake answers against the wrong pipeline.
func TestResolveFormRefAmbiguous(t *testing.T) {
	t.Parallel()
	_, err := resolveFormRef("EP Consult", resolveTestForms)
	var ambiguous *formAmbiguousError
	if !As(err, &ambiguous) {
		t.Fatalf("want an ambiguity error, got %v", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("want 2 candidates, got %+v", ambiguous.Candidates)
	}
	if !strings.Contains(err.Error(), "0451 - EP Consult Follow Up") {
		t.Errorf("the error should name the candidates: %v", err)
	}
}

func TestResolveFormRefNotFound(t *testing.T) {
	t.Parallel()
	for _, ref := range []string{"", "   ", "9999", "nonexistent form"} {
		_, err := resolveFormRef(ref, resolveTestForms)
		var notFound *formNotFoundError
		if !As(err, &notFound) {
			t.Errorf("resolveFormRef(%q): want not-found, got %v", ref, err)
		}
	}
}

// An exact name match must win even when it is also a substring of other
// forms, otherwise a precisely-specified form becomes unusable as the
// account grows.
func TestResolveFormRefExactWinsOverSubstring(t *testing.T) {
	t.Parallel()
	forms := []formSummary{
		{ID: "a", Name: "Intake"},
		{ID: "b", Name: "Intake (Spanish)"},
	}
	form, err := resolveFormRef("Intake", forms)
	if err != nil {
		t.Fatalf("resolveFormRef: %v", err)
	}
	if form.ID != "a" {
		t.Fatalf("got %s, want the exact match a", form.ID)
	}
}

func TestLooksLikeFormUUID(t *testing.T) {
	t.Parallel()
	if !looksLikeFormUUID("38d8ec11-87cd-41d6-b983-f025cfec8926") {
		t.Error("real uuid not recognized")
	}
	for _, ref := range []string{"0450", "Form 450", "38d8ec11-87cd-41d6-b983", ""} {
		if looksLikeFormUUID(ref) {
			t.Errorf("%q should not look like a uuid", ref)
		}
	}
}
