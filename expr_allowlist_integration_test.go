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

func TestEscapeLike(t *testing.T) {
	if got := momo.EscapeLike(`50%_off!`); got != `50!%!_off!!` {
		t.Fatalf("EscapeLike = %q", got)
	}
	testopen.EachWithMySQL(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := t.Context()
		for _, title := range []string{"50%_off", "50 off", "50!off"} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1, ?)", title); err != nil {
				t.Fatal(err)
			}
		}
		for _, c := range []struct{ in, want string }{{"%_", "50%_off"}, {"!", "50!off"}} {
			rows, err := momo.Select[suite.Document](db).Where(momo.ContainsLiteral(suite.DocTitle, c.in)).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Title != c.want {
				t.Fatalf("ContainsLiteral(%q) returned %d rows, want only %q", c.in, len(rows), c.want)
			}
		}
	})
}
