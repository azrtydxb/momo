// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

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
