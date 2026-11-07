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

func TestJoinsAliasesSelfJoin(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		for _, row := range []struct {
			tenant int64
			title  string
		}{{1, "a"}, {1, "b"}, {2, "c"}} {
			if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (?, ?)", row.tenant, row.title); err != nil {
				t.Fatal(err)
			}
		}
		rows, err := selfJoin(db).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []pair{{"a", "b"}, {"b", "a"}}
		if len(rows) != len(want) || (pair{rows[0].L, rows[0].R}) != want[0] || (pair{rows[1].L, rows[1].R}) != want[1] {
			t.Fatalf("self-join rows = %+v, want %+v", rows, want)
		}
	})
}
