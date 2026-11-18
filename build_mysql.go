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

func mysqlTargetError(target []string, why string) error {
	return &Error{Kind: ErrUnsupported, Op: "insert", Err: fmt.Errorf(
		"MySQL ON DUPLICATE KEY UPDATE cannot target %v: it fires for any unique key; %s", target, why)}
}

// wantsGeneratedKey reports whether execOne should read LAST_INSERT_ID and
// write it back: a single Model insert on MySQL or MariaDB without a conflict
// clause, whose identity column the statement leaves to the server. a.hasBack
// tells whether the caller's model is there to receive it.
func (r insertRef) wantsGeneratedKey(a insAccess) bool {
	return r.nBack == 1 && a.hasBack(0) && r.n == 1 && r.leavesKeyToServer()
}

// leavesKeyToServer reports whether the statement is a plain INSERT on MySQL
// or MariaDB whose identity column the server generates (no conflict clause,
// no Go-side id, not named in Explicit).
func (r insertRef) leavesKeyToServer() bool {
	id := r.m.Identity
	return r.q != nil && r.q.Dialect().OnDuplicateKey &&
		!r.i.upsert && r.i.conflict == conflictNone && !r.explicitCol(id) && id >= 0 && id < len(r.m.Columns) &&
		r.m.HasPtr && r.m.Columns[id].Flags&FlagIdentity != 0 && !r.isIDGenColumn(id)
}

// assignGeneratedKey stores id in the identity field p of the model passed to
// Model (see identityIsInteger for the field kinds). A multi-row insert through
// Exec returns and writes back no keys, because the ids of a multi-row
// statement depend on innodb_autoinc_lock_mode; CreateGraph inserts one row per
// statement on MySQL for that reason. MariaDB callers who need keys use
// Returning, which renders INSERT ... RETURNING; MySQL has no equivalent and
// Returning is ErrUnsupported there.
func assignGeneratedKey(p any, id int64) {
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
func (r insertRef) resolveMySQLIdentity(a insAccess) insertRef {
	id := r.m.Identity
	if r.q == nil || !r.q.Dialect().OnDuplicateKey || r.n < 2 || id < 0 || id >= len(r.m.Columns) ||
		r.m.Columns[id].Flags&FlagIdentity == 0 || slices.Contains(r.i.omit, r.m.Columns[id].Name) {
		return r
	}
	for row := range r.n {
		if reflect.ValueOf(a.val(row, id)).IsZero() {
			return r
		}
	}
	r.i.withIdentity = true
	return r
}
