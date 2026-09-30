package page

import (
	"fmt"
	"strings"
)

// SortDir represents sort direction: Asc (1) or Desc (-1), matching MongoDB convention.
type SortDir int

const (
	Asc  SortDir = 1
	Desc SortDir = -1
)

// SortField is a single sort criterion: a field name and its direction.
type SortField struct {
	Field string
	Dir   SortDir
}

// SortRequest is an ordered list of sort fields.
// Order matters: the first field has highest sort priority.
type SortRequest []SortField

// ParseSort parses a comma-separated sort string into a SortRequest.
// Format: "field[:dir]" where dir is "asc"/"1" or "desc"/"-1" (default: asc).
// Examples:
//
//	"name"                  → [{name, Asc}]
//	"name:asc,createdAt:desc" → [{name, Asc}, {createdAt, Desc}]
func ParseSort(raw string) (SortRequest, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	result := make(SortRequest, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ":", 2)
		field := strings.TrimSpace(kv[0])
		if field == "" {
			return nil, fmt.Errorf("sort: empty field name in %q", part)
		}
		if !ValidSortField(field) {
			return nil, fmt.Errorf("sort: invalid field name %q", field)
		}
		dir := Asc
		if len(kv) == 2 {
			switch strings.ToLower(strings.TrimSpace(kv[1])) {
			case "asc", "1":
				dir = Asc
			case "desc", "-1":
				dir = Desc
			default:
				return nil, fmt.Errorf("sort: invalid direction %q for field %q", kv[1], field)
			}
		}
		result = append(result, SortField{Field: field, Dir: dir})
	}
	return result, nil
}

// ValidSortField dice se name è un nome di campo ordinabile: uno o più identificatori (lettere,
// cifre, `_`, non iniziano con una cifra) separati da `.` — `name`, `created_at`, `address.city`.
//
// Il sort arriva da un query param (`?sort=`), e un nome di campo che finisce in un ORDER BY SQL o
// in una chiave bson è un input dell'utente dentro una query: senza questo controllo
// `?sort=id;DROP TABLE x` veniva interpolato così com'era, e in Mongo una chiave `$...` è un
// operatore. Lo si verifica qui, all'ingresso, e di nuovo nei builder (SortRequest.Validate), perché
// una SortRequest si può anche costruire a mano.
func ValidSortField(name string) bool {
	if name == "" {
		return false
	}
	for _, seg := range strings.Split(name, ".") {
		if seg == "" {
			return false
		}
		for i, r := range seg {
			switch {
			case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9' && i > 0:
			default:
				return false
			}
		}
	}
	return true
}

// Validate verifica che ogni campo sia un nome ordinabile (vedi ValidSortField). I builder di
// go-core-sql e go-core-mongo lo chiamano prima di usare la richiesta.
func (s SortRequest) Validate() error {
	for _, f := range s {
		if !ValidSortField(f.Field) {
			return fmt.Errorf("sort: invalid field name %q", f.Field)
		}
	}
	return nil
}
