// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// Catches: Like and ILike lacking ESCAPE '!', where "!%" would be a literal
// "!" plus a wildcard (PostgreSQL's default escape is a backslash), so an
// escaped user value matched far more than itself.
func TestLikeWithEscapedUserInputMatchesLiterally(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := t.Context()
		seedTitles(t, db, "100%", "100 percent", "a_b", "axb", "wow!", "wow", "Plain")
		cases := []struct{ in, want string }{{"100%", "100%"}, {"a_b", "a_b"}, {"wow!", "wow!"}, {"%", "100%"}, {"_", "a_b"}}
		for _, c := range cases {
			pats := []momo.Predicate{momo.Like(suite.DocTitle, "%"+momo.EscapeLike(c.in)+"%")}
			if d == dialect.Postgres {
				pats = append(pats, momo.ILike(suite.DocTitle, "%"+momo.EscapeLike(c.in)+"%"))
			}
			for _, p := range pats {
				rows, err := momo.Select[suite.Document](db).Where(p).All(ctx)
				if err != nil {
					t.Fatalf("%q: %v", c.in, err)
				}
				if len(rows) != 1 || rows[0].Title != c.want {
					t.Fatalf("%q matched %d rows, want only %q", c.in, len(rows), c.want)
				}
			}
		}
		// Intentional wildcards in a hand-written pattern keep working.
		rows, err := momo.Select[suite.Document](db).Where(momo.Like(suite.DocTitle, "a_b")).All(ctx)
		if err != nil || len(rows) != 2 {
			t.Fatalf("wildcard _ matched %d rows, err %v, want 2", len(rows), err)
		}
	})
}
