// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

// mysqlRowAlias names the inserted row in MySQL's INSERT ... AS alias ON
// DUPLICATE KEY UPDATE form (MySQL 8.0.19 and later).
const mysqlRowAlias = "momo_new"

// renderMySQLUpsert writes the tail of an upsert, after the VALUES list:
// the row alias (MySQL, when the rows come from VALUES) and
// ON DUPLICATE KEY UPDATE with one assignment per set. MariaDB has no row
// alias that is verified on 11.8, so its assignments use VALUES(col). With
// doNothing it writes the no-op assignment firstKey = table.firstKey, which is
// how MySQL skips a conflicting row. With clientFoundRows the statement then
// reports RowsAffected 1 for a skipped row (it matched) as well as for an
// inserted one, so a count of 1 does not tell the two apart. value renders the right-hand side of one
// assignment.
func renderMySQLUpsert(b *Builder, flavor dialect.Flavor, table, firstKey string, sets []Assignment, doNothing, fromValues bool, value func(*Builder, Assignment)) {
	if flavor == dialect.FlavorMySQL && fromValues {
		b.WriteString(" AS ")
		b.Ident(mysqlRowAlias)
		b.excluded = mysqlRowAlias
	} else {
		b.excluded = ""
	}
	b.odku = true
	defer func() { b.odku, b.excluded = false, "" }()
	b.WriteString(" ON DUPLICATE KEY UPDATE ")
	if doNothing {
		// Qualified on the right: with a row alias the bare name is ambiguous.
		b.Ident(firstKey)
		b.WriteString(" = ")
		b.QualifiedIdent(table, firstKey)
		return
	}
	for i, a := range sets {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(a.col)
		b.WriteString(" = ")
		value(b, a)
	}
}

// writeExcluded writes the value proposed for insertion: EXCLUDED.col on
// PostgreSQL and SQLite, the row alias column on MySQL, VALUES(col) on MariaDB.
func (b *Builder) writeExcluded(col string) {
	switch {
	case !b.odku:
		b.WriteString("EXCLUDED.")
		b.Ident(col)
	case b.excluded != "":
		b.Ident(b.excluded)
		b.writeByte('.')
		b.Ident(col)
	default:
		b.WriteString("VALUES(")
		b.Ident(col)
		b.writeByte(')')
	}
}

// checkMySQLConflictTarget returns ErrUnsupported unless target is a unique
// key of the table and no other unique key can conflict. MySQL's
// ON DUPLICATE KEY UPDATE fires for a conflict on any unique key and cannot
// name one, so with a second unique key the statement would also update the
// row that conflicts on the key the caller did not name. A second key is
// harmless when it consists only of columns in unsupplied: auto-increment
// columns the insert leaves to the database. The server hands such a column a
// fresh value, so the key cannot match an existing row and the statement acts
// on the target alone (an auto-increment primary key next to UNIQUE(email),
// with the key omitted). With no schema (a hand-written Meta) the unique keys
// are unknown: only a target equal to pk is accepted, and the caller owns the
// claim that the table has no other unique key.
func checkMySQLConflictTarget(ts *schema.Table, target []string, pk []string, unsupplied []string) error {
	if len(target) == 0 {
		return nil
	}
	if ts == nil {
		if sameStrings(target, pk) {
			return nil
		}
		return mysqlTargetError(target, "the table has no schema, so only the primary key can be targeted")
	}
	var uniques [][]string
	if len(ts.PrimaryKey) > 0 {
		uniques = append(uniques, ts.PrimaryKey)
	}
	for _, u := range ts.Uniques {
		uniques = append(uniques, u.Columns)
	}
	for _, ix := range ts.Indexes {
		if !ix.Unique {
			continue
		}
		cols := make([]string, 0, len(ix.Parts))
		for _, p := range ix.Parts {
			cols = append(cols, p.Column) // an expression key has no column: it never counts as unsupplied
		}
		uniques = append(uniques, cols)
	}
	matched := false
	var reachable [][]string
	for _, u := range uniques {
		switch {
		case !matched && sameStrings(target, u):
			matched = true
		case onlyColumns(u, unsupplied):
			// cannot conflict: every column takes a fresh auto-increment value
		default:
			reachable = append(reachable, u)
		}
	}
	switch {
	case !matched:
		return mysqlTargetError(target, fmt.Sprintf("it is not a unique key of the table (the unique keys are %v)", uniques))
	case len(reachable) > 0:
		return mysqlTargetError(target, fmt.Sprintf("the table also has the unique key %v, which this insert can conflict on and ON DUPLICATE KEY UPDATE cannot tell apart; the target is accepted when every other unique key is made only of auto-increment columns the insert leaves to the database", reachable[0]))
	}
	return nil
}

// onlyColumns reports whether cols is not empty and every column is in set.
func onlyColumns(cols, set []string) bool {
	if len(cols) == 0 {
		return false
	}
	for _, c := range cols {
		if !slices.Contains(set, c) {
			return false
		}
	}
	return true
}

// unsuppliedIdentity returns the names of the auto-increment columns the
// INSERT leaves to the database: the identity column when it is absent from
// written, the column list this statement really writes (the resolved model
// columns, the Values columns or the FromSelect target columns). A Go-side
// generated id counts as supplied.
func (q InsertQuery[T]) unsuppliedIdentity(written []string) []string {
	id := q.meta.Identity
	if id < 0 || id >= len(q.meta.Columns) || q.meta.Columns[id].Flags&FlagIdentity == 0 || q.isIDGenColumn(id) {
		return nil
	}
	name := q.meta.Columns[id].Name
	if slices.Contains(written, name) {
		return nil
	}
	return []string{name}
}

func mysqlTargetError(target []string, why string) error {
	return &Error{Kind: ErrUnsupported, Op: "insert", Err: fmt.Errorf(
		"MySQL ON DUPLICATE KEY UPDATE cannot target %v: it fires for any unique key; %s", target, why)}
}

// renderMySQLConflict renders the conflict clause of an INSERT for the MySQL
// dialect as ON DUPLICATE KEY UPDATE.
func (q InsertQuery[T]) renderMySQLConflict(b *Builder, written []string) {
	if len(q.i.targetWhere) > 0 || len(q.i.actionWhere) > 0 {
		b.unsupported("a WHERE on a conflict clause (MySQL ON DUPLICATE KEY UPDATE has no conflict condition)")
		return
	}
	for _, n := range q.i.target {
		if q.column(n) < 0 {
			b.SetErr(q.verr(n, "OnConflict: column %q is not mapped by %s", n, q.meta.Table))
			return
		}
	}
	pk := q.pkNames()
	if err := checkMySQLConflictTarget(q.meta.TableSchema, q.i.target, pk, q.unsuppliedIdentity(written)); err != nil {
		b.SetErr(err)
		return
	}
	fromValues := q.i.sel == nil
	if q.i.conflict == conflictNothing {
		key := ""
		switch {
		case len(pk) > 0:
			key = pk[0]
		case len(q.meta.Columns) > 0:
			key = q.meta.Columns[0].Name
		}
		if key == "" {
			b.SetErr(q.verr("", "DoNothing needs a column to leave unchanged, and %s maps none", q.meta.Table))
			return
		}
		renderMySQLUpsert(b, b.d.Flavor, q.meta.Table, key, nil, true, fromValues, q.appendValueTo)
		return
	}
	if len(q.i.target) == 0 {
		b.SetErr(q.verr("", "DoUpdate needs a conflict target: pass columns to OnConflict"))
		return
	}
	if len(q.i.set) == 0 {
		b.SetErr(q.verr("", "DoUpdate needs at least one assignment"))
		return
	}
	for _, a := range q.i.set {
		ci := q.column(a.col)
		if ci < 0 || !q.writable(ci) {
			b.SetErr(q.verr(a.col, "DoUpdate: column %q is not mapped or not writable", a.col))
			return
		}
		if a.kind == setDefault {
			b.SetErr(q.verr(a.col, "DoUpdate: SetDefault is not supported"))
			return
		}
	}
	renderMySQLUpsert(b, b.d.Flavor, q.meta.Table, "", q.i.set, false, fromValues, q.appendValueTo)
}

// appendValueTo adapts appendValue to the callback form.
func (q InsertQuery[T]) appendValueTo(b *Builder, a Assignment) { q.appendValue(b, a) }

// wantsGeneratedKey reports whether execOne should read LAST_INSERT_ID and
// write it back: a single Model insert on MySQL or MariaDB without a conflict
// clause, whose identity column the statement leaves to the server.
func (q InsertQuery[T]) wantsGeneratedKey() bool {
	return len(q.back) == 1 && q.back[0] != nil && len(q.models) == 1 && q.leavesKeyToServer()
}

// leavesKeyToServer reports whether the statement is a plain INSERT on MySQL
// or MariaDB whose identity column the server generates (no conflict clause,
// no Go-side id, not named in Explicit).
func (q InsertQuery[T]) leavesKeyToServer() bool {
	id := q.meta.Identity
	return q.q != nil && q.q.Dialect().OnDuplicateKey &&
		!q.i.upsert && q.i.conflict == conflictNone && !q.explicitCol(id) && id >= 0 && id < len(q.meta.Columns) &&
		q.meta.Ptr != nil && q.meta.Columns[id].Flags&FlagIdentity != 0 && !q.isIDGenColumn(id)
}

// isIDGenColumn reports whether column ci takes a Go-side generated id. Those
// ids are written back by generateIDs and always win: LAST_INSERT_ID is read
// only for a key the database generates.
func (q InsertQuery[T]) isIDGenColumn(ci int) bool {
	for _, g := range q.meta.IDGens {
		if g.Column == ci {
			return true
		}
	}
	return false
}

// assignGeneratedKey stores id in the identity field of the model passed to
// Model (see identityIsInteger for the field kinds). A multi-row insert through
// Exec returns and writes back no keys, because the ids of a multi-row
// statement depend on innodb_autoinc_lock_mode; CreateGraph inserts one row per
// statement on MySQL for that reason. MariaDB callers who need keys use
// Returning, which renders INSERT ... RETURNING; MySQL has no equivalent and
// Returning is ErrUnsupported there.
func (q InsertQuery[T]) assignGeneratedKey(id int64) {
	p := q.meta.Ptr(q.back[0], q.meta.Identity)
	if p == nil {
		return
	}
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	ev := rv.Elem()
	switch ev.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		ev.SetInt(id)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		ev.SetUint(uint64(id)) // #nosec G115 -- an auto-increment key is positive
	}
}

// identityIsInteger reports whether the identity field has an integer kind,
// the only kind assignGeneratedKey can store a LAST_INSERT_ID in.
func identityIsInteger[T any](meta *Meta[T]) bool {
	var z T
	p := meta.Ptr(&z, meta.Identity)
	if p == nil {
		return false
	}
	rv := reflect.ValueOf(p)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return false
	}
	switch rv.Elem().Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	}
	return false
}

// resolveMySQLIdentity makes a multi-row model insert on MySQL write the
// identity column when every row carries an explicit value, so the rows keep
// the keys the caller chose. Other inserts are returned unchanged.
func (q InsertQuery[T]) resolveMySQLIdentity() InsertQuery[T] {
	id := q.meta.Identity
	if q.q == nil || !q.q.Dialect().OnDuplicateKey || len(q.models) < 2 || id < 0 || id >= len(q.meta.Columns) ||
		q.meta.Columns[id].Flags&FlagIdentity == 0 || slices.Contains(q.i.omit, q.meta.Columns[id].Name) {
		return q
	}
	for r := range q.models {
		if reflect.ValueOf(q.meta.Value(&q.models[r], id)).IsZero() {
			return q
		}
	}
	q.i.withIdentity = true
	return q
}
