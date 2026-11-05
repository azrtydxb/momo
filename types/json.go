// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// JSON[T] stores any JSON-encodable T: jsonb on PostgreSQL, TEXT on SQLite.
// A nil map, slice or pointer encodes as the JSON literal null, an empty one
// as {} or []; Scan restores the same distinction.
type JSON[T any] struct{ V T }

// SQLType returns the column type for d.
func (JSON[T]) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "jsonb"
	}
	return "TEXT"
}

// Value encodes V as JSON text.
func (j JSON[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, fmt.Errorf("types: encode JSON: %w", err)
	}
	return string(b), nil
}

// Scan decodes JSON text (string or []byte) into V. NULL is an error; the
// JSON literal null decodes to the zero value of T.
func (j *JSON[T]) Scan(src any) error {
	s, err := text("JSON", src)
	if err != nil {
		return err
	}
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return fmt.Errorf("types: decode JSON: %w", err)
	}
	j.V = v
	return nil
}
