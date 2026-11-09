// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
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
	c, err := compile(dialect.For(dialect.Postgres), func(b *Builder) {
		CountAll().Between(1, 5).AppendSQL(b)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, "COUNT(*) BETWEEN") {
		t.Fatalf("got %q", c.SQL)
	}
}

// TestLibraryErrorsAreMatchable parses the non-test sources of the root
// package (err.go excluded) and flags a bare library error: an errors.New
// or fmt.Errorf call whose first argument is a double-quoted literal starting
// with "momo:" and containing no %w verb. Such an error has no Kind, so
// errors.Is cannot match it; build it through newError, validation or
// noQuerier instead. It does not look at other messages or at non-literal
// arguments. Package-level var declarations are skipped (sentinels are
// matchable), and allowedBare lists the one deliberate exception.
func TestLibraryErrorsAreMatchable(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "err.go" {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				continue
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && isBareMomoError(call) && !allowedBare(f, call) {
					t.Errorf("%s: bare momo error without a sentinel kind", fset.Position(call.Pos()))
				}
				return true
			})
		}
	}
}

// allowedBare lists the one deliberate kind-less error: the cause handed to
// the rollback of a chunked insert when fn panics (db_atomic.go). The panic is
// re-raised and the error is never returned, and a panic is not a validation
// failure, so it carries no misleading Kind.
func allowedBare(file string, call *ast.CallExpr) bool {
	lit, ok := call.Args[0].(*ast.BasicLit)
	return file == "db_atomic.go" && ok && strings.HasPrefix(lit.Value, `"momo: panic during chunked`)
}

// isBareMomoError reports whether call is errors.New or fmt.Errorf with a
// "momo:" message and no %w verb.
func isBareMomoError(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) == 0 {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if !(pkg.Name == "errors" && sel.Sel.Name == "New") && !(pkg.Name == "fmt" && sel.Sel.Name == "Errorf") {
		return false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	return ok && strings.HasPrefix(lit.Value, `"momo:`) && !strings.Contains(lit.Value, "%w")
}

func TestCanceledErrorMatchesKindAndContext(t *testing.T) {
	err := error(newError(ErrCanceled, "select", "t", context.Canceled))
	if !errors.Is(err, ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want a match for ErrCanceled and context.Canceled", err)
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
