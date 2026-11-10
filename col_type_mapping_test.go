// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/gen/typemap"
	"github.com/azrtydxb/momo/types"
)

// Provenance of a documented mapping.
const (
	srcGenerator = "generator" // gen/typemap.Map decides
	srcCodec     = "codec"     // the type's own SQLType method decides
	srcManual    = "manual"    // not generated; the user writes the DDL
)

// mappingRow is one documented Go-to-SQL mapping. docs/TYPE_MAPPING.md must
// contain exactly these rows, and each row's PostgreSQL type must be what the
// code produces and what the live round trip (col_type_mapping_integration_test.go)
// stores into.
type mappingRow struct {
	goType string // first cell of the doc row, without backticks
	pg     string // PostgreSQL cell
	sqlite string // SQLite cell; "n/a" for manual rows
	src    string
	// mapType is the Go type string passed to typemap.Map (srcGenerator).
	mapType string
	tags    tags.Tags
	// codecType returns the type's own SQLType per dialect (srcCodec).
	codecType func(dialect.Name) string
}

var mappingRows = []mappingRow{
	{goType: "int", pg: "bigint", sqlite: "INTEGER", src: srcGenerator, mapType: "int"},
	{goType: "int8", pg: "smallint", sqlite: "INTEGER", src: srcGenerator, mapType: "int8"},
	{goType: "int16", pg: "smallint", sqlite: "INTEGER", src: srcGenerator, mapType: "int16"},
	{goType: "int32", pg: "integer", sqlite: "INTEGER", src: srcGenerator, mapType: "int32"},
	{goType: "int64", pg: "bigint", sqlite: "INTEGER", src: srcGenerator, mapType: "int64"},
	{goType: "float32", pg: "real", sqlite: "REAL", src: srcGenerator, mapType: "float32"},
	{goType: "float64", pg: "double precision", sqlite: "REAL", src: srcGenerator, mapType: "float64"},
	{goType: "bool", pg: "boolean", sqlite: "INTEGER", src: srcGenerator, mapType: "bool"},
	{goType: "string", pg: "text", sqlite: "TEXT", src: srcGenerator, mapType: "string"},
	{goType: "string with size:N", pg: "character varying(N)", sqlite: "TEXT", src: srcGenerator, mapType: "string", tags: tags.Tags{Size: 3}},
	{goType: "[]byte", pg: "bytea", sqlite: "BLOB", src: srcGenerator, mapType: "[]byte"},
	{goType: "time.Time", pg: "timestamp with time zone", sqlite: "TEXT", src: srcGenerator, mapType: "time.Time"},
	{goType: "types.UUID", pg: "uuid", sqlite: "TEXT", src: srcGenerator, mapType: "types.UUID"},
	{goType: "uuid.UUID (github.com/google/uuid)", pg: "uuid", sqlite: "TEXT", src: srcGenerator, mapType: "uuid.UUID"},
	{goType: "types.Date", pg: "date", sqlite: "TEXT", src: srcGenerator, mapType: "types.Date"},
	{goType: "types.JSON[T]", pg: "jsonb", sqlite: "TEXT", src: srcGenerator, mapType: "types.JSON[T]"},
	{goType: "vector.Vector", pg: "vector(N)", sqlite: "unsupported", src: srcGenerator, mapType: "vector.Vector", tags: tags.Tags{Dim: 3}},
	{goType: "vector.HalfVector", pg: "halfvec(N)", sqlite: "unsupported", src: srcGenerator, mapType: "vector.HalfVector", tags: tags.Tags{Dim: 3}},
	{goType: "vector.SparseVector", pg: "sparsevec(N)", sqlite: "unsupported", src: srcGenerator, mapType: "vector.SparseVector", tags: tags.Tags{Dim: 3}},
	{goType: "vector.Bits", pg: "bit(N)", sqlite: "unsupported", src: srcGenerator, mapType: "vector.Bits", tags: tags.Tags{Dim: 3}},
	{goType: "[]string", pg: "text[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]string"},
	{goType: "[]int16", pg: "smallint[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]int16"},
	{goType: "[]int32", pg: "integer[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]int32"},
	{goType: "[]int64", pg: "bigint[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]int64"},
	{goType: "[]bool", pg: "boolean[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]bool"},
	{goType: "[]float32", pg: "real[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]float32"},
	{goType: "[]float64", pg: "double precision[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]float64"},
	{goType: "[]types.UUID", pg: "uuid[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]types.UUID"},
	{goType: "[]uuid.UUID (github.com/google/uuid)", pg: "uuid[]", sqlite: "unsupported", src: srcGenerator, mapType: "[]uuid.UUID"},
	{goType: "vector.Vector (no dim)", pg: "vector", sqlite: "unsupported", src: srcGenerator, mapType: "vector.Vector"},
	{goType: "vector.HalfVector (no dim)", pg: "halfvec", sqlite: "unsupported", src: srcGenerator, mapType: "vector.HalfVector"},
	{goType: "types.TimeOfDay", pg: "time", sqlite: "TEXT", src: srcCodec, codecType: types.TimeOfDay{}.SQLType},
	{goType: "types.Hstore", pg: "hstore", sqlite: "TEXT", src: srcCodec, codecType: types.Hstore{}.SQLType},
	{goType: "types.TSVector", pg: "tsvector", sqlite: "TEXT", src: srcCodec, codecType: types.TSVector("").SQLType},
	{goType: "types.TSQuery", pg: "tsquery", sqlite: "TEXT", src: srcCodec, codecType: types.TSQuery("").SQLType},
	{goType: "time.Time (timestamp)", pg: "timestamp", sqlite: "n/a", src: srcManual},
	{goType: "string (decimal)", pg: "numeric", sqlite: "n/a", src: srcManual},
}

const typeMappingDoc = "docs/TYPE_MAPPING.md"

// docRowRE matches a three-cell table row whose first cell is a code span.
// A generated row (internal/gendocs) carries a fourth cell for MySQL, which the
// type map does not cover; it is read and ignored here.
var docRowRE = regexp.MustCompile("^\\|\\s*`([^`]+)`\\s*\\|([^|]*)\\|([^|]*)\\|(?:[^|]*\\|)?$")

// parseDocRows extracts the mapping rows (goType -> [pg, sqlite]) from the
// document. Mapping tables are the only tables on the page whose first cell is
// a code span and that have three or four columns, so other tables are
// ignored.
func parseDocRows(t *testing.T) map[string][2]string {
	t.Helper()
	b, err := os.ReadFile(typeMappingDoc)
	if err != nil {
		t.Fatal(err)
	}
	cell := func(s string) string { return strings.Trim(strings.TrimSpace(s), "`") }
	out := map[string][2]string{}
	for _, line := range strings.Split(string(b), "\n") {
		m := docRowRE.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		if _, dup := out[m[1]]; dup {
			t.Errorf("%s: duplicate row %q", typeMappingDoc, m[1])
		}
		out[m[1]] = [2]string{cell(m[2]), cell(m[3])}
	}
	return out
}

// withTag substitutes the (N) placeholder in a documented cell with the tag
// value the test passed to the generator.
func withTag(cell string, tg tags.Tags) string {
	n := tg.Size
	if tg.Dim > 0 {
		n = tg.Dim
	}
	return strings.ReplaceAll(cell, "(N)", fmt.Sprintf("(%d)", n))
}

// TestTypeMappingDoc keeps docs/TYPE_MAPPING.md and the code in step: the
// document holds exactly the rows in mappingRows, and every row's cells equal
// what the generator's type map (or the codec's SQLType) returns.
func TestTypeMappingDoc(t *testing.T) {
	doc := parseDocRows(t)
	seen := map[string]bool{}
	for _, r := range mappingRows {
		seen[r.goType] = true
		cells, ok := doc[r.goType]
		if !ok {
			t.Errorf("%s is missing row %q", typeMappingDoc, r.goType)
			continue
		}
		if cells[0] != r.pg || cells[1] != r.sqlite {
			t.Errorf("row %q: doc says (%s | %s), table says (%s | %s)", r.goType, cells[0], cells[1], r.pg, r.sqlite)
		}
		switch r.src {
		case srcGenerator:
			for _, c := range []struct {
				d    dialect.Name
				want string
			}{{dialect.Postgres, r.pg}, {dialect.SQLite, r.sqlite}} {
				got, err := typemap.Map(r.mapType, c.d, r.tags)
				if c.want == "unsupported" {
					if err == nil {
						t.Errorf("%s on %s: Map returned %q, doc says unsupported", r.goType, c.d, got)
					}
					continue
				}
				if want := withTag(c.want, r.tags); err != nil || got != want {
					t.Errorf("%s on %s: Map = %q, %v; doc says %q", r.goType, c.d, got, err, want)
				}
			}
		case srcCodec:
			if got := r.codecType(dialect.Postgres); got != r.pg {
				t.Errorf("%s: SQLType(postgres) = %q, doc says %q", r.goType, got, r.pg)
			}
			if got := r.codecType(dialect.SQLite); got != r.sqlite {
				t.Errorf("%s: SQLType(sqlite) = %q, doc says %q", r.goType, got, r.sqlite)
			}
		case srcManual:
			// Not generated: the user writes the DDL, nothing to compare in code.
		}
	}
	for k := range doc {
		if !seen[k] {
			t.Errorf("%s documents %q but no test row covers it", typeMappingDoc, k)
		}
	}
}
