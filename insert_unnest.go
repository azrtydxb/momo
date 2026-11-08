// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

// Unnest switches a Model or Models insert to one statement that binds one
// array parameter per column:
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
func (q InsertQuery[T]) Unnest() InsertQuery[T] {
	q.i.unnest = true
	return q
}

// unnestCheck rejects the combinations Unnest cannot render.
func (q InsertQuery[T]) unnestCheck(d *dialect.Dialect, mode insertMode) error {
	if d.Name != dialect.Postgres {
		return &Error{Kind: ErrUnsupported, Op: "insert", Table: q.meta.Table,
			Err: fmt.Errorf("Unnest needs PostgreSQL array parameters, not %s", d.Name)}
	}
	if mode != modeModels {
		return q.verr("", "Unnest works with Model and Models only")
	}
	if q.meta.TableSchema == nil {
		return q.verr("", "Unnest needs the table schema in the generated metadata for the column types")
	}
	return nil
}

// writeUnnest writes the column list and the unnest row source of a plan: one
// array parameter per written column, cast to the column's array type.
func (q InsertQuery[T]) writeUnnest(b *Builder, d *dialect.Dialect, p insertPlan[T]) {
	types := make(map[string]string, len(q.meta.TableSchema.Columns))
	for _, c := range q.meta.TableSchema.Columns {
		types[c.Name] = c.Type
	}
	b.WriteString(" (")
	for i, ci := range p.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(q.meta.Columns[ci].Name)
	}
	b.writeByte(')')
	if q.i.withIdentity && d.Name == dialect.Postgres {
		b.WriteString(" OVERRIDING SYSTEM VALUE")
	}
	b.WriteString(" SELECT * FROM unnest(")
	for i, ci := range p.cols {
		name := q.meta.Columns[ci].Name
		typ := types[name]
		if typ == "" {
			b.SetErr(q.verr(name, "Unnest: column %q has no type in the table schema", name))
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		col := make([]any, len(q.models))
		for r := range q.models {
			if p.vals != nil {
				col[r] = p.vals[r][i]
			} else {
				col[r] = q.modelValue(&q.models[r], ci, p.now, p.fp)
			}
		}
		b.argFor(q.meta.Columns[ci].Flags, col)
		b.WriteString("::")
		b.WriteString(typ)
		b.WriteString("[]")
	}
	b.writeByte(')')
}
