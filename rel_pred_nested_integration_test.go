// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
)

// TestNestedRelationPredicatesRun executes relation predicates nested inside
// one another. Seed: tag go is on document 1; tag db is on documents 1 and 3;
// author 1 owns document 1 as its first document, author 2 owns document 2.
func TestNestedRelationPredicatesRun(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		docIDsFor := func(p momo.Predicate) string {
			t.Helper()
			got, err := momo.Select[models.Document, *models.Document](db).Where(p).OrderBy(document.ID.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return fmt.Sprint(docIDs(got))
		}
		authorIDsFor := func(p momo.Predicate) string {
			t.Helper()
			got, err := momo.Select[models.Author, *models.Author](db).Where(p).OrderBy(author.ID.Asc()).All(ctx)
			if err != nil {
				t.Fatal(err)
			}
			ids := []int64{}
			for _, a := range got {
				ids = append(ids, a.ID)
			}
			return fmt.Sprint(ids)
		}

		if got := docIDsFor(document.Taggings.Has(tagging.Tag.Has(tag.Name.As("momo_r").Eq("db")))); got != "[1 3]" {
			t.Fatalf("Has in Has (db): %s", got)
		}
		if got := docIDsFor(document.Taggings.Has(tagging.Tag.Has(tag.Name.As("momo_r").Eq("go")))); got != "[1]" {
			t.Fatalf("Has in Has (go): %s", got)
		}
		// A nested Has inside HasNo: documents that carry no tag named go.
		if got := docIDsFor(document.Taggings.HasNo(tagging.Tag.Has(tag.Name.As("momo_r").Eq("go")))); got != "[2 3 4]" {
			t.Fatalf("Has in HasNo: %s", got)
		}
		// A nested HasNo inside Has: documents with a tagging whose tag is not go
		// (document 1 has db as well, document 3 only db).
		if got := docIDsFor(document.Taggings.Has(tagging.Tag.HasNo(tag.Name.As("momo_r").Eq("go")))); got != "[1 3]" {
			t.Fatalf("HasNo in Has: %s", got)
		}
		// Explicit parent alias, as in the original report.
		if got := docIDsFor(document.Taggings.Has(tagging.Tag.HasFrom("momo_r", tag.Name.As("momo_r").Eq("db")))); got != "[1 3]" {
			t.Fatalf("HasFrom in Has: %s", got)
		}
		// Three levels, and a many-to-many level nested in a has-one level.
		if got := authorIDsFor(author.FirstDocument.Has(document.Taggings.Has(tagging.Tag.Has(tag.Name.As("momo_r").Eq("db"))))); got != "[1]" {
			t.Fatalf("three levels: %s", got)
		}
		if got := authorIDsFor(author.FirstDocument.Has(document.Tags.Has(tag.Name.As("momo_r").Eq("go")))); got != "[1]" {
			t.Fatalf("many-to-many in has-one: %s", got)
		}
		if got := authorIDsFor(author.FirstDocument.HasNo(document.Tags.Has(tag.Name.As("momo_r").Eq("go")))); got != "[2]" {
			t.Fatalf("many-to-many in HasNo: %s", got)
		}
	})
}
