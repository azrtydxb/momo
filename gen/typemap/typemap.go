// SPDX-License-Identifier: Apache-2.0

// Package typemap maps the Go types the generator understands to SQL column
// types per dialect. A codec's own SQLType method wins over this table; that
// lookup is the caller's job.
package typemap

import (
	"fmt"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
)

// nullTypes maps database/sql Null* wrappers to the type they wrap.
var nullTypes = map[string]string{
	"sql.NullBool": "bool",
	// sql.NullByte wraps a Go byte (uint8). uint8 itself is unsupported, but
	// database/sql stores the wrapper as a smallint because 0..255 does not
	// fit a signed 8-bit column, so it maps like int16.
	"sql.NullByte":    "int16",
	"sql.NullFloat64": "float64",
	"sql.NullInt16":   "int16",
	"sql.NullInt32":   "int32",
	"sql.NullInt64":   "int64",
	"sql.NullString":  "string",
	"sql.NullTime":    "time.Time",
}

// googleUUID is how the loader spells github.com/google/uuid.UUID for the type
// map (see gen.baseType, which matches the import path). The type map names
// the type by string, so it does not import the package.
const googleUUID = "uuid.UUID"

// Map returns the SQL type for goType on dialect d. Pointer and sql.Null*
// wrappers map like the type they wrap: nullability is a column property, not
// a type property. Unknown types, unsupported dialects and unrepresentable
// combinations return an error wrapping momo.ErrUnsupported; the generator
// reports it at the field's position.
func Map(goType string, d dialect.Name, tg tags.Tags) (string, error) {
	t := strings.TrimPrefix(goType, "*")
	if w, ok := nullTypes[t]; ok {
		t = w
	}
	var sql string
	switch d {
	case dialect.Postgres:
		sql = postgres(t, tg)
	case dialect.SQLite:
		sql = sqlite(t, tg)
	}
	if sql == "" {
		return "", fmt.Errorf("%w: Go type %s has no SQL mapping on %s", momo.ErrUnsupported, goType, d)
	}
	return sql, nil
}

func postgres(t string, tg tags.Tags) string {
	switch t {
	case "int8", "int16":
		return "smallint"
	case "int32":
		return "integer"
	case "int", "int64":
		return "bigint"
	case "float32":
		return "real"
	case "float64":
		return "double precision"
	case "bool":
		return "boolean"
	case "string":
		if tg.Size > 0 {
			return fmt.Sprintf("character varying(%d)", tg.Size)
		}
		return "text"
	case "[]byte":
		return "bytea"
	case "time.Time":
		return "timestamp with time zone"
	case "types.UUID", googleUUID:
		return "uuid"
	case "types.Date":
		return "date"
	case "types.Numeric":
		return "numeric"
	case "vector.Vector":
		return withDim("vector", tg, true)
	case "vector.HalfVector":
		return withDim("halfvec", tg, true)
	case "vector.SparseVector":
		return withDim("sparsevec", tg, false)
	case "vector.Bits":
		return withDim("bit", tg, false)
	}
	if strings.HasPrefix(t, "types.JSON[") && strings.HasSuffix(t, "]") {
		return "jsonb"
	}
	if elem, ok := strings.CutPrefix(t, "[]"); ok && elem != "byte" && elem != "uint8" {
		if elemSQL := postgresElem(elem, tg); elemSQL != "" {
			return elemSQL + "[]"
		}
	}
	return ""
}

// postgresElem is the PostgreSQL element type of a slice column: the scalar
// Go types that have a plain array type, and types.UUID (uuid[]). Sized strings keep their length
// (character varying(N)[]). Nested slices, byte slices and codec types are
// not arrays.
func postgresElem(elem string, tg tags.Tags) string {
	switch elem {
	case "string", "int16", "int32", "int64", "bool", "float32", "float64", "types.UUID", googleUUID:
		return postgres(elem, tg)
	}
	return ""
}

// sqlite maps to SQLite storage classes. time.Time is stored as RFC3339 text
// in UTC; JSON, UUID, Date and Numeric are stored as text. Vector types have no SQLite
// column type.
func sqlite(t string, _ tags.Tags) string {
	switch t {
	case "int", "int8", "int16", "int32", "int64", "bool":
		return "INTEGER"
	case "float32", "float64":
		return "REAL"
	case "string", "time.Time", "types.UUID", googleUUID, "types.Date", "types.Numeric":
		return "TEXT"
	case "[]byte":
		return "BLOB"
	}
	if strings.HasPrefix(t, "types.JSON[") && strings.HasSuffix(t, "]") {
		return "TEXT"
	}
	return ""
}

// withDim renders a dimensioned type. The dim tag is required unless bare is
// true, in which case a missing dim gives the dimensionless type (a vector or
// halfvec column that holds any size).
func withDim(name string, tg tags.Tags, bare bool) string {
	if tg.Dim <= 0 {
		if bare {
			return name
		}
		return ""
	}
	return fmt.Sprintf("%s(%d)", name, tg.Dim)
}
