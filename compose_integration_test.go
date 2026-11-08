// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestCompositionHelpers(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		if _, err := momo.Exec(ctx, db, "INSERT INTO documents (tenant_id, title) VALUES (1,'a'),(2,'b'),(2,'c')"); err != nil {
			t.Fatal(err)
		}
		byTenant := func(id int64) momo.Scope[suite.Document] {
			return func(q momo.SelectQuery[suite.Document]) momo.SelectQuery[suite.Document] {
				return q.Where(suite.DocTenantID.Eq(id))
			}
		}
		n, err := momo.Select[suite.Document](db).Scope(byTenant(2)).Count(ctx)
		if err != nil || n != 2 {
			t.Fatalf("scope count = %d %v", n, err)
		}
		al := momo.NewAllowlist(suite.DocTenantID, suite.DocTitle)
		f := momo.Filter{Or: []momo.Filter{
			{Field: "tenant_id", Op: "eq", Value: int64(1)},
			{Field: "title", Op: "in", Value: []string{"c"}},
		}}
		p, err := f.Predicate(al)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := momo.Select[suite.Document](db).Where(p).OrderBy(suite.DocTitle.Asc()).All(ctx)
		if err != nil || len(rows) != 2 || rows[0].Title != "a" || rows[1].Title != "c" {
			t.Fatalf("filter rows = %v %v", rows, err)
		}
		bad := momo.Filter{Field: "tenant_id; drop", Op: "eq", Value: 1}
		if _, err := bad.Predicate(al); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("unknown field err = %v", err)
		}
		locked := momo.Select[suite.Document](db).ForUpdate("NOWAIT")
		_, err = locked.SQL()
		if d == dialect.SQLite {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("sqlite FOR UPDATE NOWAIT err = %v, want ErrUnsupported", err)
			}
		} else if err != nil {
			t.Fatalf("%s FOR UPDATE NOWAIT: %v", d, err)
		}
	})
}
