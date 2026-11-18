// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// Unnest switches a Model or Models insert to one statement that binds one
// array parameter per column. It is a method of InsertQuery and is not the
// package-level FROM source Unnest (see UnnestValues), which turns bound arrays
// into rows of a SELECT.
//
//	INSERT INTO t (c1, c2) SELECT * FROM unnest($1::bigint[], $2::text[])
//
// The statement has as many bind parameters as the table has written columns,
// however many rows are inserted, so the bind-parameter limit does not split
// the insert into chunks. The column array types come from the table schema
// recorded in the generated metadata, so a model without a table schema fails
// with ErrValidation. Generated keys come back through RETURNING as for any
// insert. PostgreSQL only: other dialects fail with ErrUnsupported. Values and
// ValuesRows inserts are not supported, because their columns can differ per
// row.
//
// Column values are bound as typed slices (int, float, bool, string,
// time.Time and []byte columns, as []*T when a value is NULL). An element type
// this does not know (uuid, json, decimal, a named enum) falls back to []any,
// which pgx may reject for the array type; use Models for such a model.
// OnConflict, Returning, hooks, interceptors, automatic timestamps and id
// generators work as for Models. A sensitive column's array is redacted in
// query events.
func (q InsertQuery[T]) Unnest() InsertQuery[T] {
	q.i.unnest = true
	return q
}

// unnestCheck rejects the combinations Unnest cannot render.
func (r insertRef) unnestCheck(d *dialect.Dialect, mode insertMode) error {
	if d.Name != dialect.Postgres {
		return &Error{Kind: ErrUnsupported, Op: "insert", Table: r.m.Table,
			Err: fmt.Errorf("Unnest needs PostgreSQL array parameters, not %s", d.Name)}
	}
	if mode != modeModels {
		return r.verr("", "Unnest works with Model and Models only")
	}
	if r.m.TableSchema == nil {
		return r.verr("", "Unnest needs the table schema in the generated metadata for the column types")
	}
	return nil
}

// writeUnnest writes the column list and the unnest row source of a plan: one
// array parameter per written column, cast to the column's array type.
func (r insertRef) writeUnnest(b *Builder, d *dialect.Dialect, p insertPlan, a insAccess) {
	b.WriteString(" (")
	for i, ci := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(r.m.Columns[ci].Name)
	}
	b.writeByte(')')
	if r.overriding(d, p.cols) {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" SELECT * FROM unnest(")
	for i, ci := range p.cols {
		name := r.m.Columns[ci].Name
		typ := r.schemaColumnType(name)
		if typ == "" {
			b.SetErr(r.verr(name, "Unnest: column %q has no type in the table schema", name))
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		col := make([]any, r.n)
		for row := range r.n {
			if p.vals != nil {
				col[row] = p.vals[row][i]
			} else {
				col[row] = r.modelValue(row, ci, p.now, a)
			}
		}
		b.argFor(r.m.Columns[ci].Flags, typedColumn(col))
		b.WriteString("::")
		b.WriteString(typ)
		b.WriteString("[]")
	}
	b.writeByte(')')
}

// schemaColumnType returns the type the table schema records for the column
// name, or "" when it has none. It scans the schema columns instead of
// building a map, so a render allocates nothing for the lookup.
func (r insertRef) schemaColumnType(name string) string {
	cols := r.m.TableSchema.Columns
	for i := range cols {
		if cols[i].Name == name {
			return cols[i].Type
		}
	}
	return ""
}

// typedColumn turns the values of one column into a typed slice that pgx can
// encode as a PostgreSQL array: []int64 for int64 values, []*string when some
// values are NULL, and so on. A column of values this does not know stays a
// []any, which pgx may reject for the array type; use Models for such a model.
func typedColumn(vals []any) any {
	for _, v := range vals {
		switch v.(type) {
		case nil:
			continue
		case int64, *int64:
			return columnOf[int64](vals)
		case int32, *int32:
			return columnOf[int32](vals)
		case int16, *int16:
			return columnOf[int16](vals)
		case int, *int:
			return columnOf[int](vals)
		case float64, *float64:
			return columnOf[float64](vals)
		case float32, *float32:
			return columnOf[float32](vals)
		case bool, *bool:
			return columnOf[bool](vals)
		case string, *string:
			return columnOf[string](vals)
		case time.Time, *time.Time:
			return columnOf[time.Time](vals)
		case []byte:
			return columnOf[[]byte](vals)
		}
		break
	}
	return vals
}

// columnOf returns []T when every value is a T, and []*T when some value is a
// *T or NULL, so a NULL stays a NULL. If a value is neither it returns vals.
func columnOf[T any](vals []any) any {
	plain := make([]T, len(vals))
	allPlain := true
	for i, v := range vals {
		x, ok := v.(T)
		if !ok {
			allPlain = false
			break
		}
		plain[i] = x
	}
	if allPlain {
		return plain
	}
	ptrs := make([]*T, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case nil:
		case T:
			ptrs[i] = &x
		case *T:
			ptrs[i] = x
		default:
			return vals
		}
	}
	return ptrs
}
