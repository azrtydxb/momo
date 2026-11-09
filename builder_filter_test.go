// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: Builder.AppendFilter losing the FILTER text or its bound argument,
// accepting a nil predicate, or rendering FILTER where MySQL has none.
func TestBuilderAppendFilter(t *testing.T) {
	n := momo.NewColumn[int64]("docs", "n", 0)

	pg := momo.NewBuilder(dialect.For(dialect.Postgres))
	pg.WriteString("count(*)")
	pg.AppendFilter(n.Gt(1))
	if pg.Err() != nil || pg.String() != `count(*) FILTER (WHERE "docs"."n" > $1)` || len(pg.Args()) != 1 || pg.Args()[0] != int64(1) {
		t.Errorf("postgres: got %q %v, %v", pg.String(), pg.Args(), pg.Err())
	}

	nilPred := momo.NewBuilder(dialect.For(dialect.Postgres))
	nilPred.AppendFilter(nil)
	if !errors.Is(nilPred.Err(), momo.ErrValidation) {
		t.Errorf("nil predicate: err = %v; want ErrValidation", nilPred.Err())
	}

	my := momo.NewBuilder(dialect.For(dialect.MySQL))
	my.AppendFilter(n.Gt(1))
	if !errors.Is(my.Err(), momo.ErrUnsupported) {
		t.Errorf("mysql: err = %v; want ErrUnsupported", my.Err())
	}
}
