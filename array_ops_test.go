// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/obstest"
)

// Catches: an array predicate that binds with the plain arg path, leaking the
// values compared against a sensitive column into hook events.
func TestArrayOperatorsMaskSensitiveColumn(t *testing.T) {
	for _, sensitive := range []bool{true, false} {
		var fl uint16
		if sensitive {
			fl = momo.FlagSensitive
		}
		tags := momo.NewColumn[[]string]("docs", "tags", fl)
		title := momo.NewColumn[string]("docs", "title", fl)
		preds := map[string]momo.Predicate{
			"contains":     momo.ArrayContains(tags, "a"),
			"contained by": momo.ArrayContainedBy(tags, "a"),
			"overlaps":     momo.ArrayOverlaps(tags, "a"),
			"any of":       momo.AnyOf(title, "a"),
		}
		for name, p := range preds {
			rec := &obstest.Recorder{}
			drv := fakedriver.New(dialect.Postgres)
			db := momo.New(drv, momo.WithHook(rec), momo.WithArgLogging())
			if _, err := momo.Select[unitTally](db).From(momo.TableRef{Name: "docs"}).Columns(momo.CountAll()).Where(p).All(t.Context()); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			evs, calls := rec.Events(), drv.Calls()
			if len(evs) == 0 || len(calls) == 0 || len(evs[len(evs)-1].Args) != 1 {
				t.Fatalf("%s: want one statement with one argument", name)
			}
			seen := evs[len(evs)-1].Args[0]
			if got := seen == momo.RedactedArg; got != sensitive {
				t.Errorf("%s sensitive=%v: hook saw %v", name, sensitive, seen)
			}
			if calls[len(calls)-1].Args[0] == momo.RedactedArg {
				t.Errorf("%s: the driver received the placeholder", name)
			}
		}
	}
}

var (
	arrTags  = momo.NewColumn[[]string]("docs", "tags", 0)
	arrTitle = momo.NewColumn[string]("docs", "title", 0)
	arrIDs   = momo.NewColumn[[]int32]("docs", "ids", 0)
)

// Catches: a value interpolated instead of bound, a list expanded into one
// bind per element, a wrong operator, and a nil slice bound as NULL.
func TestArrayOperatorsGolden(t *testing.T) {
	cases := []struct {
		name     string
		pred     momo.Predicate
		wantSQL  string
		wantArgs []any
	}{
		{"contains", momo.ArrayContains(arrTags, "a", "b"), `"docs"."tags" @> $1`, []any{[]string{"a", "b"}}},
		{"contained by", momo.ArrayContainedBy(arrTags, "a", "b"), `"docs"."tags" <@ $1`, []any{[]string{"a", "b"}}},
		{"overlaps", momo.ArrayOverlaps(arrTags, "x'; DROP TABLE docs; --"), `"docs"."tags" && $1`, []any{[]string{"x'; DROP TABLE docs; --"}}},
		{"overlaps ints", momo.ArrayOverlaps(arrIDs, 1, 2, 3), `"docs"."ids" && $1`, []any{[]int32{1, 2, 3}}},
		{"any of", momo.AnyOf(arrTitle, "j", "k", "l"), `"docs"."title" = ANY($1)`, []any{[]string{"j", "k", "l"}}},
		{"no values is an empty array", momo.ArrayOverlaps[string](arrTags), `"docs"."tags" && $1`, []any{[]string{}}},
		{"any of nothing", momo.AnyOf[string](arrTitle), `"docs"."title" = ANY($1)`, []any{[]string{}}},
	}
	for _, c := range cases {
		got, err := momo.Select[unitTally](typedDB(dialect.Postgres)).From(momo.TableRef{Name: "docs"}).Columns(momo.CountAll()).Where(c.pred).SQL()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if want := `SELECT COUNT(*) FROM "docs" WHERE ` + c.wantSQL; got.SQL != want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, c.wantArgs) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.wantArgs)
		}
	}
}

// Catches: an array predicate silently rendered where the dialect has no
// array type.
func TestArrayOperatorsUnsupportedOffPostgres(t *testing.T) {
	preds := map[string]momo.Predicate{
		"contains":     momo.ArrayContains(arrTags, "a"),
		"contained by": momo.ArrayContainedBy(arrTags, "a"),
		"overlaps":     momo.ArrayOverlaps(arrTags, "a"),
		"any of":       momo.AnyOf(arrTitle, "a"),
	}
	for _, d := range []dialect.Name{dialect.SQLite, dialect.MySQL} {
		for name, p := range preds {
			_, err := momo.Select[unitTally](typedDB(d)).From(momo.TableRef{Name: "docs"}).Columns(momo.CountAll()).Where(p).SQL()
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s %s: err = %v, want ErrUnsupported", d, name, err)
			}
		}
	}
}
