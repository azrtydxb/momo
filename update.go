// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// UpdateQuery builds an UPDATE of one model table. It is a value: every
// method returns a copy, so a base query can be reused safely.
//
// Column identifiers come only from typed column descriptors and the
// model's MomoMeta, are always quoted, and every value is bound. A statement
// without a WHERE clause is refused with ErrUnfilteredMutation unless
// AllRows confirms it.
type UpdateQuery[T any] struct {
	q       Querier
	meta    *Meta[T]
	assigns []Assignment
	where   []Predicate
	model   *T
	// modelSet records that Model was called, so Model(nil) is an error and
	// not an unfiltered statement.
	modelSet bool
	allRows  bool
	// returning is the RETURNING list, read with Rows.
	returning returningSet
	// count counts the rows of the target table matching ps, soft-deleted
	// rows included. It is bound at Update, where the Modeler is known.
	count func(ctx context.Context, q Querier, ps []Predicate) (int64, error)
}

// Update starts an UPDATE of T's table.
func Update[T any, PT Modeler[T]](q Querier) UpdateQuery[T] {
	return UpdateQuery[T]{q: q, meta: metaOf[T, PT](), count: func(ctx context.Context, q Querier, ps []Predicate) (int64, error) {
		return Select[T, PT](q).Where(ps...).WithDeleted().Count(ctx)
	}}
}

// Set adds explicit column assignments. An assignment to a column that is
// read-only, identity, create-only or immutable in the model's metadata is
// refused with ErrValidation when the statement is built. A later assignment
// to the same column replaces an earlier one.
func (u UpdateQuery[T]) Set(assigns ...Assignment) UpdateQuery[T] {
	u.assigns = append(slices.Clip(u.assigns), assigns...)
	return u
}

// Where adds predicates combined with AND.
func (u UpdateQuery[T]) Where(ps ...Predicate) UpdateQuery[T] {
	u.where = append(slices.Clip(u.where), ps...)
	return u
}

// Model updates the row identified by m's primary key with m's writable
// column values. When the model has a version column the statement is
// version-checked (WHERE pk = ? AND version = ?, SET version = version + 1):
// a stale m fails with ErrOptimisticLock and a missing row with ErrNotFound.
// After a successful versioned update m's version field is advanced. The
// identity, read-only, create-only, immutable, soft-delete, created-at and
// updated-at columns are not written from m; use Set for explicit writes.
func (u UpdateQuery[T]) Model(m *T) UpdateQuery[T] { u.model, u.modelSet = m, true; return u }

// AllRows confirms that an UPDATE without WHERE is intended.
func (u UpdateQuery[T]) AllRows() UpdateQuery[T] { u.allRows = true; return u }

// Returning appends a RETURNING list; read the rows with Rows.
func (u UpdateQuery[T]) Returning(cols ...ColumnRef) UpdateQuery[T] {
	u.returning = u.returning.with(cols...)
	return u
}

// ReturningAll returns every column mapped by T, in metadata order.
func (u UpdateQuery[T]) ReturningAll() UpdateQuery[T] {
	u.returning = u.returning.withAll()
	return u
}

func (u UpdateQuery[T]) fail(kind error, err error) error {
	return &Error{Kind: kind, Op: "update", Table: u.meta.Table, Err: err}
}

// updModelWritable reports whether Model writes the column from the struct.
func updModelWritable(f uint16) bool {
	return columnAssignable(writeUpdate, f) && f&(FlagPK|FlagVersion|FlagSoftDelete|FlagCreatedAt|FlagUpdatedAt) == 0
}

// updPKEq is the predicate "qualifier.col = ?" with a bound value.
type updPKEq struct {
	table, col string
	val        any
}

func (p updPKEq) AppendSQL(b *Builder) {
	b.QualifiedIdent(p.table, p.col)
	b.WriteString(" = ")
	b.Arg(p.val)
}
func (updPKEq) resultType() bool { return false }

// pkPredicates returns the primary-key equality predicates for m.
func (u UpdateQuery[T]) pkPredicates(m *T) ([]Predicate, error) {
	if len(u.meta.PK) == 0 {
		return nil, u.fail(ErrValidation, errors.New("model has no primary key"))
	}
	ps := make([]Predicate, 0, len(u.meta.PK)+1)
	for _, i := range u.meta.PK {
		ps = append(ps, updPKEq{u.meta.Table, u.meta.Columns[i].Name, u.meta.Value(m, i)})
	}
	return ps, nil
}

// versioned reports whether the query is a version-checked Model update.
func (u UpdateQuery[T]) versioned() bool {
	return u.model != nil && u.meta.Version >= 0
}

// plan resolves the SET assignments and WHERE predicates, applying the
// guardrails. It does not render.
func (u UpdateQuery[T]) plan() ([]Assignment, []Predicate, error) {
	if u.meta.Table == "" {
		return nil, nil, u.fail(ErrValidation, errors.New("model has no table"))
	}
	var sets []Assignment
	where := slices.Clone(u.where)
	if u.modelSet && u.model == nil {
		return nil, nil, u.fail(ErrValidation, errors.New("Model(nil)"))
	}
	if u.model != nil {
		if u.versioned() {
			if err := u.checkVersionKind(); err != nil {
				return nil, nil, err
			}
		}
		for i, c := range u.meta.Columns {
			if !updModelWritable(c.Flags) {
				continue
			}
			sets = append(sets, Assignment{col: c.Name, kind: setValue, val: u.meta.Value(u.model, i)})
		}
		pks, err := u.pkPredicates(u.model)
		if err != nil {
			return nil, nil, err
		}
		where = append(pks, where...)
		if u.versioned() {
			v := u.meta.Version
			where = append(where, updPKEq{u.meta.Table, u.meta.Columns[v].Name, u.meta.Value(u.model, v)})
		}
	}
	for _, a := range u.assigns {
		if role := u.reservedRole(a.col); role != "" {
			// Tasks 18 (soft delete) and 20 (updated-at) own these columns,
			// and Model owns the key and version; a plain Set must not
			// bypass them.
			return nil, nil, u.fail(ErrValidation, fmt.Errorf("column %q is the %s column and cannot be set directly", a.col, role))
		}
		for _, c := range u.meta.Columns {
			if c.Name == a.col && !columnAssignable(writeUpdate, c.Flags) {
				return nil, nil, u.fail(ErrValidation, fmt.Errorf("column %q cannot be written by UPDATE", c.Name))
			}
		}
		if j := slices.IndexFunc(sets, func(s Assignment) bool { return s.col == a.col }); j >= 0 {
			sets[j] = a
		} else {
			sets = append(sets, a)
		}
	}
	if len(sets) == 0 && !u.versioned() {
		return nil, nil, u.fail(ErrValidation, errors.New("no columns to update"))
	}
	if !hasEffectiveFilter(where) && !u.allRows {
		return nil, nil, u.fail(ErrUnfilteredMutation, nil)
	}
	return sets, where, nil
}

// SQL renders the statement without executing it. Guardrail violations are
// reported here, before anything can reach the database.
func (u UpdateQuery[T]) SQL() (Compiled, error) {
	if u.q == nil {
		return Compiled{}, errors.New("momo: query has no Querier")
	}
	sets, where, err := u.plan()
	if err != nil {
		return Compiled{}, err
	}
	target := TableRef{Name: u.meta.Table, Schema: u.meta.Schema}
	return compile(u.q.Dialect(), func(b *Builder) {
		// The target table is the statement's scope, so a correlated
		// subquery in SET or WHERE may reference it.
		b.withScope([]string{target.Qualifier()}, func() {
			b.WriteString("UPDATE ")
			target.appendFrom(b)
			b.WriteString(" SET ")
			for i, a := range sets {
				if i > 0 {
					b.WriteString(", ")
				}
				u.appendAssignment(b, a)
			}
			if u.versioned() {
				vc := u.meta.Columns[u.meta.Version].Name
				if len(sets) > 0 {
					b.WriteString(", ")
				}
				b.Ident(vc)
				b.WriteString(" = ")
				b.QualifiedIdent(target.Qualifier(), vc)
				b.WriteString(" + 1")
			}
			if len(where) > 0 {
				b.WriteString(" WHERE ")
				appendAnd(b, where)
			}
		})
		renderReturning(b, u.meta, u.returning, func(col string) error {
			return u.fail(ErrValidation, fmt.Errorf("Returning: column %q is not mapped by %s", col, u.meta.Table))
		})
	})
}

func (u UpdateQuery[T]) appendAssignment(b *Builder, a Assignment) {
	b.Ident(a.col)
	b.WriteString(" = ")
	switch a.kind {
	case setValue:
		b.Arg(a.val)
	case setExpr:
		a.expr.AppendSQL(b)
	case setNull:
		b.WriteString("NULL")
	case setDefault:
		if !b.Dialect().DefaultInUpdate {
			b.unsupported("SET column = DEFAULT")
			return
		}
		b.WriteString("DEFAULT")
	default:
		b.SetErr(u.fail(ErrValidation, fmt.Errorf("column %q: SetExcluded is only valid in an upsert", a.col)))
	}
}

// Exec runs the UPDATE. A version-checked Model update that matches no row
// looks the row up inside the same transaction (one is started when q is a
// *DB) to tell ErrOptimisticLock from ErrNotFound.
func (u UpdateQuery[T]) Exec(ctx context.Context) (Result, error) {
	c, err := u.SQL()
	if err != nil {
		return Result{}, err
	}
	if !u.versioned() {
		n, err := u.q.conn().exec(ctx, "update", u.meta.Table, c)
		if err != nil {
			return Result{}, err
		}
		return Result{RowsAffected: n}, nil
	}
	var res Result
	run := func(q Querier) error {
		n, err := q.conn().exec(ctx, "update", u.meta.Table, c)
		if err != nil {
			return err
		}
		if n == 0 {
			return u.zeroRows(ctx, q)
		}
		res.RowsAffected = n
		return nil
	}
	if db, ok := u.q.(*DB); ok {
		err = db.Tx(ctx, TxOptions{}, func(tx *Tx) error { return run(tx) })
	} else {
		err = run(u.q)
	}
	if err != nil {
		return Result{}, err
	}
	u.advanceVersion()
	return res, nil
}

// Rows runs the UPDATE and returns its RETURNING rows. It fails with
// ErrValidation when no Returning list was set, and for a version-checked
// Model update, whose stale-row classification needs the affected-row count
// (use Exec for that).
func (u UpdateQuery[T]) Rows(ctx context.Context) (*ModelRows[T], error) {
	if !u.returning.active() {
		return nil, u.fail(ErrValidation, errors.New("Rows needs a Returning list"))
	}
	if u.versioned() {
		return nil, u.fail(ErrValidation, errors.New("Rows is not supported with a version-checked Model update; use Exec"))
	}
	c, err := u.SQL()
	if err != nil {
		return nil, err
	}
	return runReturning(ctx, u.q, "update", u.meta, u.returning, c)
}

// zeroRows classifies a versioned update that affected no row.
func (u UpdateQuery[T]) zeroRows(ctx context.Context, q Querier) error {
	pks, err := u.pkPredicates(u.model)
	if err != nil {
		return err
	}
	n, err := u.count(ctx, q, pks)
	if err != nil {
		return err
	}
	if n == 0 {
		return u.fail(ErrNotFound, nil)
	}
	return u.fail(ErrOptimisticLock, nil)
}

// reservedRole names the role of column name when it is the model's primary
// key, version, soft-delete, created-at or updated-at column.
func (u UpdateQuery[T]) reservedRole(name string) string {
	roles := []struct {
		role string
		idx  []int
	}{
		{"primary key", u.meta.PK},
		{"version", []int{u.meta.Version}},
		{"soft-delete", []int{u.meta.SoftDelete}},
		{"created-at", []int{u.meta.CreatedAt}},
		{"updated-at", []int{u.meta.UpdatedAt}},
	}
	for _, r := range roles {
		for _, i := range r.idx {
			if i >= 0 && i < len(u.meta.Columns) && u.meta.Columns[i].Name == name {
				return r.role
			}
		}
	}
	return ""
}

// checkVersionKind refuses, at build time, a version field that is not an
// integer, since the in-memory increment after the update needs one.
func (u UpdateQuery[T]) checkVersionKind() error {
	v := reflect.ValueOf(u.meta.Ptr(u.model, u.meta.Version))
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		switch v.Elem().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return nil
		}
	}
	return u.fail(ErrValidation, errors.New("version column must be an integer field"))
}

// advanceVersion mirrors the database's version increment into the model.
func (u UpdateQuery[T]) advanceVersion() {
	e := reflect.ValueOf(u.meta.Ptr(u.model, u.meta.Version)).Elem()
	switch e.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		e.SetInt(e.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		e.SetUint(e.Uint() + 1)
	}
}
