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

// Zero rows from All is (nil, nil), an empty IN list matches nothing without
// emitting IN (), and only One reports ErrNotFound.
func TestEmptyResultNotNotFoundIntegration(t *testing.T) {
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		suite.CreateDocuments(t, db, suite.CapsOf(db))
		ctx := context.Background()
		rows, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(99)).All(ctx)
		if err != nil || rows != nil {
			t.Fatalf("All on zero rows = %v, %v; want nil, nil", rows, err)
		}
		empty, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.In()).All(ctx)
		if err != nil || len(empty) != 0 {
			t.Fatalf("empty IN list: %v %v", empty, err)
		}
		if _, err := momo.Select[suite.Document](db).Where(suite.DocTenantID.Eq(99)).One(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("One on zero rows = %v, want ErrNotFound", err)
		}
	})
}
