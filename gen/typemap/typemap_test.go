// SPDX-License-Identifier: Apache-2.0

package typemap

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/schema"
)

func TestGeneratorTypeMap(t *testing.T) {
	cases := []struct {
		goType string
		d      dialect.Name
		tg     tags.Tags
		want   string
	}{
		{"int64", dialect.Postgres, tags.Tags{}, "bigint"},
		{"int64", dialect.SQLite, tags.Tags{}, "INTEGER"},
		{"string", dialect.Postgres, tags.Tags{Size: 40}, "character varying(40)"},
		{"string", dialect.SQLite, tags.Tags{}, "TEXT"},
		{"time.Time", dialect.Postgres, tags.Tags{}, "timestamp with time zone"},
		{"[]byte", dialect.SQLite, tags.Tags{}, "BLOB"},
		{"vector.Vector", dialect.Postgres, tags.Tags{Dim: 3}, "vector(3)"},
	}
	for _, c := range cases {
		got, err := Map(c.goType, c.d, c.tg)
		if err != nil || got != c.want {
			t.Errorf("Map(%s,%s)=%q,%v want %q", c.goType, c.d, got, err, c.want)
		}
	}
	if _, err := Map("uint64", dialect.Postgres, tags.Tags{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("uint64 err=%v", err)
	}
}

func TestGeneratorTypeMapFullTable(t *testing.T) {
	d3 := tags.Tags{Dim: 3}
	cases := []struct {
		goType     string
		tg         tags.Tags
		pg, sqlite string
	}{
		{"int", tags.Tags{}, "bigint", "INTEGER"},
		{"int8", tags.Tags{}, "smallint", "INTEGER"},
		{"int16", tags.Tags{}, "smallint", "INTEGER"},
		{"int32", tags.Tags{}, "integer", "INTEGER"},
		{"int64", tags.Tags{}, "bigint", "INTEGER"},
		{"float32", tags.Tags{}, "real", "REAL"},
		{"float64", tags.Tags{}, "double precision", "REAL"},
		{"bool", tags.Tags{}, "boolean", "INTEGER"},
		{"string", tags.Tags{}, "text", "TEXT"},
		{"string", tags.Tags{Size: 8}, "character varying(8)", "TEXT"},
		{"[]byte", tags.Tags{}, "bytea", "BLOB"},
		{"time.Time", tags.Tags{}, "timestamp with time zone", "TEXT"},
		{"types.JSON[map[string]any]", tags.Tags{}, "jsonb", "TEXT"},
		{"types.UUID", tags.Tags{}, "uuid", "TEXT"},
		{"types.Date", tags.Tags{}, "date", "TEXT"},
		{"vector.Vector", d3, "vector(3)", ""},
		{"vector.HalfVector", d3, "halfvec(3)", ""},
		{"vector.SparseVector", d3, "sparsevec(3)", ""},
		{"vector.Bits", d3, "bit(3)", ""},
		// nullability does not change the SQL type
		{"*int64", tags.Tags{}, "bigint", "INTEGER"},
		{"*string", tags.Tags{Size: 8}, "character varying(8)", "TEXT"},
		{"*time.Time", tags.Tags{}, "timestamp with time zone", "TEXT"},
		{"sql.NullInt64", tags.Tags{}, "bigint", "INTEGER"},
		{"sql.NullInt32", tags.Tags{}, "integer", "INTEGER"},
		{"sql.NullInt16", tags.Tags{}, "smallint", "INTEGER"},
		{"sql.NullString", tags.Tags{}, "text", "TEXT"},
		{"sql.NullBool", tags.Tags{}, "boolean", "INTEGER"},
		{"sql.NullFloat64", tags.Tags{}, "double precision", "REAL"},
		{"sql.NullTime", tags.Tags{}, "timestamp with time zone", "TEXT"},
		{"sql.NullByte", tags.Tags{}, "smallint", "INTEGER"},
	}
	for _, c := range cases {
		got, err := Map(c.goType, dialect.Postgres, c.tg)
		if err == nil && schema.NormalizeType(dialect.Postgres, got) != got {
			t.Errorf("postgres Map(%s)=%q is not canonical: NormalizeType gives %q", c.goType, got, schema.NormalizeType(dialect.Postgres, got))
		}
		if err != nil || got != c.pg {
			t.Errorf("postgres Map(%s)=%q,%v want %q", c.goType, got, err, c.pg)
		}
		got, err = Map(c.goType, dialect.SQLite, c.tg)
		if err == nil && schema.NormalizeType(dialect.SQLite, got) != got {
			t.Errorf("sqlite Map(%s)=%q is not canonical: NormalizeType gives %q", c.goType, got, schema.NormalizeType(dialect.SQLite, got))
		}
		if c.sqlite == "" {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("sqlite Map(%s)=%q,%v want ErrUnsupported", c.goType, got, err)
			}
			continue
		}
		if err != nil || got != c.sqlite {
			t.Errorf("sqlite Map(%s)=%q,%v want %q", c.goType, got, err, c.sqlite)
		}
	}
}

func TestGeneratorTypeMapUnsupported(t *testing.T) {
	bad := []struct {
		goType string
		d      dialect.Name
		tg     tags.Tags
	}{
		{"uint", dialect.Postgres, tags.Tags{}},
		{"uint8", dialect.SQLite, tags.Tags{}},
		{"uint32", dialect.Postgres, tags.Tags{}},
		{"complex128", dialect.Postgres, tags.Tags{}},
		{"map[string]int", dialect.Postgres, tags.Tags{}},
		{"chan int", dialect.SQLite, tags.Tags{}},
		{"mypkg.Custom", dialect.Postgres, tags.Tags{}},
		{"", dialect.Postgres, tags.Tags{}},
		{"int64", dialect.MySQL, tags.Tags{}},
		{"vector.Vector", dialect.Postgres, tags.Tags{}}, // missing dim tag
		{"vector.Vector", dialect.SQLite, tags.Tags{Dim: 3}},
	}
	for _, c := range bad {
		got, err := Map(c.goType, c.d, c.tg)
		if !errors.Is(err, momo.ErrUnsupported) || got != "" {
			t.Errorf("Map(%q,%s)=%q,%v want ErrUnsupported", c.goType, c.d, got, err)
			continue
		}
		if c.goType != "" && !strings.Contains(err.Error(), c.goType) {
			t.Errorf("error %q does not name the Go type %q", err, c.goType)
		}
	}
}
