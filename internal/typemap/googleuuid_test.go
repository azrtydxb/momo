// SPDX-License-Identifier: Apache-2.0

package typemap

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
)

// Catches: a github.com/google/uuid type that needs a type tag, a pointer or
// slice of it that loses the mapping, and a uuid[] offered on SQLite.
func TestMapGoogleUUID(t *testing.T) {
	for _, c := range []struct {
		goType, pg, sqlite string
	}{
		{"uuid.UUID", "uuid", "TEXT"},
		{"*uuid.UUID", "uuid", "TEXT"},
		{"[]uuid.UUID", "uuid[]", ""},
	} {
		got, err := Map(c.goType, dialect.Postgres, tags.Tags{})
		if err != nil || got != c.pg {
			t.Errorf("Map(%s, postgres) = %q, %v; want %q", c.goType, got, err, c.pg)
		}
		got, err = Map(c.goType, dialect.SQLite, tags.Tags{})
		if c.sqlite == "" {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("Map(%s, sqlite) = %q, %v; want ErrUnsupported", c.goType, got, err)
			}
			continue
		}
		if err != nil || got != c.sqlite {
			t.Errorf("Map(%s, sqlite) = %q, %v; want %q", c.goType, got, err, c.sqlite)
		}
	}
}
