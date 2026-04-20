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
