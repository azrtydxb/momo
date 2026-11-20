// SPDX-License-Identifier: Apache-2.0

package rangeop_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
	"github.com/azrtydxb/momo/rangeop"
	"github.com/azrtydxb/momo/types"
)

var (
	colQty    = momo.NewColumn[types.Range[int32]]("docs", "qty", 0)
	colBig    = momo.NewColumn[types.Range[int64]]("docs", "big", 0)
	colPrice  = momo.NewColumn[types.Range[types.Numeric]]("docs", "price", 0)
	colDay    = momo.NewColumn[types.Range[types.Date]]("docs", "day", 0)
	colLocal  = momo.NewColumn[types.Range[types.Timestamp]]("docs", "local", 0)
	colDuring = momo.NewColumn[types.Range[time.Time]]("docs", "during", 0)
)

var (
	t10 = time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	t12 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
)

// Catches: a wrong operator, a value interpolated instead of bound, an operand
// bound without the cast that lets the server choose between the element and
// the range form of @>, a cast that names the wrong type for an element
// (timestamp against timestamptz) and an expression rendered with the wrong
// function.
func TestRangePredicatesGolden(t *testing.T) {
	cases := []struct {
		name     string
		pred     momo.Predicate
		wantSQL  string
		wantArgs []any
	}{
		{"contains int4", rangeop.Contains(colQty, 5), `"docs"."qty" @> $1::integer`, []any{int32(5)}},
		{"contains int8", rangeop.Contains(colBig, 5), `"docs"."big" @> $1::bigint`, []any{int64(5)}},
		{"contains numeric", rangeop.Contains(colPrice, types.Numeric{Decimal: "1.5"}), `"docs"."price" @> $1::numeric`, []any{types.Numeric{Decimal: "1.5"}}},
		{"contains date", rangeop.Contains(colDay, types.Date{Time: t10}), `"docs"."day" @> $1::date`, []any{types.Date{Time: t10}}},
		{"contains timestamp", rangeop.Contains(colLocal, types.Timestamp{Time: t10}), `"docs"."local" @> $1::timestamp`, []any{types.Timestamp{Time: t10}}},
		{"contains timestamptz", rangeop.Contains(colDuring, t10), `"docs"."during" @> $1::timestamptz`, []any{t10}},
		{"contains range", rangeop.ContainsRange(colDuring, types.ClosedOpen(t10, t12)), `"docs"."during" @> $1::tstzrange`, []any{types.ClosedOpen(t10, t12)}},
		{"contained by", rangeop.ContainedBy(colQty, types.ClosedOpen[int32](1, 9)), `"docs"."qty" <@ $1::int4range`, []any{types.ClosedOpen[int32](1, 9)}},
		{"overlaps", rangeop.Overlaps(colLocal, types.ClosedOpen(types.Timestamp{Time: t10}, types.Timestamp{Time: t12})), `"docs"."local" && $1::tsrange`, []any{types.ClosedOpen(types.Timestamp{Time: t10}, types.Timestamp{Time: t12})}},
		{"adjacent", rangeop.Adjacent(colDay, types.EmptyRange[types.Date]()), `"docs"."day" -|- $1::daterange`, []any{types.EmptyRange[types.Date]()}},
		{"strictly left", rangeop.StrictlyLeft(colPrice, types.EmptyRange[types.Numeric]()), `"docs"."price" << $1::numrange`, []any{types.EmptyRange[types.Numeric]()}},
		{"strictly right", rangeop.StrictlyRight(colBig, types.ClosedOpen[int64](1, 2)), `"docs"."big" >> $1::int8range`, []any{types.ClosedOpen[int64](1, 2)}},
		{"is empty", rangeop.IsEmpty(colQty), `isempty("docs"."qty")`, nil},
		{"lower infinite", rangeop.LowerInfinite(colQty), `lower_inf("docs"."qty")`, nil},
		{"upper infinite", rangeop.UpperInfinite(colQty), `upper_inf("docs"."qty")`, nil},
		{"lower compared", rangeop.Lower(colQty).Ge(3), `lower("docs"."qty") >= $1`, []any{int32(3)}},
		{"upper compared", rangeop.Upper(colDuring).Lt(t12), `upper("docs"."during") < $1`, []any{t12}},
		{"lower is null", rangeop.Lower(colQty).IsNull(), `lower("docs"."qty") IS NULL`, nil},
	}
	for _, c := range cases {
		got, err := exprtest.Compile(dialect.Postgres, c.pred)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := `SELECT COUNT(*) FROM "docs" WHERE ` + c.wantSQL; got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, want)
		}
		if len(got.Args) == 0 && len(c.wantArgs) == 0 {
			continue
		}
		if !reflect.DeepEqual(got.Args, c.wantArgs) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.wantArgs)
		}
	}
}

// Catches: a range predicate silently rendered on a dialect with no range type.
func TestRangePredicatesUnsupportedOffPostgres(t *testing.T) {
	preds := map[string]momo.Predicate{
		"contains":       rangeop.Contains(colQty, 5),
		"contains range": rangeop.ContainsRange(colQty, types.ClosedOpen[int32](1, 2)),
		"contained by":   rangeop.ContainedBy(colQty, types.ClosedOpen[int32](1, 2)),
		"overlaps":       rangeop.Overlaps(colQty, types.ClosedOpen[int32](1, 2)),
		"adjacent":       rangeop.Adjacent(colQty, types.ClosedOpen[int32](1, 2)),
		"strictly left":  rangeop.StrictlyLeft(colQty, types.ClosedOpen[int32](1, 2)),
		"strictly right": rangeop.StrictlyRight(colQty, types.ClosedOpen[int32](1, 2)),
		"is empty":       rangeop.IsEmpty(colQty),
		"lower infinite": rangeop.LowerInfinite(colQty),
		"upper infinite": rangeop.UpperInfinite(colQty),
		"lower":          rangeop.Lower(colQty).Eq(1),
		"upper":          rangeop.Upper(colQty).Eq(1),
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, p := range preds {
			_, err := exprtest.Compile(d, p)
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s: err = %v, want ErrUnsupported", d, name, err)
			}
		}
	}
}

// Catches: a range predicate that binds with the plain arg path, leaking the
// values compared with a sensitive column into hook events.
func TestRangePredicatesMaskSensitiveColumn(t *testing.T) {
	for _, sensitive := range []bool{true, false} {
		var fl uint16
		if sensitive {
			fl = momo.FlagSensitive
		}
		qty := momo.NewColumn[types.Range[int32]]("docs", "qty", fl)
		preds := map[string]momo.Predicate{
			"contains":       rangeop.Contains(qty, 5),
			"contains range": rangeop.ContainsRange(qty, types.ClosedOpen[int32](1, 2)),
			"overlaps":       rangeop.Overlaps(qty, types.ClosedOpen[int32](1, 2)),
			"lower":          rangeop.Lower(qty).Eq(1),
		}
		for name, p := range preds {
			rec := &obstest.Recorder{}
			drv := fakedriver.New(dialect.Postgres)
			db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
			if _, err := momo.Select[exprtest.Tally](db).From(momo.TableRef{Name: "docs"}).Columns(momo.CountAll()).Where(p).All(t.Context()); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			evs, calls := rec.Events(), drv.Calls()
			if len(evs) == 0 || len(calls) == 0 || len(evs[len(evs)-1].Args) != 1 {
				t.Fatalf("%s: want one statement with one argument", name)
			}
			if got := evs[len(evs)-1].Args[0] == momo.RedactedArg; got != sensitive {
				t.Errorf("%s sensitive=%v: hook saw %v", name, sensitive, evs[len(evs)-1].Args[0])
			}
			if calls[len(calls)-1].Args[0] == momo.RedactedArg {
				t.Errorf("%s: the driver received the placeholder", name)
			}
		}
	}
}
