package store

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ContactMatch is a deliberately small local projection, not a live API record.
// Phone numbers retain their original formatting; callers must normalize them
// before comparing against another system. A nil phone sync time means unknown.
type ContactMatch struct {
	ID                   string     `json:"id"`
	FirstName            string     `json:"first_name"`
	LastName             string     `json:"last_name"`
	Name                 string     `json:"name"`
	Email                string     `json:"email"`
	PhoneNumbers         []string   `json:"phone_numbers"`
	PhoneNumbersSyncedAt *time.Time `json:"phone_numbers_synced_at"`
}

// FindContactsByName searches only contact name fields, without the generic
// List method's 200-record default limit. It never picks one duplicate name.
func (s *Store) FindContactsByName(name string) ([]ContactMatch, error) {
	normalize := func(value string) string {
		return strings.ToLower(strings.Join(strings.Fields(value), " "))
	}
	query := normalize(name)
	if query == "" {
		return nil, fmt.Errorf("contact name must not be blank")
	}
	rows, err := s.db.Query(`SELECT id, data FROM resources WHERE resource_type = 'contacts' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	matches := make([]ContactMatch, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil {
			rows.Close()
			return nil, err
		}
		// Lawmatics returns JSON:API attributes. Flat cached records are also
		// supported, since older imports may already have unwrapped them.
		if attrs, ok := record["attributes"]; ok {
			if err := json.Unmarshal(attrs, &record); err != nil {
				rows.Close()
				return nil, err
			}
		}
		field := func(key string) string {
			var value string
			_ = json.Unmarshal(record[key], &value)
			return value
		}
		first, last := field("first_name"), field("last_name")
		full := strings.TrimSpace(first + " " + last)
		if full == "" {
			full = field("name")
		}
		if !strings.Contains(normalize(full), query) {
			continue
		}
		match := ContactMatch{ID: id, FirstName: first, LastName: last, Name: full,
			Email: field("email"), PhoneNumbers: []string{}}
		for _, key := range []string{"phone", "phone_number"} {
			match.addPhone(field(key))
		}
		byID[id] = len(matches)
		matches = append(matches, match)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(byID) == 0 {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'contacts'`).Scan(&count); err != nil {
			return nil, err
		}
		if count == 0 {
			if _, syncedAt, _, err := s.GetSyncState("contacts"); err != nil || syncedAt.IsZero() {
				return nil, fmt.Errorf("no local contacts. Run 'lawmatics-pp-cli sync --resources contacts,phone_numbers --param fields=all' first")
			}
		}
		return matches, nil
	}
	// The official Phone Number response identifies its owner through this
	// polymorphic relationship. Match BOTH type and ID to avoid company/matter
	// records that happen to share a contact ID.
	phones, err := s.db.Query(`SELECT data FROM resources WHERE resource_type = 'phone_numbers' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer phones.Close()
	for phones.Next() {
		var raw []byte
		if err := phones.Scan(&raw); err != nil {
			return nil, err
		}
		var phone struct {
			Attributes struct {
				Info string `json:"info"`
			} `json:"attributes"`
			Relationships struct {
				Informationable struct {
					Data struct {
						ID   string `json:"id"`
						Type string `json:"type"`
					} `json:"data"`
				} `json:"informationable"`
			} `json:"relationships"`
		}
		if err := json.Unmarshal(raw, &phone); err != nil {
			return nil, err
		}
		owner := phone.Relationships.Informationable.Data
		if index, ok := byID[owner.ID]; ok && owner.Type == "contact" {
			matches[index].addPhone(phone.Attributes.Info)
		}
	}
	if err := phones.Err(); err != nil {
		return nil, err
	}
	phones.Close()
	if _, syncedAt, _, err := s.GetSyncState("phone_numbers"); err == nil && !syncedAt.IsZero() {
		for i := range matches {
			matches[i].PhoneNumbersSyncedAt = &syncedAt
		}
	}
	return matches, nil
}

func (m *ContactMatch) addPhone(phone string) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return
	}
	for _, existing := range m.PhoneNumbers {
		if existing == phone {
			return
		}
	}
	m.PhoneNumbers = append(m.PhoneNumbers, phone)
}
