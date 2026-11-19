// SPDX-License-Identifier: Apache-2.0

//go:build integration

package rangeop_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/rangeop"
	"github.com/azrtydxb/momo/types"
)

// idRow reads one bigint column.
type idRow struct{ ID int64 }

var idRowMeta = momo.Meta[idRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}},
	Ptr:      func(m *idRow, _ int) any { return &m.ID },
	Value:    func(m *idRow, _ int) any { return m.ID },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*idRow) MomoMeta() *momo.Meta[idRow] { return &idRowMeta }

// rangeRow reads an id and one column of each range type.
type rangeRow struct {
	ID     int64
	Qty    types.Range[int32]
	Big    types.Range[int64]
	Price  types.Range[types.Numeric]
	Day    types.Range[types.Date]
	Local  types.Range[types.Timestamp]
	During types.Range[time.Time]
}

var rangeRowMeta = momo.Meta[rangeRow]{
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "qty"}, {Name: "big"}, {Name: "price"}, {Name: "day"}, {Name: "local"}, {Name: "during"}},
	Ptr: func(m *rangeRow, i int) any {
		return [...]any{&m.ID, &m.Qty, &m.Big, &m.Price, &m.Day, &m.Local, &m.During}[i]
	},
	Value: func(m *rangeRow, i int) any {
		return [...]any{m.ID, m.Qty, m.Big, m.Price, m.Day, m.Local, m.During}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*rangeRow) MomoMeta() *momo.Meta[rangeRow] { return &rangeRowMeta }

var (
	rID     = momo.NewColumn[int64]("rng", "id", momo.FlagPK)
	rQty    = momo.NewColumn[types.Range[int32]]("rng", "qty", 0)
	rBig    = momo.NewColumn[types.Range[int64]]("rng", "big", 0)
	rPrice  = momo.NewColumn[types.Range[types.Numeric]]("rng", "price", 0)
	rDay    = momo.NewColumn[types.Range[types.Date]]("rng", "day", 0)
	rLocal  = momo.NewColumn[types.Range[types.Timestamp]]("rng", "local", 0)
	rDuring = momo.NewColumn[types.Range[time.Time]]("rng", "during", 0)
)

const createRng = `CREATE TABLE rng (id bigint PRIMARY KEY, qty int4range NOT NULL, big int8range NOT NULL,
  price numrange NOT NULL, day daterange NOT NULL, local tsrange NOT NULL, during tstzrange NOT NULL)`

func date(y int, m time.Month, d int) types.Date {
	return types.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

func at(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func local(h int) types.Timestamp { return types.Timestamp{Time: at(h)} }

// Catches: a range that does not survive the trip through its PostgreSQL
// column: a bound, its inclusiveness, an infinite side or the empty range lost
// or changed, for each of the six range types.
func TestRangeRoundTripLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, createRng); err != nil {
		t.Fatal(err)
	}
	rows := []rangeRow{
		{
			ID:     1,
			Qty:    types.ClosedOpen[int32](1, 10),
			Big:    types.ClosedOpen[int64](-9000000000, 9000000000),
			Price:  types.NewRange(types.Incl(types.Numeric{Decimal: "1.5"}), types.Incl(types.Numeric{Decimal: "2.5"})),
			Day:    types.ClosedOpen(date(2026, 1, 1), date(2026, 2, 1)),
			Local:  types.ClosedOpen(local(10), local(12)),
			During: types.ClosedOpen(at(10), at(12)),
		},
		{
			ID:     2,
			Qty:    types.EmptyRange[int32](),
			Big:    types.EmptyRange[int64](),
			Price:  types.EmptyRange[types.Numeric](),
			Day:    types.EmptyRange[types.Date](),
			Local:  types.EmptyRange[types.Timestamp](),
			During: types.EmptyRange[time.Time](),
		},
		{
			ID:     3,
			Qty:    types.NewRange(types.Incl[int32](5), types.NoBound[int32]()),
			Big:    types.NewRange(types.NoBound[int64](), types.Excl[int64](5)),
			Price:  types.NewRange(types.NoBound[types.Numeric](), types.NoBound[types.Numeric]()),
			Day:    types.NewRange(types.Incl(date(2026, 1, 1)), types.NoBound[types.Date]()),
			Local:  types.NewRange(types.NoBound[types.Timestamp](), types.Excl(local(12))),
			During: types.NewRange(types.Incl(at(10)), types.NoBound[time.Time]()),
		},
	}
	for _, r := range rows {
		if _, err := momo.Exec(ctx, db, "INSERT INTO rng (id, qty, big, price, day, local, during) VALUES (?, ?, ?, ?, ?, ?, ?)",
			r.ID, r.Qty, r.Big, r.Price, r.Day, r.Local, r.During); err != nil {
			t.Fatalf("insert %d: %v", r.ID, err)
		}
	}
	got, err := momo.Select[rangeRow](db).From(momo.TableRef{Name: "rng"}).
		Columns(rID, rQty, rBig, rPrice, rDay, rLocal, rDuring).OrderBy(rID.Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, rows) {
		t.Fatalf("rows read back differ:\n got %+v\nwant %+v", got, rows)
	}
}

// Catches: a range bound that reaches the database as SQL instead of a value
// (the text form of a range is parsed by the server), and a time zone that
// shifts a timestamptz bound: the instant must survive a non-UTC location.
func TestRangeBindsInstants(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, createRng); err != nil {
		t.Fatal(err)
	}
	zone := time.FixedZone("UTC+4", 4*3600)
	in := types.ClosedOpen(time.Date(2026, 1, 1, 14, 0, 0, 0, zone), time.Date(2026, 1, 1, 16, 0, 0, 0, zone))
	if _, err := momo.Exec(ctx, db, "INSERT INTO rng VALUES (1, '(,)', '(,)', '(,)', '(,)', '(,)', ?)", in); err != nil {
		t.Fatal(err)
	}
	rows, err := momo.Select[rangeRow](db).From(momo.TableRef{Name: "rng"}).
		Columns(rID, rQty, rBig, rPrice, rDay, rLocal, rDuring).All(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("select: %v (%d rows)", err, len(rows))
	}
	want := types.ClosedOpen(at(10), at(12))
	if !reflect.DeepEqual(rows[0].During, want) {
		t.Fatalf("during = %+v, want %+v", rows[0].During, want)
	}
}

func ids(t *testing.T, db *momo.DB, p momo.Predicate) []int64 {
	t.Helper()
	rows, err := momo.Select[idRow](db).From(momo.TableRef{Name: "rng"}).Columns(rID).Where(p).OrderBy(rID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := []int64{}
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// Catches: a wrong range operator, an element or range operand bound without
// the cast the server needs to choose between @> on an element and on a range,
// a wrong result for the empty range or an unbounded side, and Lower, Upper,
// IsEmpty and the infinity tests reading the wrong bound.
func TestRangePredicatesLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, createRng); err != nil {
		t.Fatal(err)
	}
	unbounded := "'(,)'"
	for i, q := range []string{"'[1,10)'", "'[10,20)'", "'[5,15)'", "'empty'", "'(,5)'"} {
		if _, err := momo.Exec(ctx, db, "INSERT INTO rng VALUES (?, "+q+", "+unbounded+", "+unbounded+", "+unbounded+", "+unbounded+", "+unbounded+")", int64(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name string
		pred momo.Predicate
		want []int64
	}{
		{"contains 5", rangeop.Contains(rQty, 5), []int64{1, 3}},
		{"contains 4", rangeop.Contains(rQty, 4), []int64{1, 5}},
		{"contains range", rangeop.ContainsRange(rQty, types.ClosedOpen[int32](6, 8)), []int64{1, 3}},
		{"contained by", rangeop.ContainedBy(rQty, types.ClosedOpen[int32](0, 12)), []int64{1, 4}},
		{"overlaps", rangeop.Overlaps(rQty, types.ClosedOpen[int32](9, 11)), []int64{1, 2, 3}},
		{"adjacent", rangeop.Adjacent(rQty, types.ClosedOpen[int32](10, 12)), []int64{1}},
		{"strictly left", rangeop.StrictlyLeft(rQty, types.ClosedOpen[int32](10, 12)), []int64{1, 5}},
		{"strictly right", rangeop.StrictlyRight(rQty, types.ClosedOpen[int32](0, 10)), []int64{2}},
		{"is empty", rangeop.IsEmpty(rQty), []int64{4}},
		{"lower is 5", rangeop.Lower(rQty).Eq(5), []int64{3}},
		{"upper above 14", rangeop.Upper(rQty).Gt(14), []int64{2, 3}},
		{"lower infinite", rangeop.LowerInfinite(rQty), []int64{5}},
		{"upper infinite", rangeop.UpperInfinite(rQty), []int64{}},
		{"combined", momo.And(rangeop.Overlaps(rQty, types.ClosedOpen[int32](9, 11)), momo.Not(rangeop.Contains(rQty, 15))), []int64{1, 3}},
	}
	for _, c := range cases {
		if got := ids(t, db, c.pred); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ids = %v, want %v", c.name, got, c.want)
		}
	}
}

// Catches: an unset range field stored as the range (0,0) instead of the empty
// range: the zero Range[T] must be written as empty for every element type, so
// the database holds an empty range, IsEmpty finds it, and it reads back empty.
func TestZeroRangeStoresAsEmptyLive(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, createRng); err != nil {
		t.Fatal(err)
	}
	var zero rangeRow
	zero.ID = 1
	if _, err := momo.Exec(ctx, db, "INSERT INTO rng (id, qty, big, price, day, local, during) VALUES (?, ?, ?, ?, ?, ?, ?)",
		zero.ID, zero.Qty, zero.Big, zero.Price, zero.Day, zero.Local, zero.During); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]momo.Predicate{
		"int4": rangeop.IsEmpty(rQty), "int8": rangeop.IsEmpty(rBig), "numeric": rangeop.IsEmpty(rPrice),
		"date": rangeop.IsEmpty(rDay), "timestamp": rangeop.IsEmpty(rLocal), "timestamptz": rangeop.IsEmpty(rDuring),
	} {
		if got := ids(t, db, p); !reflect.DeepEqual(got, []int64{1}) {
			t.Errorf("%s: a zero range is not stored as the empty range: ids = %v", name, got)
		}
	}
	rows, err := momo.Select[rangeRow](db).From(momo.TableRef{Name: "rng"}).
		Columns(rID, rQty, rBig, rPrice, rDay, rLocal, rDuring).All(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatalf("select: %v (%d rows)", err, len(rows))
	}
	r := rows[0]
	if !r.Qty.IsEmpty() || !r.Big.IsEmpty() || !r.Price.IsEmpty() || !r.Day.IsEmpty() || !r.Local.IsEmpty() || !r.During.IsEmpty() {
		t.Errorf("a zero range did not read back empty: %+v", r)
	}
	if !r.Qty.Empty {
		t.Errorf("a range read from the database as empty has no Empty flag: %+v", r.Qty)
	}
}

// Catches: an element cast that names the wrong type (a timestamp bound against
// a timestamptz range), so a predicate on one of the six range types fails at
// the server with "operator does not exist".
func TestRangeContainsEveryElementType(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	if _, err := momo.Exec(ctx, db, createRng); err != nil {
		t.Fatal(err)
	}
	if _, err := momo.Exec(ctx, db, `INSERT INTO rng VALUES (1, '[1,10)', '[1,10)', '[1.5,2.5]', '[2026-01-01,2026-02-01)',
  '[2026-01-01 10:00,2026-01-01 12:00)', '[2026-01-01 10:00+00,2026-01-01 12:00+00)')`); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		in, out momo.Predicate
	}{
		"int4range":   {rangeop.Contains(rQty, 5), rangeop.Contains(rQty, 10)},
		"int8range":   {rangeop.Contains(rBig, 5), rangeop.Contains(rBig, 10)},
		"numrange":    {rangeop.Contains(rPrice, types.Numeric{Decimal: "2.5"}), rangeop.Contains(rPrice, types.Numeric{Decimal: "2.6"})},
		"daterange":   {rangeop.Contains(rDay, date(2026, 1, 31)), rangeop.Contains(rDay, date(2026, 2, 1))},
		"tsrange":     {rangeop.Contains(rLocal, local(11)), rangeop.Contains(rLocal, local(12))},
		"tstzrange":   {rangeop.Contains(rDuring, at(11)), rangeop.Contains(rDuring, at(12))},
		"range <@ it": {rangeop.ContainedBy(rDuring, types.ClosedOpen(at(9), at(13))), rangeop.ContainedBy(rDuring, types.ClosedOpen(at(9), at(11)))},
	} {
		if got := ids(t, db, c.in); !reflect.DeepEqual(got, []int64{1}) {
			t.Errorf("%s: inside: ids = %v, want [1]", name, got)
		}
		if got := ids(t, db, c.out); len(got) != 0 {
			t.Errorf("%s: outside: ids = %v, want none", name, got)
		}
	}
}
