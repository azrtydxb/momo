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

// Catches: EqPtr(nil) matching no row (= NULL) and SetPtr(nil) not writing NULL.
func TestNullColumnPtrRoundTrip(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		val := "has-body"
		for title, body := range map[string]*string{"p-nil": nil, "p-val": &val} {
			if _, err := momo.Insert[suite.Document](db).
				Values(suite.DocTenantID.Set(1), suite.DocTitle.Set(title), suite.DocBody.SetPtr(body)).Exec(ctx); err != nil {
				t.Fatal(err)
			}
		}
		count := func(p momo.Predicate) int64 {
			n, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(1), p).Count(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		if n := count(suite.DocBody.EqPtr(nil)); n != 1 {
			t.Fatalf("EqPtr(nil) matched %d rows, want 1", n)
		}
		if n := count(suite.DocBody.EqPtr(&val)); n != 1 {
			t.Fatalf("EqPtr(&val) matched %d rows, want 1", n)
		}
		// Update both rows: nil clears, non-nil writes.
		if _, err := momo.Update[suite.Document](db).Set(suite.DocBody.SetPtr(nil)).Where(suite.DocTitle.Eq("p-val")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		other := "other"
		if _, err := momo.Update[suite.Document](db).Set(suite.DocBody.SetPtr(&other)).Where(suite.DocTitle.Eq("p-nil")).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		if n := count(suite.DocBody.EqPtr(nil)); n != 1 {
			t.Fatalf("after update EqPtr(nil) matched %d rows, want 1", n)
		}
		if n := count(suite.DocBody.EqPtr(&other)); n != 1 {
			t.Fatalf("after update EqPtr(&other) matched %d rows, want 1", n)
		}
	})
}
