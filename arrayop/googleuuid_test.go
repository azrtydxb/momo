// SPDX-License-Identifier: Apache-2.0

package arrayop_test

import (
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/arrayop"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/exprtest"
)

// Catches: a []uuid.UUID array predicate that binds without the ::uuid[] cast
// (the server cannot type the parameter), as []types.UUID already does.
func TestArrayOperatorsGoogleUUIDGolden(t *testing.T) {
	a := uuid.MustParse("00000001-0000-0000-0000-000000000000")
	b := uuid.MustParse("00000002-0000-0000-0000-000000000000")
	one := momo.NewColumn[uuid.UUID]("docs", "uid", 0)
	many := momo.NewColumn[[]uuid.UUID]("docs", "uids", 0)
	cases := []struct {
		name     string
		pred     momo.Predicate
		wantSQL  string
		wantArgs []any
	}{
		{"any of", arrayop.AnyOf(one, a, b), `"docs"."uid" = ANY($1::uuid[])`, []any{[]uuid.UUID{a, b}}},
		{"overlaps", arrayop.Overlaps(many, a), `"docs"."uids" && $1::uuid[]`, []any{[]uuid.UUID{a}}},
		{"contains", arrayop.Contains(many, a, b), `"docs"."uids" @> $1::uuid[]`, []any{[]uuid.UUID{a, b}}},
		{"contained by", arrayop.ContainedBy(many, a), `"docs"."uids" <@ $1::uuid[]`, []any{[]uuid.UUID{a}}},
		{"nothing is an empty array", arrayop.Overlaps[uuid.UUID](many), `"docs"."uids" && $1::uuid[]`, []any{[]uuid.UUID{}}},
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
		if !reflect.DeepEqual(got.Args, c.wantArgs) {
			t.Errorf("%s: args = %#v, want %#v", c.name, got.Args, c.wantArgs)
		}
	}
}
