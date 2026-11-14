// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"slices"
	"time"
)

// Upsert starts an INSERT ... ON CONFLICT DO UPDATE for model T. Unless the
// caller chooses otherwise with OnConflict, the conflict target is T's
// primary key, and DoUpdate (when not given, or given without assignments)
// assigns every written column that an update may change with
// SetExcluded. The primary key, version, soft-delete and created-at columns
// are never part of DO UPDATE SET, and an explicit assignment to one (or to
// updated-at) is refused with ErrValidation. When the model has an
// updated-at column, the conflict branch always sets updated_at to the
// statement clock, the value Update binds, as the last assignment of DO
// UPDATE SET (also after an explicit DoUpdate list). It is a bound value and
// not EXCLUDED.updated_at, so a struct that already carries an old UpdatedAt
// still refreshes the row (before, such an upsert wrote the old value back).
// The bump does not depend on the INSERT: Omit(updated_at) and a FromSelect
// without an updated_at column affect only the insert branch, and an upsert
// whose default set is empty renders a touch-only SET updated_at = <clock>.
// created_at is never overwritten. The same holds for a plain Insert with
// OnConflict and DoUpdate that has at least one assignment (DoUpdate() alone
// stays an error there). A DoNothing conflict does not touch the row, so it
// bumps nothing.
//
// The version column is neither advanced nor checked on conflict: an upsert
// overwrites the existing row without optimistic-lock detection. Explicit
// identity values inserted with OVERRIDING SYSTEM VALUE do not advance the
// PostgreSQL identity sequence, and explicit serial ids do not advance the
// serial sequence either, so later generated keys may collide with them; the
// caller must setval the sequence. FromSelect cannot supply serial ids at all.
// A serial or identity column that is not the conflict-target key is written
// by naming it in InsertQuery.Explicit, which also works on Upsert.
//
// When the conflict target is the primary key and every model carries a
// non-zero identity value, the identity column joins the INSERT column list
// (PostgreSQL renders OVERRIDING SYSTEM VALUE), so the statement can hit an
// existing row. A model whose identity is zero is inserted with a generated
// identity. Mixing both in one Models call is refused with ErrValidation.
// On MySQL and MariaDB the statement renders ON DUPLICATE KEY UPDATE, which
// cannot name a key, so the target must be a unique key of the table and no
// other unique key may be able to conflict: another key is allowed only when it
// is made of auto-increment columns the insert leaves out (an auto-increment
// primary key next to UNIQUE(email), the key left zero). Any other target
// fails with ErrUnsupported and the message names the reason; a dialect with
// neither form fails with ErrUnsupported.
func Upsert[T any, PT Modeler[T]](q Querier) InsertQuery[T] {
	i := Insert[T, PT](q)
	i.i.upsert = true
	return i
}

// upsertUpdatable reports whether DO UPDATE SET may assign the column.
func upsertUpdatable(f uint16) bool { return updModelWritable(f) }

// resolveUpsert fills the upsert defaults into a copy of q: the primary-key
// target, the default DO UPDATE list, the identity-in-INSERT decision. It
// validates an explicit DO UPDATE list against the update guard rules.
func (q InsertQuery[T]) resolveUpsert() (InsertQuery[T], error) {
	if !q.i.upsert {
		if q.i.conflict != conflictUpdate || q.i.bumped {
			return q.resolveMySQLIdentity(), nil
		}
		// A plain Insert ... OnConflict ... DoUpdate gets the same
		// updated_at rules as Upsert: refuse an explicit set, bump on conflict.
		if err := q.refuseUpdatedAtSet(); err != nil {
			return q, err
		}
		if len(q.i.set) > 0 { // DoUpdate() alone stays "needs at least one assignment"
			var err error
			if q, err = q.bumpUpdatedAt(); err != nil {
				return q, err
			}
		}
		q.i.bumped = true
		return q.resolveMySQLIdentity(), nil
	}
	if q.i.conflict == conflictNone {
		q.i.conflict = conflictUpdate
	}
	if len(q.i.target) == 0 && q.i.conflict == conflictUpdate {
		if len(q.meta.PK) == 0 {
			return q, q.verr("", "Upsert: %s has no primary key; pass OnConflict columns", q.meta.Table)
		}
		q.i.target = q.pkNames()
	}
	if err := q.resolveIdentity(); err != nil {
		return q, err
	}
	if q.i.conflict != conflictUpdate {
		return q, nil
	}
	for _, a := range q.i.set {
		ci := q.column(a.col)
		if ci < 0 {
			continue // renderConflict reports the unmapped column
		}
		f := q.meta.Columns[ci].Flags
		if role := q.reservedRoleOf(ci); role != "" {
			return q, q.verr(a.col, "DoUpdate: column %q is the %s column and cannot be set directly", a.col, role)
		}
		if !columnAssignable(writeUpdate, f) {
			return q, q.verr(a.col, "DoUpdate: column %q cannot be written by UPDATE", a.col)
		}
	}
	if len(q.i.set) == 0 {
		q.i.set = q.defaultUpsertSet()
	}
	var err error
	if q, err = q.bumpUpdatedAt(); err != nil {
		return q, err
	}
	q.i.bumped = true
	return q, nil
}

// refuseUpdatedAtSet refuses an explicit DO UPDATE assignment to the
// updated-at column, with the error Update gives for the same Set.
func (q InsertQuery[T]) refuseUpdatedAtSet() error {
	ci := q.meta.UpdatedAt
	if ci < 0 || ci >= len(q.meta.Columns) {
		return nil
	}
	for _, a := range q.i.set {
		if a.col == q.meta.Columns[ci].Name {
			return q.verr(a.col, "DoUpdate: column %q is the updated-at column and cannot be set directly", a.col)
		}
	}
	return nil
}

// bumpUpdatedAt appends updated_at = <clock> to the DO UPDATE list, the value
// Update binds, so the conflict branch always refreshes the timestamp whatever
// the INSERT carries: the struct's own (possibly old) UpdatedAt, an Omit, or a
// FromSelect without the column. It reads the clock once into q.clock, which
// the INSERT and every chunk then share. It does nothing when the model has
// no updated-at column or the column is not assignable on UPDATE.
func (q InsertQuery[T]) bumpUpdatedAt() (InsertQuery[T], error) {
	ci := q.meta.UpdatedAt
	if ci < 0 || ci >= len(q.meta.Columns) || !columnAssignable(writeUpdate, q.meta.Columns[ci].Flags) {
		return q, nil
	}
	c := q.meta.Columns[ci]
	if err := checkTimestampField(q.meta, ci); err != nil {
		return q, q.verr(c.Name, "%v", err)
	}
	if q.clock.IsZero() {
		if q.q != nil {
			q.clock = q.q.conn().now()
		} else {
			q.clock = time.Now()
		}
	}
	v, ok := autoTimestamp(q.meta, ci, q.clock)
	if !ok {
		return q, nil
	}
	q.i.set = append(slices.Clone(q.i.set), Assignment{col: c.Name, kind: setValue, val: v, flags: c.Flags})
	return q, nil
}

// pkNames returns the primary-key column names.
func (q InsertQuery[T]) pkNames() []string {
	names := make([]string, len(q.meta.PK))
	for i, ci := range q.meta.PK {
		names[i] = q.meta.Columns[ci].Name
	}
	return names
}

// reservedRoleOf names the role of column ci when it is the primary key,
// version, soft-delete, created-at or updated-at column.
func (q InsertQuery[T]) reservedRoleOf(ci int) string {
	switch {
	case slices.Contains(q.meta.PK, ci):
		return "primary key"
	case ci == q.meta.Version:
		return "version"
	case ci == q.meta.SoftDelete:
		return "soft-delete"
	case ci == q.meta.CreatedAt:
		return "created-at"
	case ci == q.meta.UpdatedAt:
		return "updated-at"
	}
	return ""
}

// defaultUpsertSet assigns EXCLUDED to every column of the INSERT list that
// an upsert may update.
func (q InsertQuery[T]) defaultUpsertSet() []Assignment {
	var out []Assignment
	if q.i.sel != nil {
		for _, n := range q.i.selCols {
			if ci := q.column(n); ci >= 0 && upsertUpdatable(q.meta.Columns[ci].Flags) {
				out = append(out, Assignment{col: n, kind: setExcluded})
			}
		}
		return out
	}
	if len(q.models) > 0 {
		omit := map[string]struct{}{}
		for _, n := range q.i.omit {
			omit[n] = struct{}{}
		}
		for i, c := range q.meta.Columns {
			if _, skip := omit[c.Name]; skip || !q.writable(i) || !upsertUpdatable(c.Flags) || q.isNonPKIDGen(i) || q.dbDefaultCount(i) > 0 {
				continue // a generated id or database default is never overwritten by a fresh one
			}
			out = append(out, Assignment{col: c.Name, kind: setExcluded})
		}
		return out
	}
	src := q.i.rows
	if !q.i.hasRows {
		src = [][]Assignment{q.i.assigns}
	}
	seen := map[string]struct{}{}
	for _, row := range src {
		for _, a := range row {
			ci := q.column(a.col)
			if _, dup := seen[a.col]; dup || ci < 0 || a.kind == setDefault || !upsertUpdatable(q.meta.Columns[ci].Flags) {
				continue
			}
			seen[a.col] = struct{}{}
			out = append(out, Assignment{col: a.col, kind: setExcluded})
		}
	}
	return out
}

// resolveIdentity decides whether the identity column joins the INSERT list.
func (q *InsertQuery[T]) resolveIdentity() error {
	id := q.meta.Identity
	if len(q.models) == 0 || id < 0 || id >= len(q.meta.Columns) || q.explicitCol(id) {
		return nil // Explicit already decides: the value is written as-is
	}
	if q.meta.Columns[id].Flags&FlagReadOnly != 0 || !slices.Contains(q.meta.PK, id) {
		return nil
	}
	if !sameStrings(q.i.target, q.pkNames()) || slices.Contains(q.i.omit, q.meta.Columns[id].Name) {
		return nil
	}
	set := 0
	for r := range q.models {
		if !reflect.ValueOf(q.meta.Value(&q.models[r], id)).IsZero() {
			set++
		}
	}
	switch set {
	case 0:
		return nil
	case len(q.models):
		q.i.withIdentity = true
		return nil
	}
	return q.verr(q.meta.Columns[id].Name, "Upsert: models mix explicit and zero identity values; split them into separate calls")
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as, bs := slices.Clone(a), slices.Clone(b)
	slices.Sort(as)
	slices.Sort(bs)
	return slices.Equal(as, bs)
}

// insertsIdentity reports whether column i is the identity column and an
// upsert writes its explicit values.
func (q InsertQuery[T]) insertsIdentity(i int) bool {
	return q.i.withIdentity && i == q.meta.Identity
}
