// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// obsSeed applies the relation fixture schema and rows to an empty database.
func obsSeed(t testing.TB, db *momo.DB) {
	t.Helper()
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)
}

// obsDocs returns every seeded document ordered by id.
func obsDocs(t testing.TB, db *momo.DB) []models.Document {
	t.Helper()
	docs, err := momo.Select[models.Document](db).OrderBy(document.ID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}
