// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
)

// TestNestedRelationPredicateAliases pins the SQL of relation predicates
// nested inside one another: every level has its own child alias and
// correlates on the alias of the level that encloses it.
func TestNestedRelationPredicateAliases(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))

	docs := func(p momo.Predicate) (momo.Compiled, error) {
		return momo.Select[models.Document, *models.Document](db).Where(p).SQL()
	}
	authors := func(p momo.Predicate) (momo.Compiled, error) {
		return momo.Select[models.Author, *models.Author](db).Where(p).SQL()
	}

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
	}{
		{"two levels", func() (momo.Compiled, error) {
			return docs(document.Taggings.Has(tagging.Tag.Has(tag.Name.As("momo_r").Eq("go"))))
		}, `EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id" AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id" AND ("momo_r2"."name" = $1))))`},
		{"two levels with an explicit parent alias", func() (momo.Compiled, error) {
			return docs(document.Taggings.Has(tagging.Tag.HasFrom("momo_r", tag.Name.As("momo_r").Eq("go"))))
		}, `EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id" AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id" AND ("momo_r2"."name" = $1))))`},
		{"three levels", func() (momo.Compiled, error) {
			return authors(author.FirstDocument.Has(document.Taggings.Has(tagging.Tag.Has(tag.Name.As("momo_r").Eq("db")))))
		}, `EXISTS (SELECT 1 FROM "documents" AS "momo_r" WHERE "momo_r"."id" = "authors"."id" AND (EXISTS (SELECT 1 FROM "taggings" AS "momo_r2" WHERE "momo_r2"."document_id" = "momo_r"."id" AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r3" WHERE "momo_r3"."id" = "momo_r2"."tag_id" AND ("momo_r3"."name" = $1))))))`},
		{"HasNo outside", func() (momo.Compiled, error) {
			return docs(document.Taggings.HasNo(tagging.Tag.Has(tag.Name.As("momo_r").Eq("go"))))
		}, `NOT EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id" AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id" AND ("momo_r2"."name" = $1))))`},
		{"HasNo inside", func() (momo.Compiled, error) {
			return docs(document.Taggings.Has(tagging.Tag.HasNo(tag.Name.As("momo_r").Eq("go"))))
		}, `EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id" AND (NOT EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id" AND ("momo_r2"."name" = $1))))`},
		{"many-to-many nested gets its own join alias", func() (momo.Compiled, error) {
			return authors(author.FirstDocument.Has(document.Tags.Has(tag.Name.As("momo_r").Eq("go"))))
		}, `EXISTS (SELECT 1 FROM "documents" AS "momo_r" WHERE "momo_r"."id" = "authors"."id" AND (EXISTS (SELECT 1 FROM "taggings" AS "momo_j2" INNER JOIN "tags" AS "momo_r2" ON "momo_r2"."id" = "momo_j2"."tag_id" WHERE "momo_j2"."document_id" = "momo_r"."id" AND ("momo_r2"."name" = $1))))`},
		{"a top-level relation keeps the unnumbered aliases", func() (momo.Compiled, error) {
			return docs(document.Tags.Has(tag.Name.As("momo_r").Eq("go")))
		}, `EXISTS (SELECT 1 FROM "taggings" AS "momo_j" INNER JOIN "tags" AS "momo_r" ON "momo_r"."id" = "momo_j"."tag_id" WHERE "momo_j"."document_id" = "documents"."id" AND ("momo_r"."name" = $1))`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := tc.q()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(c.SQL, tc.want) {
				t.Fatalf("SQL does not contain\n  %s\ngot\n  %s", tc.want, c.SQL)
			}
		})
	}
}
