// SPDX-License-Identifier: Apache-2.0

//go:build integration

package sqlshared_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitepure"
	"github.com/azrtydxb/momo/jsonop"
	"github.com/azrtydxb/momo/types"
)

type exprRow struct {
	ID int64
	S  string
	F  float64
}

var exprRowMeta = momo.Meta[exprRow]{
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "s"}, {Name: "f"}},
	Ptr:      func(m *exprRow, i int) any { return [...]any{&m.ID, &m.S, &m.F}[i] },
	Value:    func(m *exprRow, i int) any { return [...]any{m.ID, m.S, m.F}[i] },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*exprRow) MomoMeta() *momo.Meta[exprRow] { return &exprRowMeta }

type exprTime struct{ T time.Time }

var exprTimeMeta = momo.Meta[exprTime]{
	Columns:  []momo.ColumnMeta{{Name: "t"}},
	Ptr:      func(m *exprTime, _ int) any { return &m.T },
	Value:    func(m *exprTime, _ int) any { return m.T },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}

func (*exprTime) MomoMeta() *momo.Meta[exprTime] { return &exprTimeMeta }

var storedStamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`)

// Catches: a JSON operator or time expression that renders on SQLite but
// behaves differently on one of the two SQLite drivers (boolean extraction,
// SQL NULL versus JSON null, strftime fractions, month overflow, the stored
// timestamp text of Now).
func TestJSONAndTimeExpressionsOnEachDriver(t *testing.T) {
	eachDriver(t, func(t *testing.T, o opener) {
		_, drv := openAt(t, o, sqlitepure.Config{})
		db := momo.New(drv)
		ctx := context.Background()
		for _, stmt := range []string{
			"CREATE TABLE ev (id integer PRIMARY KEY, doc text, at text)",
			`INSERT INTO ev VALUES (1, '{"via":"api","gone":null,"flag":true,"off":false,"n":{"x":["p","q"]}}', '2023-01-31 10:15:30.250')`,
			"INSERT INTO ev VALUES (2, NULL, '2023-02-28 10:15:30.250')",
			"INSERT INTO ev VALUES (3, 'null', '2024-03-15 13:45:30.000')",
		} {
			if _, err := momo.Exec(ctx, db, stmt); err != nil {
				t.Fatal(err)
			}
		}
		doc := momo.NewNullColumn[types.JSON[any]]("ev", "doc", 0)
		at := momo.ColumnOf[time.Time]("ev", "at")
		id := momo.ColumnOf[int64]("ev", "id")
		src := momo.TableRef{Name: "ev"}

		ids := func(p momo.Predicate) []int64 {
			t.Helper()
			rows, err := momo.Select[exprRow](db).From(src).Columns(id, id, id).Where(p).OrderBy(id.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var out []int64
			for _, r := range rows {
				out = append(out, r.ID)
			}
			return out
		}
		eq := func(name string, got []int64, want ...int64) {
			t.Helper()
			if !slices.Equal(got, want) {
				t.Errorf("%s = %v, want %v", name, got, want)
			}
		}
		eq("text", ids(jsonop.Text(doc, "via").Eq("api")), 1)
		eq("path", ids(jsonop.Path(doc, "n", "x", "1").Eq("q")), 1)
		eq("bool true reads as 1", ids(jsonop.Text(doc, "flag").Eq("1")), 1)
		eq("bool false reads as 0", ids(jsonop.Text(doc, "off").Eq("0")), 1)
		eq("has key", ids(jsonop.HasKey(doc, "via")), 1)
		eq("has key with JSON null value", ids(jsonop.HasKey(doc, "gone")), 1)
		eq("SQL NULL column", ids(doc.IsNull()), 2)
		eq("JSON null extracts as NULL", ids(jsonop.Text(doc, "via").IsNull()), 2, 3)
		_, err := momo.Select[exprRow](db).From(src).Columns(id, id, id).
			Where(jsonop.Contains(doc, map[string]any{"via": "api"})).All(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("jsonop.Contains = %v, want ErrUnsupported", err)
		}

		tm := func(row int64, e momo.Expr) time.Time {
			t.Helper()
			r, err := momo.Select[exprTime](db).From(src).Columns(e).Where(id.Eq(row)).One(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.T
		}
		num := func(row int64, e momo.Expr) float64 {
			t.Helper()
			r, err := momo.Select[exprRow](db).From(src).Columns(id, id, e).Where(id.Eq(row)).One(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return r.F
		}
		for _, c := range []struct {
			f    momo.ExtractField
			want float64
		}{
			{momo.ExtractYear, 2024}, {momo.ExtractMonth, 3}, {momo.ExtractDay, 15},
			{momo.ExtractHour, 13}, {momo.ExtractMinute, 45}, {momo.ExtractSecond, 30},
			{momo.ExtractQuarter, 1}, {momo.ExtractDow, 5}, {momo.ExtractISODow, 5},
			{momo.ExtractDoy, 75}, {momo.ExtractEpoch, 1710510330},
		} {
			if got := num(3, momo.Extract(c.f, at)); got != c.want {
				t.Errorf("Extract %s = %v, want %v", c.f, got, c.want)
			}
		}
		for _, c := range []struct {
			u    momo.TruncUnit
			want time.Time
		}{
			{momo.TruncSecond, time.Date(2024, 3, 15, 13, 45, 30, 0, time.UTC)},
			{momo.TruncMinute, time.Date(2024, 3, 15, 13, 45, 0, 0, time.UTC)},
			{momo.TruncHour, time.Date(2024, 3, 15, 13, 0, 0, 0, time.UTC)},
			{momo.TruncDay, time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)},
			{momo.TruncWeek, time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)},
			{momo.TruncMonth, time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)},
			{momo.TruncQuarter, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
			{momo.TruncYear, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		} {
			if got := tm(3, momo.DateTrunc(c.u, at)); !got.Equal(c.want) {
				t.Errorf("DateTrunc %s = %v, want %v", c.u, got, c.want)
			}
		}
		month := momo.IntervalOf(1, momo.IntervalMonth)
		if got, want := tm(2, momo.AddInterval(at, month)), time.Date(2023, 3, 28, 10, 15, 30, 250_000_000, time.UTC); !got.Equal(want) {
			t.Errorf("Feb 28 + 1 month = %v, want %v", got, want)
		}
		// SQLite normalises the overflow: Jan 31 2023 plus a month is Mar 3.
		if got, want := tm(1, momo.AddInterval(at, month)), time.Date(2023, 3, 3, 10, 15, 30, 250_000_000, time.UTC); !got.Equal(want) {
			t.Errorf("Jan 31 + 1 month = %v, want %v", got, want)
		}
		if got, want := tm(3, momo.SubInterval(at, momo.IntervalOf(2, momo.IntervalWeek))), time.Date(2024, 3, 1, 13, 45, 30, 0, time.UTC); !got.Equal(want) {
			t.Errorf("- 2 weeks = %v, want %v", got, want)
		}

		r, err := momo.Select[exprRow](db).From(src).Columns(id, momo.Now(), id).Where(id.Eq(1)).One(ctx)
		if err != nil || !storedStamp.MatchString(r.S) {
			t.Errorf("Now() = %q, %v; want the stored timestamp text", r.S, err)
		}
	})
}
