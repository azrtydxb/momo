// SPDX-License-Identifier: Apache-2.0

package fromdb

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// ErrUnmapped is wrapped by MapType for a SQL type with no Go mapping. A
// column of such a type is never given a fallback type: add an introspect
// types or columns rule instead.
var ErrUnmapped = errors.New("fromdb: unmapped column type")

const (
	importTypes  = "github.com/azrtydxb/momo/types"
	importVector = "github.com/azrtydxb/momo/vector"

	importGoogleUUID = "github.com/google/uuid"
	// UUIDGoogle is the introspect.uuid value that writes github.com/google/uuid
	// types for uuid columns; the default (empty or "momo") writes types.UUID.
	UUIDGoogle = "google"
	// UUIDMomo is the default introspect.uuid value: types.UUID.
	UUIDMomo = "momo"
)

// GoType is the Go type chosen for a SQL column type.
type GoType struct {
	// Expr is the Go type without a pointer, for example "int64" or
	// "types.UUID".
	Expr string
	// Import is the import path Expr needs, empty for none.
	Import string
	// Nilable is set for slices and maps. They hold NULL as nil, so a
	// nullable column gets the nullable tag instead of a pointer.
	Nilable bool
	// Size is the length of a character varying(N) column, for the size tag.
	Size int
	// Dim is the dimension of a vector family column, for the dim tag.
	Dim int
	// Note is a report note for a mapping the reader should know about.
	Note string
}

// This table is the single place that maps a PostgreSQL SQL type to a Go
// type. The keys are schema.NormalizeType spellings without arguments.
var postgresTypes = map[string]GoType{
	"smallint":         {Expr: "int16"},
	"integer":          {Expr: "int32"},
	"bigint":           {Expr: "int64"},
	"real":             {Expr: "float32"},
	"double precision": {Expr: "float64"},
	"boolean":          {Expr: "bool"},
	"text":             {Expr: "string"},
	"character":        {Expr: "string"},
	"bytea":            {Expr: "[]byte", Nilable: true},
	"uuid":             {Expr: "types.UUID", Import: importTypes},
	"date":             {Expr: "types.Date", Import: importTypes},
	"jsonb":            {Expr: "types.JSON[json.RawMessage]", Import: "encoding/json"},
	"json":             {Expr: "types.JSON[json.RawMessage]", Import: "encoding/json"},
	"hstore":           {Expr: "types.Hstore", Import: importTypes, Nilable: true},
	"tsvector":         {Expr: "types.TSVector", Import: importTypes},
	"numeric":          {Expr: "string", Note: "decimal-as-string: numeric is carried as a string; set an introspect types rule for a decimal type"},

	"timestamp without time zone": {Expr: "time.Time", Import: "time"},
	"timestamp with time zone":    {Expr: "time.Time", Import: "time"},
	"time without time zone":      {Expr: "types.TimeOfDay", Import: importTypes},
}

// postgresArrays maps the element type of a one-dimensional array.
var postgresArrays = map[string]string{
	"smallint":          "[]int16",
	"integer":           "[]int32",
	"bigint":            "[]int64",
	"real":              "[]float32",
	"double precision":  "[]float64",
	"boolean":           "[]bool",
	"text":              "[]string",
	"character varying": "[]string",
	"uuid":              "[]types.UUID",
}

// postgresVectors maps the pgvector family to its Go type.
var postgresVectors = map[string]string{
	"vector":    "vector.Vector",
	"halfvec":   "vector.HalfVector",
	"sparsevec": "vector.SparseVector",
	"bit":       "vector.Bits",
}

// sqliteTypes maps the SQLite storage classes and common affinities.
var sqliteTypes = map[string]GoType{
	"INTEGER":   {Expr: "int64"},
	"REAL":      {Expr: "float64"},
	"TEXT":      {Expr: "string"},
	"BLOB":      {Expr: "[]byte", Nilable: true},
	"NUMERIC":   {Expr: "float64", Note: "numeric-as-float64: SQLite NUMERIC is carried as float64"},
	"BOOLEAN":   {Expr: "bool"},
	"DATETIME":  {Expr: "time.Time", Import: "time"},
	"TIMESTAMP": {Expr: "time.Time", Import: "time"},
}

// MapType returns the Go type for a normalized SQL type (schema.NormalizeType
// output) on dialect d. A type it does not know returns an error wrapping
// ErrUnmapped; there is no fallback to string.
func MapType(d dialect.Name, sqlType string) (GoType, error) {
	s := schema.NormalizeType(d, sqlType)
	switch d {
	case dialect.Postgres:
		if g, ok := mapPostgres(s); ok {
			return g, nil
		}
	case dialect.SQLite:
		if g, ok := sqliteTypes[s]; ok {
			return g, nil
		}
	}
	return GoType{}, fmt.Errorf("%w: %q on %s", ErrUnmapped, sqlType, d)
}

// splitType splits "base(args) rest" into its three parts.
func splitType(s string) (base, args, rest string) {
	i := strings.IndexByte(s, '(')
	if i < 0 {
		return strings.TrimSpace(s), "", ""
	}
	j := strings.IndexByte(s[i:], ')')
	if j < 0 {
		return strings.TrimSpace(s[:i]), "", ""
	}
	return strings.TrimSpace(s[:i]), s[i+1 : i+j], strings.TrimSpace(s[i+j+1:])
}

func mapPostgres(s string) (GoType, bool) {
	if elem, ok := strings.CutSuffix(s, "[]"); ok {
		base, _, _ := splitType(elem)
		if g, ok := postgresArrays[base]; ok {
			out := GoType{Expr: g, Nilable: true}
			if strings.Contains(g, "types.") {
				out.Import = importTypes
			}
			return out, true
		}
		return GoType{}, false
	}
	base, args, rest := splitType(s)
	full := strings.TrimSpace(base + " " + rest)
	if g, ok := postgresTypes[full]; ok {
		return g, true
	}
	switch base {
	case "character varying":
		g := GoType{Expr: "string"}
		if args != "" {
			n, err := strconv.Atoi(args)
			if err != nil || n <= 0 {
				return GoType{}, false
			}
			g.Size = n
		}
		return g, true
	case "character", "numeric":
		return postgresTypes[base], true
	}
	if expr, ok := postgresVectors[base]; ok && rest == "" {
		g := GoType{Expr: expr, Import: importVector}
		if args != "" {
			n, err := strconv.Atoi(args)
			if err != nil || n <= 0 {
				return GoType{}, false
			}
			g.Dim = n
		}
		return g, true
	}
	return GoType{}, false
}
