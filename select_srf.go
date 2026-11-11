// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"fmt"
	"reflect"
	"time"

	"github.com/azrtydxb/momo/dialect"
)

// Set-returning functions as FROM and JOIN sources. They are PostgreSQL
// only; the other dialects fail the build with ErrUnsupported.

func srfErr(msg string, args ...any) error {
	return &Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf(msg, args...)}
}

func srfPostgresOnly(b *Builder, what string) bool {
	if b.d.Name != dialect.Postgres {
		b.unsupported(what)
		return false
	}
	return true
}

// UnnestArray is one array argument of Unnest; UnnestCol implements it.
type UnnestArray interface {
	columnName() string
	appendArray(b *Builder) error
}

// UnnestCol is one bound array of an Unnest source and the name of the
// column it produces. Build it with UnnestValues or UnnestValuesAs.
type UnnestCol[T any] struct {
	name    string
	sqlType string
	vals    []T
	err     error
}

// UnnestValues binds vals as one array argument that unnest turns into the
// column name. The array's SQL element type is derived from T: string is
// text, int64 and int bigint, int32 integer, int16 smallint, float64 double
// precision, float32 real, bool boolean, time.Time timestamptz, []byte bytea
// and a 16-byte array uuid (a pointer to any of these allows NULL elements;
// named types follow their underlying kind). The array is bound as one
// parameter and is not masked in logs or traces, even for sensitive values;
// do not unnest secrets. Any other T fails the build
// with ErrValidation; use UnnestValuesAs to name the type.
func UnnestValues[T any](name string, vals []T) UnnestCol[T] {
	t, err := arrayElemSQLType(reflect.TypeFor[T]())
	return UnnestCol[T]{name: name, sqlType: t, vals: vals, err: err}
}

// UnnestValuesAs is UnnestValues with an explicit SQL element type such as
// numeric or timestamp (words of letters, digits and underscores, with an
// optional length suffix). The bound array is cast to sqlType[].
func UnnestValuesAs[T any](name, sqlType string, vals []T) UnnestCol[T] {
	c := UnnestCol[T]{name: name, sqlType: sqlType, vals: vals}
	if err := validTypeName(sqlType); err != nil {
		c.err = srfErr("unnest column %q type %q: %w", name, sqlType, err)
	}
	return c
}

// Ref is the typed column through which the query reads this column from
// the Unnest source called source (its alias).
func (c UnnestCol[T]) Ref(source string) Column[T] { return ColumnOf[T](source, c.name) }

func (c UnnestCol[T]) columnName() string { return c.name }

func (c UnnestCol[T]) appendArray(b *Builder) error {
	if c.err != nil {
		return c.err
	}
	b.Arg(c.vals)
	b.WriteString("::" + c.sqlType + "[]")
	return nil
}

func arrayElemSQLType(t reflect.Type) (string, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t == reflect.TypeFor[time.Time]():
		return "timestamptz", nil
	case t == reflect.TypeFor[[]byte]():
		return "bytea", nil
	}
	switch t.Kind() {
	case reflect.String:
		return "text", nil
	case reflect.Int64, reflect.Int:
		return "bigint", nil
	case reflect.Int32:
		return "integer", nil
	case reflect.Int16:
		return "smallint", nil
	case reflect.Float64:
		return "double precision", nil
	case reflect.Float32:
		return "real", nil
	case reflect.Bool:
		return "boolean", nil
	case reflect.Array:
		if t.Len() == 16 && t.Elem().Kind() == reflect.Uint8 {
			return "uuid", nil
		}
	}
	return "", srfErr("unnest has no SQL array type for Go type %s; use UnnestValuesAs to name it", t)
}

// UnnestSource is unnest(a1, a2, ...) AS alias (c1, c2, ...) used as a FROM
// or JOIN source. Build it with Unnest.
type UnnestSource struct {
	alias      string
	cols       []UnnestArray
	ordinality string
}

// Unnest expands one or more bound arrays into rows, one column per array:
//
//	momo.Select[fx](db).From(momo.Unnest("fx", currency, rate))
//
// renders FROM unnest($1::text[], $2::double precision[]) AS "fx"
// ("currency", "rate"). Arrays of different lengths are padded with NULL to
// the longest. Read the columns through UnnestCol.Ref(alias). Unnest also
// serves as a CTE body source and as the right side of a Join. PostgreSQL
// only.
func Unnest(alias string, cols ...UnnestArray) UnnestSource {
	return UnnestSource{alias: alias, cols: append([]UnnestArray(nil), cols...)}
}

// WithOrdinality adds WITH ORDINALITY: an extra 1-based bigint column called
// name, read as an int64 column through ColumnOf.
func (u UnnestSource) WithOrdinality(name string) UnnestSource {
	u.ordinality = name
	return u
}

// Qualifier is the alias the source's columns are qualified with.
func (u UnnestSource) Qualifier() string { return u.alias }

// notLockable marks a source LockOf cannot name.
func (UnnestSource) notLockable() {}

func (u UnnestSource) appendFrom(b *Builder) {
	if !srfPostgresOnly(b, "unnest") {
		return
	}
	if len(u.cols) == 0 {
		b.SetErr(srfErr("Unnest needs at least one array"))
		return
	}
	names := make([]string, 0, len(u.cols)+1)
	for _, c := range u.cols {
		names = append(names, c.columnName())
	}
	if u.ordinality != "" {
		names = append(names, u.ordinality)
	}
	if err := checkSRFNames(u.alias, names); err != nil {
		b.SetErr(err)
		return
	}
	b.WriteString("unnest(")
	for i, c := range u.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		if err := c.appendArray(b); err != nil {
			b.SetErr(err)
			return
		}
	}
	b.writeByte(')')
	if u.ordinality != "" {
		b.WriteString(" WITH ORDINALITY")
	}
	appendSRFAlias(b, u.alias, names)
}

func checkSRFNames(alias string, cols []string) error {
	if err := validName(alias); err != nil {
		return srfErr("source alias %q: %w", alias, err)
	}
	seen := map[string]struct{}{}
	for _, c := range cols {
		if err := validName(c); err != nil {
			return srfErr("column name %q: %w", c, err)
		}
		if _, dup := seen[c]; dup {
			return srfErr("column name %q appears twice", c)
		}
		seen[c] = struct{}{}
	}
	return nil
}

func appendSRFAlias(b *Builder, alias string, cols []string) {
	b.WriteString(" AS ")
	b.Ident(alias)
	b.WriteString(" (")
	for i, c := range cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(c)
	}
	b.writeByte(')')
}

// SeriesInt is the integer types GenerateSeries accepts.
type SeriesInt interface{ ~int | ~int32 | ~int64 }

// Series is a generate_series(...) source producing one column of type T.
// Build it with GenerateSeries or GenerateTimeSeries.
type Series[T any] struct {
	alias, col string
	sqlType    string
	start      any
	stop       any
	step       any // nil: the default step of 1
	micros     bool
	err        error
}

// GenerateSeries is generate_series(start, stop[, step]) AS alias (col) over
// integers: the values from start to stop inclusive. A zero step fails the
// build with ErrValidation; a negative step counts down. More than one step
// is also a validation error. PostgreSQL only.
func GenerateSeries[T SeriesInt](alias, col string, start, stop T, step ...T) Series[T] {
	s := Series[T]{alias: alias, col: col, sqlType: "bigint", start: start, stop: stop}
	if reflect.TypeFor[T]().Kind() == reflect.Int32 {
		s.sqlType = "integer"
	}
	switch {
	case len(step) > 1:
		s.err = srfErr("GenerateSeries takes at most one step")
	case len(step) == 1 && step[0] == 0:
		s.err = srfErr("GenerateSeries step must not be zero")
	case len(step) == 1:
		s.step = step[0]
	}
	return s
}

// GenerateTimeSeries is generate_series over timestamptz: start, start+step,
// ... up to stop. step is truncated to whole microseconds; a step that
// truncates to zero fails the build with ErrValidation. PostgreSQL only.
func GenerateTimeSeries(alias, col string, start, stop time.Time, step time.Duration) Series[time.Time] {
	s := Series[time.Time]{alias: alias, col: col, sqlType: "timestamptz", start: start, stop: stop, micros: true}
	if step.Microseconds() == 0 {
		s.err = srfErr("GenerateTimeSeries step must be at least one microsecond")
		return s
	}
	s.step = step.Microseconds()
	return s
}

// Column is the typed column the series produces.
func (s Series[T]) Column() Column[T] { return ColumnOf[T](s.alias, s.col) }

// Qualifier is the alias the source's column is qualified with.
func (s Series[T]) Qualifier() string { return s.alias }

// notLockable marks a source LockOf cannot name.
func (Series[T]) notLockable() {}

func (s Series[T]) appendFrom(b *Builder) {
	if !srfPostgresOnly(b, "generate_series") {
		return
	}
	if s.err != nil {
		b.SetErr(s.err)
		return
	}
	if err := checkSRFNames(s.alias, []string{s.col}); err != nil {
		b.SetErr(err)
		return
	}
	b.WriteString("generate_series(")
	b.Arg(s.start)
	b.WriteString("::" + s.sqlType + ", ")
	b.Arg(s.stop)
	b.WriteString("::" + s.sqlType)
	if s.step != nil {
		b.WriteString(", ")
		b.Arg(s.step)
		if s.micros {
			b.WriteString("::bigint * interval '1 microsecond'")
		} else {
			b.WriteString("::" + s.sqlType)
		}
	}
	b.writeByte(')')
	appendSRFAlias(b, s.alias, []string{s.col})
}

// UnnestColumnSource is LATERAL unnest(col) AS alias (name): the elements of
// an array-valued column or expression of an earlier source, one row each.
// Build it with UnnestColumn and name it with As.
type UnnestColumnSource[T any] struct {
	arr        Expression[[]T]
	alias, col string
}

// UnnestColumn expands an array column (or any array expression) of a source
// earlier in the FROM list into rows:
//
//	momo.Select[row](db).From(q2).
//		CrossJoin(momo.UnnestColumn(q2Flags).As("fl", "flag"))
//
// renders CROSS JOIN LATERAL unnest("q2"."flags") AS "fl" ("flag"). Read the
// element through the Column of the source returned by As. Unlike Unnest it
// takes a column and not bound values, so it can reference columns of the
// sources before it. Without As the source has no alias and fails the build
// with ErrValidation. It must come after the source that holds the array: as
// the first FROM item it fails the build with ErrValidation. Use CrossJoin, or
// LeftJoin with momo.SQL("true") as the ON condition to keep the rows whose
// array is NULL or empty. A NULL array yields no rows. PostgreSQL only.
func UnnestColumn[T any](col Expression[[]T]) UnnestColumnSource[T] {
	return UnnestColumnSource[T]{arr: col}
}

// As names the source alias and the column the elements are read from.
func (u UnnestColumnSource[T]) As(alias, col string) UnnestColumnSource[T] {
	u.alias, u.col = alias, col
	return u
}

// Column is the typed column of the elements.
func (u UnnestColumnSource[T]) Column() Column[T] { return ColumnOf[T](u.alias, u.col) }

// Qualifier is the alias the source's column is qualified with.
func (u UnnestColumnSource[T]) Qualifier() string { return u.alias }

// notLockable marks a source LockOf cannot name.
func (UnnestColumnSource[T]) notLockable() {}

// needsEarlierSource marks a source that reads a column of a source before
// it, so a query cannot use it as its first FROM item.
type needsEarlierSource interface{ needsEarlierSource() }

func (UnnestColumnSource[T]) needsEarlierSource() {}

func (u UnnestColumnSource[T]) appendFrom(b *Builder) {
	if !srfPostgresOnly(b, "LATERAL unnest") {
		return
	}
	if u.arr == nil {
		b.SetErr(srfErr("UnnestColumn needs an array expression"))
		return
	}
	if err := checkSRFNames(u.alias, []string{u.col}); err != nil {
		b.SetErr(err)
		return
	}
	b.WriteString("LATERAL unnest(")
	u.arr.AppendSQL(b)
	b.writeByte(')')
	appendSRFAlias(b, u.alias, []string{u.col})
}
