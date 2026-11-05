// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"
	"slices"
)

// FieldHook attaches Go-side behavior to one column of Meta[T].
//
// Hooks are declared in Meta.Fields (generated or hand-written) or per DB
// with WithFieldHooks. Write builders run them before the statement is built,
// in this order: Default (insert), OnUpdate (update), Validate. A failing
// validator makes SQL, Exec and Rows return an *Error of kind ErrValidation
// before anything reaches the database. Hooks for a column that the statement
// cannot write (identity, read-only, or, for OnUpdate, create-only and
// immutable columns) are ignored on insert and refused on update for the
// reserved key, version, soft-delete and timestamp columns.
//
// A Default or OnUpdate function may run more than once per row (the bulk
// planner validates every row before chunking), so it must be cheap and free
// of side effects; a fresh random value per call is fine.
type FieldHook[T any] struct {
	// Column is the index into Meta.Columns.
	Column int
	// Default supplies the value of the column when a write omits it: a
	// Values or ValuesRows row that does not assign the column (an explicit
	// SetDefault, SetNull or zero value counts as assigned and wins), or a
	// Model insert that lists the column in Omit. It never replaces a zero
	// value, because a Model insert writes every column it does not omit. For
	// Values and ValuesRows rows there is no model, so m points to a zero T.
	Default func(m *T) any
	// OnUpdate supplies the value of the column on every update statement
	// that does not assign the column itself with Set; an explicit Set wins.
	// For a Model update m is the model, otherwise a zero T. It overrides the
	// value Model would write from the struct.
	OnUpdate func(m *T) any
	// Validate checks the value the statement will write for the column and
	// runs before SQL is built. v is the Go value bound for the column; it is
	// nil when the statement writes NULL or DEFAULT, or (insert only) omits
	// the column without a Default, so a required-field validator should
	// reject nil. A SQL expression assignment is not validated, and an
	// update validates only the columns it sets. A non-nil error becomes
	// *Error{Kind: ErrValidation, Op, Table, Column} wrapping it.
	Validate func(v any) error
}

// WithFieldHooks registers Go-side defaults and validators for model T on
// this DB. They apply in addition to T's Meta.Fields, after them, and are
// looked up by the Meta pointer, so no reflection or tag inspection runs per
// query. Several calls for the same T accumulate.
func WithFieldHooks[T any, PT Modeler[T]](hooks []FieldHook[T]) Option {
	return func(db *DB) {
		m := metaOf[T, PT]()
		if db.fieldHooks == nil {
			db.fieldHooks = map[any]any{}
		}
		prev, _ := db.fieldHooks[m].([]FieldHook[T])
		db.fieldHooks[m] = append(slices.Clip(prev), hooks...)
	}
}

// fieldPlan is the resolved hook set of one statement.
type fieldPlan[T any] struct {
	hooks []FieldHook[T]
	// def and upd map a column index to its Default or OnUpdate function; the
	// last hook declared for a column wins.
	def, upd map[int]func(*T) any
}

// newFieldPlan collects Meta.Fields and the DB's hooks for m and checks the
// column indexes.
func newFieldPlan[T any](q Querier, m *Meta[T]) (fieldPlan[T], error) {
	hooks := m.Fields
	if q != nil {
		if e := q.conn(); e != nil && e.db != nil {
			if extra, ok := e.db.fieldHooks[m].([]FieldHook[T]); ok {
				hooks = append(slices.Clone(hooks), extra...)
			}
		}
	}
	fp := fieldPlan[T]{hooks: hooks}
	for _, h := range hooks {
		if h.Column < 0 || h.Column >= len(m.Columns) {
			return fieldPlan[T]{}, &Error{Kind: ErrValidation, Op: "fields", Table: m.Table,
				Err: fmt.Errorf("field hook column index %d is outside the %d columns of %s", h.Column, len(m.Columns), m.Table)}
		}
		if h.Default != nil {
			if fp.def == nil {
				fp.def = map[int]func(*T) any{}
			}
			fp.def[h.Column] = h.Default
		}
		if h.OnUpdate != nil {
			if fp.upd == nil {
				fp.upd = map[int]func(*T) any{}
			}
			fp.upd[h.Column] = h.OnUpdate
		}
	}
	return fp, nil
}

// validate runs every Validate hook. value reports the value the statement
// writes for column ci; ok is false when the validator must be skipped.
func (fp fieldPlan[T]) validate(op string, m *Meta[T], value func(ci int) (v any, ok bool)) error {
	for _, h := range fp.hooks {
		if h.Validate == nil {
			continue
		}
		v, ok := value(h.Column)
		if !ok {
			continue
		}
		if err := h.Validate(v); err != nil {
			return fieldError(op, m.Table, m.Columns[h.Column].Name, err)
		}
	}
	return nil
}

// fieldError wraps a validator error as an ErrValidation *Error.
func fieldError(op, table, col string, err error) error {
	if err == nil {
		err = errors.New("invalid value")
	}
	return &Error{Kind: ErrValidation, Op: op, Table: table, Column: col, Err: err}
}

// assignedValue is the Go value an assignment binds: the value itself, nil for
// NULL and DEFAULT. ok is false for a SQL expression or EXCLUDED reference,
// which cannot be checked in Go.
func assignedValue(a Assignment) (any, bool) {
	switch a.kind {
	case setValue:
		return a.val, true
	case setNull, setDefault:
		return nil, true
	}
	return nil, false
}
