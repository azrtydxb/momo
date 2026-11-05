// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// Hstore is map[string]*string: hstore on PostgreSQL, TEXT JSON on SQLite. A
// nil value pointer is an hstore NULL.
type Hstore map[string]*string

// SQLType returns the column type for d.
func (Hstore) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "hstore"
	}
	return "TEXT"
}

// Value returns the hstore text form, e.g. "k"=>"v", "n"=>NULL, with keys
// sorted, quotes and backslashes escaped and nil values as NULL. SQLite
// stores the same text in a TEXT column; a dialect-aware JSON encoding for
// SQLite is not implemented yet.
func (h Hstore) Value() (driver.Value, error) { return h.format(), nil }

// format encodes h as hstore text.
func (h Hstore) format() string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteString(", ")
		}
		writeHstoreString(&sb, k)
		sb.WriteString("=>")
		if v := h[k]; v == nil {
			sb.WriteString("NULL")
		} else {
			writeHstoreString(&sb, *v)
		}
	}
	return sb.String()
}

func writeHstoreString(sb *strings.Builder, s string) {
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			sb.WriteByte('\\')
		}
		sb.WriteByte(s[i])
	}
	sb.WriteByte('"')
}

// Scan accepts the PostgreSQL hstore text form or a JSON object (string or
// []byte). Duplicate keys in hstore text resolve first-wins, as PostgreSQL
// does. Scan(nil) returns an error: Hstore models a NOT NULL column, so use
// a *Hstore or a Null wrapper for nullable columns.
func (h *Hstore) Scan(src any) error {
	s, err := text("Hstore", src)
	if err != nil {
		return err
	}
	t := strings.TrimSpace(s)
	m := map[string]*string{}
	if strings.HasPrefix(t, "{") {
		if err := json.Unmarshal([]byte(t), &m); err != nil {
			return fmt.Errorf("types: decode Hstore JSON: %w", err)
		}
		if m == nil {
			m = map[string]*string{}
		}
	} else if m, err = parseHstore(t); err != nil {
		return err
	}
	*h = m
	return nil
}

func parseHstore(s string) (map[string]*string, error) {
	m := map[string]*string{}
	i := 0
	skip := func() {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
			i++
		}
	}
	quoted := func() (string, error) {
		if i >= len(s) || s[i] != '"' {
			return "", fmt.Errorf("types: invalid hstore %q: expected quote at %d", s, i)
		}
		i++
		var sb strings.Builder
		for i < len(s) {
			switch s[i] {
			case '\\':
				i++
				if i >= len(s) {
					return "", errors.New("types: invalid hstore: dangling escape")
				}
				sb.WriteByte(s[i])
			case '"':
				i++
				return sb.String(), nil
			default:
				sb.WriteByte(s[i])
			}
			i++
		}
		return "", fmt.Errorf("types: invalid hstore %q: unterminated string", s)
	}
	skip()
	for i < len(s) {
		k, err := quoted()
		if err != nil {
			return nil, err
		}
		skip()
		if !strings.HasPrefix(s[i:], "=>") {
			return nil, fmt.Errorf("types: invalid hstore %q: expected => at %d", s, i)
		}
		i += 2
		skip()
		if i+4 <= len(s) && strings.EqualFold(s[i:i+4], "NULL") {
			i += 4
			if _, dup := m[k]; !dup {
				m[k] = nil
			}
		} else {
			v, err := quoted()
			if err != nil {
				return nil, err
			}
			if _, dup := m[k]; !dup {
				m[k] = &v
			}
		}
		skip()
		if i < len(s) {
			if s[i] != ',' {
				return nil, fmt.Errorf("types: invalid hstore %q: expected comma at %d", s, i)
			}
			i++
			skip()
			if i >= len(s) {
				return nil, fmt.Errorf("types: invalid hstore %q: trailing comma", s)
			}
		}
	}
	return m, nil
}
