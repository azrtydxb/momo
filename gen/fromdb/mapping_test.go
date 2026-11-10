// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestGenerateFromDBMapping(t *testing.T) {
	cases := []struct {
		d    dialect.Name
		in   string
		want string
		size int
		dim  int
	}{
		{dialect.Postgres, "bigint", "int64", 0, 0},
		{dialect.Postgres, "integer", "int32", 0, 0},
		{dialect.Postgres, "timestamp with time zone", "time.Time", 0, 0},
		{dialect.Postgres, "timestamp(3) without time zone", "time.Time", 0, 0},
		{dialect.Postgres, "character varying(200)", "string", 200, 0},
		{dialect.Postgres, "character varying", "string", 0, 0},
		{dialect.Postgres, "numeric(10,2)", "string", 0, 0},
		{dialect.Postgres, "jsonb", "types.JSON[json.RawMessage]", 0, 0},
		{dialect.Postgres, "uuid", "types.UUID", 0, 0},
		{dialect.Postgres, "time without time zone", "types.TimeOfDay", 0, 0},
		{dialect.Postgres, "vector(1536)", "vector.Vector", 0, 1536},
		{dialect.Postgres, "halfvec(8)", "vector.HalfVector", 0, 8},
		{dialect.Postgres, "tsvector", "types.TSVector", 0, 0},
		{dialect.Postgres, "text[]", "[]string", 0, 0},
		{dialect.Postgres, "bigint[]", "[]int64", 0, 0},
		{dialect.Postgres, "bytea", "[]byte", 0, 0},
		{dialect.SQLite, "INTEGER", "int64", 0, 0},
		{dialect.SQLite, "TEXT", "string", 0, 0},
	}
	for _, c := range cases {
		got, err := MapType(c.d, c.in)
		if err != nil {
			t.Errorf("MapType(%s, %q): %v", c.d, c.in, err)
			continue
		}
		if got.Expr != c.want || got.Size != c.size || got.Dim != c.dim {
			t.Errorf("MapType(%s, %q) = %+v; want %s size %d dim %d", c.d, c.in, got, c.want, c.size, c.dim)
		}
	}
	if g, _ := MapType(dialect.Postgres, "numeric(10,2)"); g.Note == "" {
		t.Error("numeric must carry the decimal-as-string note")
	}
}

func TestGenerateFromDBUnmapped(t *testing.T) {
	for _, in := range []string{"geometry(point,4326)", "inet", "interval", "money", "bit varying(8)", "uuid[]", "time with time zone"} {
		_, err := MapType(dialect.Postgres, in)
		if !errors.Is(err, ErrUnmapped) {
			t.Errorf("%q: want ErrUnmapped, got %v", in, err)
		}
	}
	if _, err := MapType(dialect.SQLite, "GEOMETRY"); !errors.Is(err, ErrUnmapped) {
		t.Errorf("sqlite GEOMETRY: want ErrUnmapped, got %v", err)
	}
}
