// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// comparisons is the comparison and ordering surface every comparable
// expression carries. The compile-time assertions below keep the sets from
// drifting apart again.
type comparisons[T any] interface {
	Eq(T) Predicate
	Ne(T) Predicate
	Lt(T) Predicate
	Le(T) Predicate
	Gt(T) Predicate
	Ge(T) Predicate
	EqExpr(Expression[T]) Predicate
	NeExpr(Expression[T]) Predicate
	LtExpr(Expression[T]) Predicate
	LeExpr(Expression[T]) Predicate
	GtExpr(Expression[T]) Predicate
	GeExpr(Expression[T]) Predicate
	IsNull() Predicate
	IsNotNull() Predicate
	IsDistinctFrom(T) Predicate
	IsNotDistinctFrom(T) Predicate
	Between(lo, hi T) Predicate
	In(...T) Predicate
	NotIn(...T) Predicate
	InQuery(Subquery) Predicate
	NotInQuery(Subquery) Predicate
	Asc() Order
	Desc() Order
}

var (
	_ comparisons[int64] = Column[int64]{}
	_ comparisons[int64] = NullColumn[int64]{}
	_ comparisons[int64] = ReadOnlyColumn[int64]{}
	_ comparisons[int64] = NullReadOnlyColumn[int64]{}
	_ comparisons[int64] = CreateOnlyColumn[int64]{}
	_ comparisons[int64] = ImmutableColumn[int64]{}
	_ comparisons[int64] = Agg[int64]{}
	_ comparisons[int64] = FuncExpr[int64]{}
)

func TestAggCarriesTheSharedComparisons(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Agg[int64]{}),
		reflect.TypeOf(Column[int64]{}),
	} {
		pt := reflect.PointerTo(typ)
		for _, name := range []string{"EqExpr", "In", "Between", "IsNull", "IsNotNull", "NotIn", "InQuery"} {
			if _, ok := pt.MethodByName(name); !ok {
				t.Errorf("%s lacks %s", typ, name)
			}
		}
	}
}

func TestAggComparisonRendersAgainstTheAggregate(t *testing.T) {
	c, err := compile(dialect.Postgres, func(b *Builder) {
		CountAll().Between(1, 5).AppendSQL(b)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, "COUNT(*) BETWEEN") {
		t.Fatalf("got %q", c.SQL)
	}
}

// TestLibraryErrorsAreMatchable sweeps the non-test sources: a library error
// must be built through newError, validation or a sentinel, never as a bare
// errors.New("momo: ...") that errors.Is cannot match.
func TestLibraryErrorsAreMatchable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "errors.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, ln := range strings.Split(string(src), "\n") {
			if strings.Contains(ln, `errors.New("momo:`) {
				t.Errorf("%s:%d: bare momo error without a sentinel kind: %s", f, i+1, strings.TrimSpace(ln))
			}
		}
	}
}

func TestNoQuerierErrorMatchesValidation(t *testing.T) {
	err := error(noQuerier("select", "t"))
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v", err)
	}
	var me *Error
	if !errors.As(err, &me) || me.Table != "t" {
		t.Fatalf("not a typed error: %v", err)
	}
}
