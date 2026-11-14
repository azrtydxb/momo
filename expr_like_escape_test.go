// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: Like or ILike rendering without ESCAPE '!', so a value escaped with
// EscapeLike still acts as a wildcard on the server.
func TestLikeFormsCarryEscapeClausePerDialect(t *testing.T) {
	cases := []struct {
		name string
		d    dialect.Name
		pred momo.Predicate
		want string
	}{
		{"like pg", dialect.Postgres, momo.Like(DocTitle, `a!%b%`), `"documents"."title" LIKE $1 ESCAPE '!'`},
		{"like sqlite", dialect.SQLite, momo.Like(DocTitle, `a!%b%`), `"documents"."title" LIKE ? ESCAPE '!'`},
		{"like mysql", dialect.MySQL, momo.Like(DocTitle, `a!%b%`), "`documents`.`title` LIKE ? ESCAPE '!'"},
		{"ilike pg", dialect.Postgres, momo.ILike(DocTitle, `a!%b%`), `"documents"."title" ILIKE $1 ESCAPE '!'`},
		{"ilike sqlite", dialect.SQLite, momo.ILike(DocTitle, `a!%b%`), `LOWER("documents"."title") LIKE LOWER(?) ESCAPE '!'`},
		{"ilike mysql", dialect.MySQL, momo.ILike(DocTitle, `a!%b%`), "LOWER(`documents`.`title`) LIKE LOWER(?) ESCAPE '!'"},
		{"not ilike sqlite", dialect.SQLite, momo.Not(momo.ILike(DocTitle, `x`)), `NOT (LOWER("documents"."title") LIKE LOWER(?) ESCAPE '!')`},
		{"not ilike mysql", dialect.MySQL, momo.Not(momo.ILike(DocTitle, `x`)), "NOT (LOWER(`documents`.`title`) LIKE LOWER(?) ESCAPE '!')"},
		{"not like pg", dialect.Postgres, momo.Not(momo.Like(DocTitle, `x`)), `NOT ("documents"."title" LIKE $1 ESCAPE '!')`},
		{"not ilike pg", dialect.Postgres, momo.Not(momo.ILike(DocTitle, `x`)), `NOT ("documents"."title" ILIKE $1 ESCAPE '!')`},
	}
	for _, c := range cases {
		db := momo.New(fakedriver.New(c.d))
		got, err := momo.Select[unitDocument](db).Where(c.pred).SQL()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !strings.Contains(got.SQL, c.want) {
			t.Errorf("%s: SQL %q lacks %q", c.name, got.SQL, c.want)
		}
		if len(got.Args) != 1 {
			t.Errorf("%s: args = %v, want the pattern bound once", c.name, got.Args)
		}
	}
}

// Catches: a typed In or NotIn over more values than the dialect binds
// reaching the driver instead of failing at build time.
func TestTypedInOverBindLimitIsValidationError(t *testing.T) {
	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite, dialect.MySQL} {
		limit := dialect.For(d).MaxParams
		db := momo.New(fakedriver.New(d))
		fits := make([]int64, limit)
		over := make([]int64, limit+1)
		if _, err := momo.Select[unitDocument](db).Where(DocTenantID.In(fits...)).SQL(); err != nil {
			t.Fatalf("%s: exactly the limit must pass: %v", d, err)
		}
		for name, p := range map[string]momo.Predicate{"In": DocTenantID.In(over...), "NotIn": DocTenantID.NotIn(over...)} {
			_, err := momo.Select[unitDocument](db).Where(p).SQL()
			if !errors.Is(err, momo.ErrValidation) {
				t.Errorf("%s %s: err = %v, want ErrValidation", d, name, err)
			}
		}
	}
}

// Catches: the cap ignoring WithMaxParams, or ignoring binds already used by
// other predicates in the same statement.
func TestTypedInRespectsMaxParamsAndPriorBinds(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithMaxParams(5))
	if _, err := momo.Select[unitDocument](db).Where(DocTenantID.In(1, 2, 3, 4, 5)).SQL(); err != nil {
		t.Fatalf("5 values under a limit of 5: %v", err)
	}
	_, err := momo.Select[unitDocument](db).Where(DocTenantID.In(1, 2, 3, 4, 5, 6)).SQL()
	if !errors.Is(err, momo.ErrValidation) || !strings.Contains(err.Error(), "limit of 5") {
		t.Fatalf("err = %v, want ErrValidation naming the limit of 5", err)
	}
	_, err = momo.Select[unitDocument](db).Where(DocTitle.Eq("x"), DocTenantID.In(1, 2, 3, 4, 5)).SQL()
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("one prior bind plus 5 values: err = %v, want ErrValidation", err)
	}
}
