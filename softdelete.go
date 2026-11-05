// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"time"
)

// SoftDeleteKind is the type of a model's soft-delete marker column. The
// zero value is the nullable timestamp, so metadata that predates the field
// keeps working. Generated code records the kind from the field's Go type:
// *time.Time, *int64 or bool.
type SoftDeleteKind uint8

const (
	// SoftDeleteTimestamp is a nullable timestamp, set to the statement time.
	SoftDeleteTimestamp SoftDeleteKind = iota
	// SoftDeleteUnix is a nullable integer, set to the statement time in Unix seconds.
	SoftDeleteUnix
	// SoftDeleteBool is a boolean, set to true.
	SoftDeleteBool
)

// softMarkerPred is the predicate on a soft-delete marker column: the row is
// live (deleted false) or soft-deleted (deleted true).
type softMarkerPred struct {
	table, col string
	kind       SoftDeleteKind
	deleted    bool
}

func (p softMarkerPred) AppendSQL(b *Builder) {
	b.QualifiedIdent(p.table, p.col)
	switch {
	case p.kind == SoftDeleteBool && p.deleted:
		b.WriteString(" = TRUE")
	case p.kind == SoftDeleteBool:
		b.WriteString(" = FALSE")
	case p.deleted:
		b.WriteString(" IS NOT NULL")
	default:
		b.WriteString(" IS NULL")
	}
}
func (softMarkerPred) resultType() bool { return false }

// softDeletePred returns the soft-delete predicate of m for rows under the
// given qualifier.
func softDeletePred[T any](m *Meta[T], qualifier string, deleted bool) Predicate {
	return softMarkerPred{qualifier, m.Columns[m.SoftDelete].Name, m.SoftDeleteKind, deleted}
}

// softDeleteValue returns the value the marker is set to at now.
func softDeleteValue(kind SoftDeleteKind, now time.Time) any {
	switch kind {
	case SoftDeleteUnix:
		return now.Unix()
	case SoftDeleteBool:
		return true
	default:
		return now.UTC()
	}
}

// SoftDelete returns an UpdateQuery that marks rows deleted instead of
// removing them. The marker follows the model's soft-delete column: a
// nullable timestamp (set to the statement time from the DB clock, UTC), a
// Unix integer (seconds) or a boolean (set true). Rows already soft-deleted
// are not touched.
//
// It keeps the mutation guard: without a Where (or Model) that restricts the
// rows it fails with ErrUnfilteredMutation unless AllRows confirms it. Set is
// refused, and a model without a soft-delete column fails with ErrValidation.
// Model identifies the row by primary key only; no column is written from it.
func SoftDelete[T any, PT Modeler[T]](q Querier) UpdateQuery[T] {
	u := Update[T, PT](q)
	u.softDelete = true
	return u
}

// planSoftDelete is plan for a SoftDelete statement.
func (u UpdateQuery[T]) planSoftDelete() ([]Assignment, []Predicate, error) {
	m := u.meta
	if m.SoftDelete < 0 || m.SoftDelete >= len(m.Columns) {
		return nil, nil, u.fail(ErrValidation, errors.New("model has no soft-delete column"))
	}
	if len(u.assigns) > 0 {
		return nil, nil, u.fail(ErrValidation, errors.New("SoftDelete does not take Set; it only writes the soft-delete column"))
	}
	if u.modelSet && u.model == nil {
		return nil, nil, u.fail(ErrValidation, errors.New("Model(nil)"))
	}
	where := append([]Predicate(nil), u.where...)
	if u.model != nil {
		pks, err := u.pkPredicates(u.model)
		if err != nil {
			return nil, nil, err
		}
		where = append(pks, where...)
	}
	if !hasEffectiveFilter(where) && !u.allRows {
		return nil, nil, u.fail(ErrUnfilteredMutation, nil)
	}
	// The live-row filter is added after the guard: it is not a caller filter.
	where = append(where, softDeletePred(m, m.Table, false))
	sets := []Assignment{{col: m.Columns[m.SoftDelete].Name, kind: setValue, val: softDeleteValue(m.SoftDeleteKind, u.q.conn().now())}}
	return sets, where, nil
}

// ForceDelete is the hard delete for soft-deleting models. It removes rows
// whether or not they are soft-deleted, and bypasses the refusal a plain
// Delete gives on such models. The mutation guard still applies.
func ForceDelete[T any, PT Modeler[T]](q Querier) DeleteQuery[T] {
	d := Delete[T, PT](q)
	d.force = true
	return d
}

// OnlyDeleted narrows a select to soft-deleted rows of a model with a
// soft-delete column. A later WithDeleted overrides it.
func (sq SelectQuery[T]) OnlyDeleted() SelectQuery[T] {
	sq.s.onlyDeleted, sq.s.withDeleted = true, false
	return sq
}
