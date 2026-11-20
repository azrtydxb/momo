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
func upsertUpdatable(f ColumnFlags) bool { return updModelWritable(f) }

// resolveUpsert fills the upsert defaults into a copy of r: the primary-key
// target, the default DO UPDATE list, the identity-in-INSERT decision. It
// validates an explicit DO UPDATE list against the update guard rules.
func (r insertRef) resolveUpsert(a insAccess) (insertRef, error) {
	if !r.i.upsert {
		if r.i.conflict != conflictUpdate || r.i.bumped {
			return r.resolveMySQLIdentity(a), nil
		}
		// A plain Insert ... OnConflict ... DoUpdate gets the same
		// updated_at rules as Upsert: refuse an explicit set, bump on conflict.
		if err := r.refuseUpdatedAtSet(); err != nil {
			return r, err
		}
		if len(r.i.set) > 0 { // DoUpdate() alone stays "needs at least one assignment"
			var err error
			if r, err = r.bumpUpdatedAt(a); err != nil {
				return r, err
			}
		}
		r.i.bumped = true
		return r.resolveMySQLIdentity(a), nil
	}
	if r.i.conflict == conflictNone {
		r.i.conflict = conflictUpdate
	}
	if len(r.i.target) == 0 && r.i.conflict == conflictUpdate {
		if len(r.m.PK) == 0 {
			return r, r.verr("", "Upsert: %s has no primary key; pass OnConflict columns", r.m.Table)
		}
		r.i.target = r.pkNames()
	}
	withIdentity, err := r.resolveIdentity(a)
	if err != nil {
		return r, err
	}
	if withIdentity {
		r.i.withIdentity = true
	}
	if r.i.conflict != conflictUpdate {
		return r, nil
	}
	for _, as := range r.i.set {
		ci := r.column(as.col)
		if ci < 0 {
			continue // renderConflict reports the unmapped column
		}
		f := r.m.Columns[ci].Flags
		if role := r.reservedRoleOf(ci); role != "" {
			return r, r.verr(as.col, "DoUpdate: column %q is the %s column and cannot be set directly", as.col, role)
		}
		if !columnAssignable(writeUpdate, f) {
			return r, r.verr(as.col, "DoUpdate: column %q cannot be written by UPDATE", as.col)
		}
	}
	if len(r.i.set) == 0 {
		r.i.set = r.upsertSet(a)
	}
	if r, err = r.bumpUpdatedAt(a); err != nil {
		return r, err
	}
	r.i.bumped = true
	return r, nil
}

// bumpUpdatedAt appends updated_at = <clock> to the DO UPDATE list, the value
// Update binds, so the conflict branch always refreshes the timestamp whatever
// the INSERT carries: the struct's own (possibly old) UpdatedAt, an Omit, or a
// FromSelect without the column. It reads the clock once into r.clock, which
// the INSERT and every chunk then share. It does nothing when the model has
// no updated-at column or the column is not assignable on UPDATE.
func (r insertRef) bumpUpdatedAt(a insAccess) (insertRef, error) {
	ci := r.m.UpdatedAt
	if ci < 0 || ci >= len(r.m.Columns) || !columnAssignable(writeUpdate, r.m.Columns[ci].Flags) {
		return r, nil
	}
	c := r.m.Columns[ci]
	if err := r.checkTimestamp(ci, a); err != nil {
		return r, r.verr(c.Name, "%v", err)
	}
	if r.clock.IsZero() {
		if r.q != nil {
			r.clock = r.q.conn().now()
		} else {
			r.clock = time.Now()
		}
	}
	v, ok := r.autoTimestamp(ci, r.clock, a)
	if !ok {
		return r, nil
	}
	r.i.set = append(slices.Clone(r.i.set), Assignment{col: c.Name, kind: setValue, val: v, flags: c.Flags})
	return r, nil
}

// upsertSet assigns EXCLUDED to every column of the INSERT list that an upsert
// may update. It needs the dbdefault plan, so it computes it.
func (r insertRef) upsertSet(a insAccess) []Assignment {
	var counts []int
	if r.i.sel == nil && r.n > 0 {
		counts = r.withDBD(a).dbd.count
	}
	return r.defaultUpsertSet(r.n, counts)
}

// resolveIdentity decides whether the identity column joins the INSERT list.
func (r insertRef) resolveIdentity(a insAccess) (withIdentity bool, err error) {
	id := r.m.Identity
	if r.n == 0 || id < 0 || id >= len(r.m.Columns) || r.explicitCol(id) {
		return false, nil // Explicit already decides: the value is written as-is
	}
	if r.m.Columns[id].Flags&FlagReadOnly != 0 || !slices.Contains(r.m.PK, id) {
		return false, nil
	}
	if !sameStrings(r.i.target, r.pkNames()) || slices.Contains(r.i.omit, r.m.Columns[id].Name) {
		return false, nil
	}
	set := 0
	for row := range r.n {
		if !reflect.ValueOf(a.val(row, id)).IsZero() {
			set++
		}
	}
	switch set {
	case 0:
		return false, nil
	case r.n:
		return true, nil
	}
	return false, r.verr(r.m.Columns[id].Name, "Upsert: models mix explicit and zero identity values; split them into separate calls")
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
