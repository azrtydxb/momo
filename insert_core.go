// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"slices"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// The bodies of InsertQuery[T] that read only the clauses and the table
// description live here, on a non-generic receiver, for the reason given in
// select_core.go: a method body on InsertQuery[T] is compiled again for every
// model type. The generic methods are one-line wrappers.

// insMeta is the part of Meta[T] the insert renderer reads that does not
// mention T.
type insMeta struct {
	Table       string
	Schema      string
	Columns     []ColumnMeta
	PK          []int
	IDGens      []IDGen
	TableSchema *schema.Table
	Identity    int
	Version     int
	SoftDelete  int
	CreatedAt   int
	UpdatedAt   int
}

// insMetaOf is go:noinline for the reason infoOf is.
//
//go:noinline
func insMetaOf[T any](m *Meta[T]) insMeta {
	if m == nil {
		return insMeta{}
	}
	return insMeta{Table: m.Table, Schema: m.Schema, Columns: m.Columns, PK: m.PK, IDGens: m.IDGens, TableSchema: m.TableSchema,
		Identity: m.Identity, Version: m.Version, SoftDelete: m.SoftDelete, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// insertRef is an insert statement stripped of T: its clauses and the table
// description. i points at the clauses of the query it was made from.
type insertRef struct {
	m insMeta
	i *insertCore
}

// ref returns the non-generic view of q; it points at q's own clauses.
func (q *InsertQuery[T]) ref() insertRef { return insertRef{m: insMetaOf(q.meta), i: &q.i} }

//go:noinline
func insertVerr(table, col, format string, args ...any) error {
	e := validation("insert", table, format, args...)
	e.Column = col
	return e
}

func (r insertRef) verr(col, format string, args ...any) error {
	return insertVerr(r.m.Table, col, format, args...)
}

// column returns the index of the mapped column name, or -1.
//
//go:noinline
func (r insertRef) column(name string) int {
	for i, c := range r.m.Columns {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// writable reports whether the column at i may appear in an INSERT.
//
//go:noinline
func (r insertRef) writable(i int) bool {
	return columnAssignable(writeInsert, r.m.Columns[i].Flags) || r.explicitCol(i)
}

// explicitCol reports whether column i is an identity column the caller named
// in Explicit, so the statement writes its value.
//
//go:noinline
func (r insertRef) explicitCol(i int) bool {
	if len(r.i.explicit) == 0 || i < 0 || i >= len(r.m.Columns) {
		return false
	}
	c := r.m.Columns[i]
	return c.Flags&FlagIdentity != 0 && c.Flags&FlagReadOnly == 0 && slices.Contains(r.i.explicit, c.Name)
}

// checkExplicit validates the Explicit list.
//
//go:noinline
func (r insertRef) checkExplicit() error {
	for _, n := range r.i.explicit {
		ci := r.column(n)
		switch {
		case ci < 0:
			return r.verr(n, "Explicit: column %q is not mapped by %s", n, r.m.Table)
		case slices.Contains(r.i.omit, n):
			return r.verr(n, "Explicit: column %q is also in Omit; use one or the other", n)
		case r.m.Columns[ci].Flags&FlagIdentity == 0 || r.m.Columns[ci].Flags&FlagReadOnly != 0:
			return r.verr(n, "Explicit: column %q is not an identity, serial or auto-increment column", n)
		}
	}
	return nil
}

// identityAlways reports whether column ci is GENERATED ALWAYS AS IDENTITY, so
// PostgreSQL needs OVERRIDING SYSTEM VALUE to accept a value for it. Without a
// table schema it answers true: the clause is harmless on the other kinds.
func (r insertRef) identityAlways(ci int) bool {
	if r.m.TableSchema != nil {
		for _, c := range r.m.TableSchema.Columns {
			if c.Name == r.m.Columns[ci].Name {
				return c.Identity == "always"
			}
		}
	}
	return true
}

// overriding reports whether the INSERT needs OVERRIDING SYSTEM VALUE: an
// upsert writes the identity key, or an explicit column is GENERATED ALWAYS.
//
//go:noinline
func (r insertRef) overriding(d *dialect.Dialect, cols []int) bool {
	if d.Name != dialect.Postgres {
		return false
	}
	if r.i.withIdentity {
		return true
	}
	for _, ci := range cols {
		if r.explicitCol(ci) && r.identityAlways(ci) {
			return true
		}
	}
	return false
}

// insertsIdentity reports whether column i is the identity column and an
// upsert writes its explicit values.
func (r insertRef) insertsIdentity(i int) bool {
	return r.i.withIdentity && i == r.m.Identity
}

// isNonPKIDGen reports whether column ci has an id generator and is not part of
// the primary key, e.g. a public id: a conflict must keep the stored value.
func (r insertRef) isNonPKIDGen(ci int) bool {
	if slices.Contains(r.m.PK, ci) {
		return false
	}
	return r.isIDGenColumn(ci)
}

// isIDGenColumn reports whether column ci takes a Go-side generated id. Those
// ids are written back by generateIDs and always win: LAST_INSERT_ID is read
// only for a key the database generates.
func (r insertRef) isIDGenColumn(ci int) bool {
	for _, g := range r.m.IDGens {
		if g.Column == ci {
			return true
		}
	}
	return false
}

// pkNames returns the primary-key column names.
func (r insertRef) pkNames() []string {
	names := make([]string, len(r.m.PK))
	for i, ci := range r.m.PK {
		names[i] = r.m.Columns[ci].Name
	}
	return names
}

// reservedRoleOf names the role of column ci when it is the primary key,
// version, soft-delete, created-at or updated-at column.
func (r insertRef) reservedRoleOf(ci int) string {
	switch {
	case slices.Contains(r.m.PK, ci):
		return "primary key"
	case ci == r.m.Version:
		return "version"
	case ci == r.m.SoftDelete:
		return "soft-delete"
	case ci == r.m.CreatedAt:
		return "created-at"
	case ci == r.m.UpdatedAt:
		return "updated-at"
	}
	return ""
}

// refuseUpdatedAtSet refuses an explicit DO UPDATE assignment to the
// updated-at column, with the error Update gives for the same Set.
func (r insertRef) refuseUpdatedAtSet() error {
	ci := r.m.UpdatedAt
	if ci < 0 || ci >= len(r.m.Columns) {
		return nil
	}
	for _, a := range r.i.set {
		if a.col == r.m.Columns[ci].Name {
			return r.verr(a.col, "DoUpdate: column %q is the updated-at column and cannot be set directly", a.col)
		}
	}
	return nil
}

// defaultUpsertSet assigns EXCLUDED to every column of the INSERT list that
// an upsert may update. models is the number of models; dbdCount holds, per
// column, the number of models that leave it to the database default (only read
// when there are models).
//
//go:noinline
func (r insertRef) defaultUpsertSet(models int, dbdCount []int) []Assignment {
	var out []Assignment
	if r.i.sel != nil {
		for _, n := range r.i.selCols {
			if ci := r.column(n); ci >= 0 && upsertUpdatable(r.m.Columns[ci].Flags) {
				out = append(out, Assignment{col: n, kind: setExcluded})
			}
		}
		return out
	}
	if models > 0 {
		omit := map[string]struct{}{}
		for _, n := range r.i.omit {
			omit[n] = struct{}{}
		}
		for i, c := range r.m.Columns {
			if _, skip := omit[c.Name]; skip || !r.writable(i) || !upsertUpdatable(c.Flags) || r.isNonPKIDGen(i) || (i < len(dbdCount) && dbdCount[i] > 0) {
				continue // a generated id or database default is never overwritten by a fresh one
			}
			out = append(out, Assignment{col: c.Name, kind: setExcluded})
		}
		return out
	}
	src := r.i.rows
	if !r.i.hasRows {
		src = [][]Assignment{r.i.assigns}
	}
	seen := map[string]struct{}{}
	for _, row := range src {
		for _, a := range row {
			ci := r.column(a.col)
			if _, dup := seen[a.col]; dup || ci < 0 || a.kind == setDefault || !upsertUpdatable(r.m.Columns[ci].Flags) {
				continue
			}
			seen[a.col] = struct{}{}
			out = append(out, Assignment{col: a.col, kind: setExcluded})
		}
	}
	return out
}

// writeInsertHead writes INSERT INTO [schema.]table.
func (r insertRef) writeInsertHead(b *Builder) {
	b.WriteString("INSERT INTO ")
	if r.m.Schema != "" {
		b.Ident(r.m.Schema)
		b.writeByte('.')
	}
	b.Ident(r.m.Table)
}

// appendAssignedValue renders one assigned value inside VALUES or SET.
func appendAssignedValue(b *Builder, a Assignment) {
	switch a.kind {
	case setValue:
		b.argFor(a.flags, a.val)
	case setExpr:
		b.exprFor(a.flags, a.expr)
	case setNull:
		b.WriteString("NULL")
	case setDefault:
		b.WriteString("DEFAULT")
	case setExcluded:
		b.writeExcluded(a.col)
	}
}

// renderConflict writes the conflict clause. written names the columns the
// statement writes, which MySQL's conflict-target rule needs.
//
//go:noinline
func (r insertRef) renderConflict(b *Builder, written []string) {
	if len(r.i.actionWhere) > 0 && r.i.conflict != conflictUpdate {
		b.SetErr(r.verr("", "Where is only valid after DoUpdate (or on Upsert): there is no DO UPDATE to condition"))
		return
	}
	if r.i.conflict == conflictNone {
		return
	}
	if b.Dialect().OnDuplicateKey {
		r.renderMySQLConflict(b, written)
		return
	}
	if !b.Dialect().OnConflict {
		b.unsupported("ON CONFLICT")
		return
	}
	for _, n := range r.i.target {
		if r.column(n) < 0 {
			b.SetErr(r.verr(n, "OnConflict: column %q is not mapped by %s", n, r.m.Table))
			return
		}
	}
	b.WriteString(" ON CONFLICT")
	if len(r.i.target) > 0 {
		b.WriteString(" (")
		for i, n := range r.i.target {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(n)
		}
		b.writeByte(')')
	}
	if len(r.i.targetWhere) > 0 {
		if len(r.i.target) == 0 {
			b.SetErr(r.verr("", "OnConflict(...).Where needs a conflict target: pass columns to OnConflict"))
			return
		}
		prev := b.unqualified
		b.unqualified, b.conflictTarget = true, true
		renderWhere(b, r.i.targetWhere)
		b.unqualified, b.conflictTarget = prev, false
	}
	if r.i.conflict == conflictNothing {
		b.WriteString(" DO NOTHING")
		return
	}
	if len(r.i.target) == 0 {
		b.SetErr(r.verr("", "DoUpdate needs a conflict target: pass columns to OnConflict"))
		return
	}
	if len(r.i.set) == 0 {
		b.SetErr(r.verr("", "DoUpdate needs at least one assignment"))
		return
	}
	b.WriteString(" DO UPDATE SET ")
	for i, a := range r.i.set {
		ci := r.column(a.col)
		if ci < 0 || !r.writable(ci) {
			b.SetErr(r.verr(a.col, "DoUpdate: column %q is not mapped or not writable", a.col))
			return
		}
		if a.kind == setDefault {
			b.SetErr(r.verr(a.col, "DoUpdate: SetDefault is not supported"))
			return
		}
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(a.col)
		b.WriteString(" = ")
		appendAssignedValue(b, a)
	}
	if len(r.i.actionWhere) > 0 {
		renderWhere(b, r.i.actionWhere)
	}
}

// unsuppliedIdentity returns the names of the auto-increment columns the
// INSERT leaves to the database: the identity column when it is absent from
// written, the column list this statement really writes (the resolved model
// columns, the Values columns or the FromSelect target columns). A Go-side
// generated id counts as supplied.
func (r insertRef) unsuppliedIdentity(written []string) []string {
	id := r.m.Identity
	if id < 0 || id >= len(r.m.Columns) || r.m.Columns[id].Flags&FlagIdentity == 0 || r.isIDGenColumn(id) {
		return nil
	}
	name := r.m.Columns[id].Name
	if slices.Contains(written, name) {
		return nil
	}
	return []string{name}
}

// renderMySQLConflict renders the conflict clause of an INSERT for the MySQL
// dialect as ON DUPLICATE KEY UPDATE.
//
//go:noinline
func (r insertRef) renderMySQLConflict(b *Builder, written []string) {
	if len(r.i.targetWhere) > 0 || len(r.i.actionWhere) > 0 {
		b.unsupported("a WHERE on a conflict clause (MySQL ON DUPLICATE KEY UPDATE has no conflict condition)")
		return
	}
	for _, n := range r.i.target {
		if r.column(n) < 0 {
			b.SetErr(r.verr(n, "OnConflict: column %q is not mapped by %s", n, r.m.Table))
			return
		}
	}
	pk := r.pkNames()
	if err := checkMySQLConflictTarget(r.m.TableSchema, r.i.target, pk, r.unsuppliedIdentity(written)); err != nil {
		b.SetErr(err)
		return
	}
	fromValues := r.i.sel == nil
	if r.i.conflict == conflictNothing {
		key := ""
		switch {
		case len(pk) > 0:
			key = pk[0]
		case len(r.m.Columns) > 0:
			key = r.m.Columns[0].Name
		}
		if key == "" {
			b.SetErr(r.verr("", "DoNothing needs a column to leave unchanged, and %s maps none", r.m.Table))
			return
		}
		renderMySQLUpsert(b, b.d.Flavor, r.m.Table, key, nil, true, fromValues, appendAssignedValueTo)
		return
	}
	if len(r.i.target) == 0 {
		b.SetErr(r.verr("", "DoUpdate needs a conflict target: pass columns to OnConflict"))
		return
	}
	if len(r.i.set) == 0 {
		b.SetErr(r.verr("", "DoUpdate needs at least one assignment"))
		return
	}
	for _, a := range r.i.set {
		ci := r.column(a.col)
		if ci < 0 || !r.writable(ci) {
			b.SetErr(r.verr(a.col, "DoUpdate: column %q is not mapped or not writable", a.col))
			return
		}
		if a.kind == setDefault {
			b.SetErr(r.verr(a.col, "DoUpdate: SetDefault is not supported"))
			return
		}
	}
	renderMySQLUpsert(b, b.d.Flavor, r.m.Table, "", r.i.set, false, fromValues, appendAssignedValueTo)
}

// appendAssignedValueTo adapts appendAssignedValue to the callback form.
func appendAssignedValueTo(b *Builder, a Assignment) { appendAssignedValue(b, a) }
