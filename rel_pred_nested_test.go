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
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/relfixture/tagging"
)

// tagTaggings is a hand-built has-many from tags to taggings: the fixture has
// no relation under Tag, and the many-to-many nesting case needs one.
var tagTaggings = momo.Relation[models.Tag, models.Tagging]{
	Name: "Taggings", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "tags"}, Child: momo.TableRef{Name: "taggings"},
	ParentMeta: (*models.Tag)(nil).MomoMeta(), ChildMeta: (*models.Tagging)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"tag_id"},
}

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
		{"RelAlias reaches level 1 from level 3", func() (momo.Compiled, error) {
			return authors(author.FirstDocument.Has(document.Taggings.Has(tagging.Tag.Has(
				tag.Name.As("momo_r").Eq("db"), document.ID.As(momo.RelAlias(1)).Eq(1)))))
		}, `AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r3" WHERE "momo_r3"."id" = "momo_r2"."tag_id" AND ("momo_r3"."name" = $1) AND ("momo_r"."id" = $2))))))`},
		{"RelAlias reaches level 2 from level 3", func() (momo.Compiled, error) {
			return authors(author.FirstDocument.Has(document.Taggings.Has(tagging.Tag.Has(
				tagging.Position.As(momo.RelAlias(2)).Eq(1)))))
		}, `AND ("momo_r2"."position" = $1)`},
		{"siblings at one level share the alias", func() (momo.Compiled, error) {
			return docs(momo.And(document.Taggings.Has(), document.Comments.HasNo()))
		}, `(EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id") AND NOT EXISTS (SELECT 1 FROM "comments" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id"))`},
		{"nested Has inside Or and Not", func() (momo.Compiled, error) {
			return docs(momo.Or(document.Taggings.Has(tagging.Tag.Has()), momo.Not(document.Comments.Has())))
		}, `(EXISTS (SELECT 1 FROM "taggings" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id" AND (EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id"))) OR NOT (EXISTS (SELECT 1 FROM "comments" AS "momo_r" WHERE "momo_r"."document_id" = "documents"."id")))`},
		{"Not wrapping a nested Has inside a scope", func() (momo.Compiled, error) {
			return docs(document.Taggings.Has(momo.Not(tagging.Tag.Has())))
		}, `AND (NOT (EXISTS (SELECT 1 FROM "tags" AS "momo_r2" WHERE "momo_r2"."id" = "momo_r"."tag_id"))))`},
		{"many-to-many at level 2 with a nested Has inside", func() (momo.Compiled, error) {
			return authors(author.FirstDocument.Has(document.Tags.Has(tagTaggings.Has(tagging.Position.As("momo_r").Eq(1)))))
		}, `EXISTS (SELECT 1 FROM "documents" AS "momo_r" WHERE "momo_r"."id" = "authors"."id" AND (EXISTS (SELECT 1 FROM "taggings" AS "momo_j2" INNER JOIN "tags" AS "momo_r2" ON "momo_r2"."id" = "momo_j2"."tag_id" WHERE "momo_j2"."document_id" = "momo_r"."id" AND (EXISTS (SELECT 1 FROM "taggings" AS "momo_r3" WHERE "momo_r3"."tag_id" = "momo_r2"."id" AND ("momo_r3"."position" = $1))))))`},
		{"self-reference nests on the enclosing child", func() (momo.Compiled, error) {
			return momo.Select[models.Employee, *models.Employee](db).
				Where(employee.Reports.Has(employee.Reports.Has(employee.Name.As("momo_r").Eq("x")))).SQL()
		}, `EXISTS (SELECT 1 FROM "employees" AS "momo_r" WHERE "momo_r"."manager_id" = "employees"."id" AND (EXISTS (SELECT 1 FROM "employees" AS "momo_r2" WHERE "momo_r2"."manager_id" = "momo_r"."id" AND ("momo_r2"."name" = $1))))`},
		{"self-reference HasFrom picks the outer table", func() (momo.Compiled, error) {
			return momo.Select[models.Employee, *models.Employee](db).
				Where(employee.Reports.Has(employee.Reports.HasFrom("employees", employee.Name.As("momo_r").Eq("x")))).SQL()
		}, `AND (EXISTS (SELECT 1 FROM "employees" AS "momo_r2" WHERE "momo_r2"."manager_id" = "employees"."id" AND ("momo_r2"."name" = $1))))`},
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
