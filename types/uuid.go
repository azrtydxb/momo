// SPDX-License-Identifier: Apache-2.0

package types

import (
	"database/sql/driver"
	"encoding/hex"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// UUID is a 16-byte identifier: uuid on PostgreSQL, TEXT canonical form on
// SQLite.
type UUID [16]byte

// SQLType returns the column type for d.
func (UUID) SQLType(d dialect.Name) string {
	if d == dialect.Postgres {
		return "uuid"
	}
	return "TEXT"
}

// String returns the canonical 36-character lowercase form.
func (u UUID) String() string {
	var b [36]byte
	hex.Encode(b[0:8], u[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], u[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], u[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], u[8:10])
	b[23] = '-'
	hex.Encode(b[24:], u[10:])
	return string(b[:])
}

// Value returns the canonical string, which PostgreSQL casts to uuid.
func (u UUID) Value() (driver.Value, error) { return u.String(), nil }

// Scan accepts the canonical 36-character form or 32 bare hex digits (string
// or []byte), or exactly 16 raw bytes. Any other length or character is an
// error.
func (u *UUID) Scan(src any) error {
	if b, ok := src.([]byte); ok && len(b) == 16 {
		copy(u[:], b)
		return nil
	}
	s, err := text("UUID", src)
	if err != nil {
		return err
	}
	parsed, err := ParseUUID(s)
	if err != nil {
		return err
	}
	*u = parsed
	return nil
}

// ParseUUID parses the canonical 36-character form (dashes at 8, 13, 18, 23)
// or 32 bare hex digits.
func ParseUUID(s string) (UUID, error) {
	var u UUID
	var digits [32]byte
	switch len(s) {
	case 36:
		if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
			return u, fmt.Errorf("types: invalid UUID %q: misplaced dashes", s)
		}
		n := copy(digits[:], s[0:8])
		n += copy(digits[n:], s[9:13])
		n += copy(digits[n:], s[14:18])
		n += copy(digits[n:], s[19:23])
		copy(digits[n:], s[24:])
	case 32:
		copy(digits[:], s)
	default:
		return u, fmt.Errorf("types: invalid UUID %q: length %d", s, len(s))
	}
	if _, err := hex.Decode(u[:], digits[:]); err != nil {
		return UUID{}, fmt.Errorf("types: invalid UUID %q: %w", s, err)
	}
	return u, nil
}
