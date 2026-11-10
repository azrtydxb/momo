// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/azrtydxb/momo/dialect"
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
	// softDelete marks the query as a SoftDelete statement.
	softDelete bool
	count      func(ctx context.Context, q Querier, ps []Predicate) (int64, error)
	// bulk is the VALUES list of BulkUpdate; nil for every other update.
	bulk *bulkSpec
	// extra holds the predicates interceptors added; plan appends them after
	// the unfiltered-mutation guard, so they never count as a caller filter.
	extra []Predicate
	// with holds the WITH clause; from the FROM source (UPDATE ... FROM).
	with []cte
	from TableSource
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

// With prefixes the statement with a common table expression. cols optionally
// names its columns. The body may be a SelectQuery, an ExprSelect, a Union or
// (on PostgreSQL only) an INSERT, UPDATE or DELETE with RETURNING; a
// data-modifying body on another dialect fails with ErrUnsupported. Reference
// the CTE with CTE and ColumnOf, for instance as the From source. MariaDB does
// not accept the clause here (ErrUnsupported), and BulkUpdate cannot be
// combined with it (ErrValidation).
func (u UpdateQuery[T]) With(name string, q Subquery, cols ...string) UpdateQuery[T] {
	u.with = addCTE(u.with, name, q, cols, false)
	return u
}

// WithRecursive is With for a recursive CTE.
func (u UpdateQuery[T]) WithRecursive(name string, q Subquery, cols ...string) UpdateQuery[T] {
	u.with = addCTE(u.with, name, q, cols, true)
	return u
}

// From adds a second source to the statement, rendering UPDATE t SET ... FROM
// src WHERE ...: the SET expressions and the WHERE may reference src's
// columns (ColumnOf on a CTE or FromSubquery alias, or a model's columns).
// src is a table, aliased table, CTE or FromSubquery, as in a join. Join src
// to the target in Where. Calling From again replaces the source.
//
// PostgreSQL and SQLite (3.33 or later) render UPDATE ... FROM. MySQL and
// MariaDB render the multi-table form UPDATE t, src SET t.col = ... WHERE ...,
// qualifying the assigned columns. Soft-deleted rows of src are not filtered:
// add that predicate to Where. Interceptors apply to the target table only,
// not to src.
//
// With a From source the unfiltered-mutation guard is stricter: at least one
// Where predicate must reference a column of the target table, otherwise the
// statement joins every target row to src and fails with ErrUnfilteredMutation
// unless AllRows confirms it. A raw SQL fragment does not count as such a
// reference. From cannot be combined with BulkUpdate (ErrValidation).
func (u UpdateQuery[T]) From(src TableSource) UpdateQuery[T] { u.from = src; return u }

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
	return newError(kind, "update", u.meta.Table, err)
}

// updModelWritable reports whether Model writes the column from the struct.
func updModelWritable(f uint16) bool {
	return columnAssignable(writeUpdate, f) && f&(FlagPK|FlagVersion|FlagSoftDelete|FlagCreatedAt|FlagUpdatedAt) == 0
}

// updPKEq is the predicate "qualifier.col = ?" with a bound value.
type updPKEq struct {
	table, col string
	val        any
	flags      uint16
}

func (p updPKEq) AppendSQL(b *Builder) {
	b.QualifiedIdent(p.table, p.col)
	b.WriteString(" = ")
	b.argFor(p.flags, p.val)
}
func (updPKEq) resultType() bool { return false }

// pkPredicates returns the primary-key equality predicates for m.
func (u UpdateQuery[T]) pkPredicates(m *T) ([]Predicate, error) {
	if len(u.meta.PK) == 0 {
		return nil, u.fail(ErrValidation, errors.New("model has no primary key"))
	}
	ps := make([]Predicate, 0, len(u.meta.PK)+1)
	for _, i := range u.meta.PK {
		ps = append(ps, updPKEq{u.meta.Table, u.meta.Columns[i].Name, u.meta.Value(m, i), u.meta.Columns[i].Flags})
	}
	return ps, nil
}

// versioned reports whether the query is a version-checked Model update.
func (u UpdateQuery[T]) versioned() bool {
	return !u.softDelete && u.model != nil && u.meta.Version >= 0
}

// plan resolves the SET assignments and WHERE predicates, applying the
// guardrails. It does not render.
func (u UpdateQuery[T]) plan() ([]Assignment, []Predicate, error) {
	if u.meta.Table == "" {
		return nil, nil, u.fail(ErrValidation, errors.New("model has no table"))
	}
	if u.softDelete {
		return u.planSoftDelete()
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
			sets = append(sets, Assignment{col: c.Name, kind: setValue, val: u.meta.Value(u.model, i), flags: c.Flags})
		}
		pks, err := u.pkPredicates(u.model)
		if err != nil {
			return nil, nil, err
		}
		where = append(pks, where...)
		if u.versioned() {
			v := u.meta.Version
			where = append(where, updPKEq{u.meta.Table, u.meta.Columns[v].Name, u.meta.Value(u.model, v), u.meta.Columns[v].Flags})
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
	sets, err := u.applyFieldHooks(sets)
	if err != nil {
		return nil, nil, err
	}
	if ci := u.meta.UpdatedAt; ci >= 0 && columnAssignable(writeUpdate, u.meta.Columns[ci].Flags) {
		// updated_at rides in the same statement as the change, so an
		// update that matches no row changes nothing.
		if err := checkTimestampField(u.meta, ci); err != nil {
			return nil, nil, u.fail(ErrValidation, err)
		}
		if v, ok := autoTimestamp(u.meta, ci, u.q.conn().now()); ok {
			sets = append(sets, Assignment{col: u.meta.Columns[ci].Name, kind: setValue, val: v, flags: u.meta.Columns[ci].Flags})
		}
	}
	if !hasEffectiveFilter(where) && !u.allRows {
		return nil, nil, u.fail(ErrUnfilteredMutation, nil)
	}
	if u.from != nil && !u.allRows && !referencesTable(u.q.Dialect(), where, u.meta.Table) {
		// Without a condition on the target, FROM is a cross join and every
		// target row is updated.
		return nil, nil, u.fail(ErrUnfilteredMutation, errors.New("with From, Where must reference the target table (or confirm with AllRows)"))
	}
	return sets, append(where, u.extra...), nil
}

// SQL renders the statement without executing it. Guardrail violations are
// reported here, before anything can reach the database.
//
// Interceptors run with a background context here; the execution methods pass
// the statement's own context.
func (u UpdateQuery[T]) SQL() (Compiled, error) {
	u, err := u.intercepted(context.Background())
	if err != nil {
		return Compiled{}, err
	}
	return u.compileSQL()
}

// intercepted runs the handle's interceptors with ctx and returns the query
// carrying their predicates.
func (u UpdateQuery[T]) intercepted(ctx context.Context) (UpdateQuery[T], error) {
	kind := "update"
	if u.softDelete {
		kind = "delete"
	}
	extra, err := runInterceptors(ctx, u.q, kind, u.meta.Table)
	if err != nil {
		return u, err
	}
	u.extra = extra
	return u, nil
}

// compileSQL renders the statement; interceptors already ran (intercepted).
func (u UpdateQuery[T]) compileSQL() (Compiled, error) {
	if u.q == nil {
		return Compiled{}, noQuerier("update", u.meta.Table)
	}
	if u.bulk != nil {
		if u.from != nil || len(u.with) > 0 {
			return Compiled{}, u.fail(ErrValidation, errors.New("BulkUpdate cannot be combined with With or From"))
		}
		return u.bulkSQL()
	}
	return compile(u.q.Dialect(), u.render)
}

func (UpdateQuery[T]) isDML() {}

// isSubquery lets an UPDATE be the body of a WITH clause (PostgreSQL only).
func (UpdateQuery[T]) isSubquery() {}

// AppendSQL renders the statement in parentheses as a WITH clause body. It
// runs the handle's interceptors with a background context, as SQL does, but
// not the beforeWrite checks of Exec. Only PostgreSQL accepts the result.
func (u UpdateQuery[T]) AppendSQL(b *Builder) {
	if u.q == nil {
		b.SetErr(noQuerier("update", u.meta.Table))
		return
	}
	if u.bulk != nil {
		b.unsupported("BulkUpdate as a WITH clause body")
		return
	}
	u, err := u.intercepted(context.Background())
	if err != nil {
		b.SetErr(err)
		return
	}
	b.writeByte('(')
	u.render(b)
	b.writeByte(')')
}

// render writes the UPDATE statement; interceptors already ran.
func (u UpdateQuery[T]) render(b *Builder) {
	sets, where, err := u.plan()
	if err != nil {
		b.SetErr(err)
		return
	}
	target := TableRef{Name: u.meta.Table, Schema: u.meta.Schema}
	names := append(cteNames(u.with), target.Qualifier())
	if q, ok := u.from.(interface{ Qualifier() string }); ok {
		names = append(names, q.Qualifier())
	}
	// MySQL has no UPDATE ... FROM: the multi-table form lists the source
	// after the target and needs the assigned columns qualified.
	multi := u.from != nil && b.d.Name == dialect.MySQL
	qual := ""
	if multi {
		qual = target.Qualifier()
	}
	// The target table and FROM source are the statement's scope, so a
	// correlated subquery in SET or WHERE may reference them.
	b.withScope(names, func() {
		if len(u.with) > 0 && b.d.Flavor == dialect.FlavorMariaDB {
			// Not verified against MariaDB, so refused rather than claimed.
			b.unsupported("WITH before UPDATE on MariaDB")
			return
		}
		writeWithClause(b, u.with)
		b.WriteString("UPDATE ")
		target.appendFrom(b)
		if multi {
			b.WriteString(", ")
			u.from.appendFrom(b)
		}
		b.WriteString(" SET ")
		for i, a := range sets {
			if i > 0 {
				b.WriteString(", ")
			}
			u.appendAssignment(b, a, qual)
		}
		if u.versioned() {
			vc := u.meta.Columns[u.meta.Version].Name
			if len(sets) > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(qual, vc)
			b.WriteString(" = ")
			b.QualifiedIdent(target.Qualifier(), vc)
			b.WriteString(" + 1")
		}
		if u.from != nil && !multi {
			b.WriteString(" FROM ")
			u.from.appendFrom(b)
		}
		if len(where) > 0 {
			b.WriteString(" WHERE ")
			appendAnd(b, where)
		}
	})
	renderReturning(b, u.meta, u.returning, func(col string) error {
		return u.fail(ErrValidation, fmt.Errorf("Returning: column %q is not mapped by %s", col, u.meta.Table))
	})
}

func (u UpdateQuery[T]) appendAssignment(b *Builder, a Assignment, qual string) {
	b.QualifiedIdent(qual, a.col)
	b.WriteString(" = ")
	switch a.kind {
	case setValue:
		b.argFor(a.flags, a.val)
	case setExpr:
		b.exprFor(a.flags, a.expr)
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
	u, err := u.intercepted(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := u.beforeWrite(ctx); err != nil {
		return Result{}, err
	}
	c, err := u.compileSQL()
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
	u, err := u.intercepted(ctx)
	if err != nil {
		return nil, err
	}
	if err := u.beforeWrite(ctx); err != nil {
		return nil, err
	}
	c, err := u.compileSQL()
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
	// The interceptor predicates count too: a row they hide is not found,
	// not a lock conflict.
	n, err := u.count(ctx, q, append(pks, u.extra...))
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

// applyFieldHooks adds the OnUpdate values for columns the caller did not
// assign with Set, then runs the validators over every column the statement
// sets. Soft-delete updates do not run it.
func (u UpdateQuery[T]) applyFieldHooks(sets []Assignment) ([]Assignment, error) {
	fp, err := newFieldPlan(u.q, u.meta)
	if err != nil || len(fp.hooks) == 0 {
		return sets, err
	}
	var zero T
	m := u.model
	if m == nil {
		m = &zero
	}
	for _, ci := range slices.Sorted(maps.Keys(fp.upd)) {
		c := u.meta.Columns[ci]
		if slices.ContainsFunc(u.assigns, func(a Assignment) bool { return a.col == c.Name }) {
			continue // an explicit Set wins
		}
		if role := u.reservedRole(c.Name); role != "" {
			hint := ""
			if role == "updated-at" {
				hint = "; updated_at is maintained automatically on every update (set the time source with WithClock)"
			}
			return nil, fieldError("update", u.meta.Table, c.Name, fmt.Errorf("OnUpdate: column %q is the %s column and cannot be set by a field hook%s", c.Name, role, hint))
		}
		if !columnAssignable(writeUpdate, c.Flags) {
			return nil, fieldError("update", u.meta.Table, c.Name, fmt.Errorf("OnUpdate: column %q cannot be written by UPDATE", c.Name))
		}
		a := Assignment{col: c.Name, kind: setValue, val: fp.upd[ci](m), flags: c.Flags}
		if j := slices.IndexFunc(sets, func(s Assignment) bool { return s.col == c.Name }); j >= 0 {
			sets[j] = a
		} else {
			sets = append(sets, a)
		}
	}
	err = fp.validate("update", u.meta, func(ci int) (any, bool) {
		for _, a := range sets {
			if a.col == u.meta.Columns[ci].Name {
				return assignedValue(a)
			}
		}
		return nil, false
	})
	return sets, err
}
