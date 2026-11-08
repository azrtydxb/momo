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
// doNothing it writes the no-op assignment firstKey = firstKey, which is how
// MySQL skips a conflicting row. value renders the right-hand side of one
// assignment.
func renderMySQLUpsert(b *Builder, flavor dialect.Flavor, firstKey string, sets []Assignment, doNothing, fromValues bool, value func(*Builder, Assignment)) {
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
		b.Ident(firstKey)
		b.WriteString(" = ")
		b.Ident(firstKey)
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

// checkMySQLConflictTarget returns ErrUnsupported unless target is the only
// unique key of the table, the primary key included. MySQL's
// ON DUPLICATE KEY UPDATE fires for a conflict on any unique key and cannot
// name one, so with a second unique key the statement would also update the
// row that conflicts on the key the caller did not name. With no schema (a
// hand-written Meta) the unique keys are unknown: only a target equal to pk is
// accepted, and the caller owns the claim that the table has no other unique
// key.
func checkMySQLConflictTarget(ts *schema.Table, target []string, pk []string) error {
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
			cols = append(cols, p.Column)
		}
		uniques = append(uniques, cols)
	}
	if len(uniques) == 1 && sameStrings(target, uniques[0]) {
		return nil
	}
	return mysqlTargetError(target, fmt.Sprintf("the table has %d unique keys and the target must be the only one", len(uniques)))
}

func mysqlTargetError(target []string, why string) error {
	return &Error{Kind: ErrUnsupported, Op: "insert", Err: fmt.Errorf(
		"MySQL ON DUPLICATE KEY UPDATE cannot target %v: it fires for any unique key; %s", target, why)}
}

// renderMySQLConflict renders the conflict clause of an INSERT for the MySQL
// dialect as ON DUPLICATE KEY UPDATE.
func (q InsertQuery[T]) renderMySQLConflict(b *Builder) {
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
	if err := checkMySQLConflictTarget(q.meta.TableSchema, q.i.target, pk); err != nil {
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
		renderMySQLUpsert(b, b.d.Flavor, key, nil, true, fromValues, q.appendValueTo)
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
	renderMySQLUpsert(b, b.d.Flavor, "", q.i.set, false, fromValues, q.appendValueTo)
}

// appendValueTo adapts appendValue to the callback form.
func (q InsertQuery[T]) appendValueTo(b *Builder, a Assignment) { q.appendValue(b, a) }

// wantsGeneratedKey reports whether execOne should read LAST_INSERT_ID and
// write it back: a single Model insert on MySQL or MariaDB without a conflict
// clause, whose identity column the statement leaves to the server.
func (q InsertQuery[T]) wantsGeneratedKey() bool {
	id := q.meta.Identity
	return q.ptr != nil && len(q.models) == 1 && q.q != nil && q.q.Dialect().OnDuplicateKey &&
		!q.i.upsert && q.i.conflict == conflictNone && id >= 0 && id < len(q.meta.Columns) &&
		q.meta.Ptr != nil && q.meta.Columns[id].Flags&FlagIdentity != 0
}

// assignGeneratedKey stores id in the identity field of the model passed to
// Model. Field types other than the integer kinds are left alone. Only the
// single Model path writes a key back: a multi-row insert has no reliable ids
// (they depend on innodb_autoinc_lock_mode), so it neither returns nor writes
// them. MariaDB callers who need them use Returning, which renders
// INSERT ... RETURNING; MySQL has no equivalent and Returning is
// ErrUnsupported there.
func (q InsertQuery[T]) assignGeneratedKey(id int64) {
	p := q.meta.Ptr(q.ptr, q.meta.Identity)
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
