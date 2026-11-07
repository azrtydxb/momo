// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"reflect"
	"slices"
)

// Upsert starts an INSERT ... ON CONFLICT DO UPDATE for model T. Unless the
// caller chooses otherwise with OnConflict, the conflict target is T's
// primary key, and DoUpdate (when not given, or given without assignments)
// assigns every written column that an update may change with
// SetExcluded. The primary key, version, soft-delete and created-at columns
// are never part of DO UPDATE SET, and an explicit assignment to one (or to
// updated-at) is refused with ErrValidation. When the model has an
// updated-at column, DO UPDATE SET always ends with updated_at =
// EXCLUDED.updated_at, also after an explicit DoUpdate list, so a conflict
// refreshes the timestamp just as Update does; created_at is never
// overwritten.
//
// The version column is neither advanced nor checked on conflict: an upsert
// overwrites the existing row without optimistic-lock detection. Explicit
// identity values inserted with OVERRIDING SYSTEM VALUE do not advance the
// PostgreSQL identity sequence, so later generated identities may collide
// with them; the caller must setval the sequence.
//
// When the conflict target is the primary key and every model carries a
// non-zero identity value, the identity column joins the INSERT column list
// (PostgreSQL renders OVERRIDING SYSTEM VALUE), so the statement can hit an
// existing row. A model whose identity is zero is inserted with a generated
// identity. Mixing both in one Models call is refused with ErrValidation.
// Dialects without ON CONFLICT (MySQL) fail with ErrUnsupported.
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
		return q, nil
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
			return q, q.verr(a.col, "DoUpdate: column %q is the %s column and cannot be set by an upsert", a.col, role)
		}
		if !columnAssignable(writeUpdate, f) {
			return q, q.verr(a.col, "DoUpdate: column %q cannot be written by UPDATE", a.col)
		}
	}
	if len(q.i.set) == 0 {
		q.i.set = q.defaultUpsertSet()
	}
	q.i.set = q.withUpdatedAt(q.i.set)
	return q, nil
}

// withUpdatedAt appends updated_at = EXCLUDED.updated_at to a DO UPDATE list
// so a conflict refreshes the timestamp the way Update always does. The
// inserted row's updated_at comes from the clock, so EXCLUDED carries now.
// It is skipped when the model has no updated-at column, the column is
// omitted from the INSERT, or no other assignment exists to extend.
func (q InsertQuery[T]) withUpdatedAt(set []Assignment) []Assignment {
	ci := q.meta.UpdatedAt
	if ci < 0 || ci >= len(q.meta.Columns) || !q.writable(ci) || len(set) == 0 {
		return set
	}
	name := q.meta.Columns[ci].Name
	if q.i.sel != nil && !slices.Contains(q.i.selCols, name) {
		return set // FromSelect writes only the listed columns
	}
	if slices.Contains(q.i.omit, name) {
		return set
	}
	for _, a := range set {
		if a.col == name {
			return set
		}
	}
	return append(set, Assignment{col: name, kind: setExcluded})
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
			if _, skip := omit[c.Name]; skip || !q.writable(i) || !upsertUpdatable(c.Flags) {
				continue
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
	if len(q.models) == 0 || id < 0 || id >= len(q.meta.Columns) {
		return nil
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
