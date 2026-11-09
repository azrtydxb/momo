// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func seedTitles(t *testing.T, db *momo.DB, titles ...string) {
	t.Helper()
	for i, title := range titles {
		if _, err := momo.Exec(context.Background(), db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", int64(i+1), title); err != nil {
			t.Fatal(err)
		}
	}
}

// Catches: Func[T] rendering a call the database rejects or binding its
// arguments wrongly.
func TestFuncExecutes(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		seedTitles(t, db, "x", "y", "z")
		docs := momo.TableRef{Name: "documents"}

		rows, err := momo.Select[kvRow](db).From(docs).
			Columns(suite.DocTenantID, momo.Func[int64]("abs", suite.DocTenantID)).
			Where(momo.Func[int64]("abs", suite.DocTenantID).Gt(1)).
			OrderBy(suite.DocTenantID.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].N != 2 || rows[1].N != 3 {
			t.Fatalf("Func abs = %+v, %v", rows, err)
		}
	})
}
