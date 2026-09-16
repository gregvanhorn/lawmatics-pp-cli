package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindContactsByName(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.FindContactsByName("Gottfried"); err == nil {
		t.Fatal("unsynced store must give sync guidance")
	}
	if err := db.SaveSyncState("contacts", "", 0); err != nil {
		t.Fatal(err)
	}
	if got, err := db.FindContactsByName("Gottfried"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("synced empty store: %v, %v", got, err)
	}
	seed := func(resource string, records ...json.RawMessage) {
		t.Helper()
		if _, _, err := db.UpsertBatch(resource, records); err != nil {
			t.Fatal(err)
		}
	}
	seed("contacts",
		json.RawMessage(`{"id":"901","type":"contact","attributes":{"first_name":"Ada","last_name":"Gottfried","email":"ada@example.test","phone":"+1 202-555-0101","phone_number":"+1 202-555-0101"}}`),
		json.RawMessage(`{"id":"902","first_name":"Ben","last_name":"Gottfried","phone":null}`),
		json.RawMessage(`{"id":"903","attributes":{"first_name":"Unrelated","last_name":"Person","bio":"Gottfried","email":"gottfried@example.test"}}`),
		json.RawMessage(`{"id":"904","attributes":{"first_name":"Zoë","last_name":"O'Neil/%_"}}`))
	// Force the desired contacts beyond List's default cap by updated_at.
	if _, err := db.db.Exec(`UPDATE resources SET updated_at = '2000-01-01'`); err != nil {
		t.Fatal(err)
	}
	var fillers []json.RawMessage
	for i := 0; i < 205; i++ {
		fillers = append(fillers, json.RawMessage(fmt.Sprintf(`{"id":"%d","attributes":{"first_name":"Other","last_name":"Person"}}`, i)))
	}
	seed("contacts", fillers...)
	seed("companies", json.RawMessage(`{"id":"901","name":"Gottfried"}`))
	for i, row := range []struct{ owner, kind, phone string }{
		{"901", "contact", "+1 202-555-0101"},
		{"901", "contact", "+1 202-555-0102"},
		{"901", "company", "+1 202-555-0199"},
		{"903", "contact", "+1 202-555-0198"},
	} {
		seed("phone_numbers", json.RawMessage(fmt.Sprintf(`{"id":"%d","attributes":{"info":%q},"relationships":{"informationable":{"data":{"id":%q,"type":%q}}}}`, i, row.phone, row.owner, row.kind)))
	}
	if err := db.SaveSyncState("phone_numbers", "", 4); err != nil {
		t.Fatal(err)
	}
	got, err := db.FindContactsByName("  goTTfried  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "901" || got[1].ID != "902" {
		t.Fatalf("wrong candidates: %+v", got)
	}
	if !reflect.DeepEqual(got[0].PhoneNumbers, []string{"+1 202-555-0101", "+1 202-555-0102"}) {
		t.Fatalf("wrong phone association: %+v", got[0])
	}
	if got[0].PhoneNumbersSyncedAt == nil || got[0].Email != "ada@example.test" || got[1].PhoneNumbers == nil || len(got[1].PhoneNumbers) != 0 {
		t.Fatalf("wrong projection: %+v", got)
	}
	for _, tc := range []struct{ query, id string }{
		{"Ada   Gottfried", "901"}, {"ben", "902"}, {"zoË o'neil/%_", "904"}, {"absent", ""}, {"%", "904"},
	} {
		got, err := db.FindContactsByName(tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if tc.id == "" {
			if got == nil || len(got) != 0 {
				t.Fatalf("%q: %+v", tc.query, got)
			}
			continue
		}
		if len(got) != 1 || got[0].ID != tc.id {
			t.Fatalf("%q: %+v", tc.query, got)
		}
	}
	if _, err := db.FindContactsByName(" \t "); err == nil {
		t.Fatal("blank name accepted")
	}
}
