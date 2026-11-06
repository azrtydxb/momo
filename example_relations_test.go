// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

// These examples back docs/relations.md. The ones without an Output comment
// need a database and are compiled only, so the snippets cannot drift from
// the API.

// Has filters parents by a correlated EXISTS subquery. Scope predicates name
// the child through the alias "momo_r".
func ExampleRelation_Has() {
	db := momo.New(pgxdriver.New(nil))
	c, err := momo.Select[models.Document](db).
		Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(true))).
		OrderBy(document.Comments.CountOrder(momo.DirDesc)).
		SQL()
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(c.SQL != "")
	// Output: true
}

// Load fills an association for a slice of parents with one statement per
// bind-limit chunk. Options scope the child statement.
func ExampleLoad() {
	var (
		ctx  = context.Background()
		db   = momo.New(pgxdriver.New(nil))
		docs []models.Document
	)
	err := momo.Load(ctx, db, docs, document.Comments,
		momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
		momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
		momo.LoadPerParentLimit[models.Comment](3),
	)
	fmt.Println(err)
}

// LoadAll loads several relations together; Then nests a load on the children.
func ExampleLoadAll() {
	var (
		ctx  = context.Background()
		db   = momo.New(pgxdriver.New(nil))
		docs []models.Document
	)
	err := momo.LoadAll(ctx, db, docs,
		document.Author,
		document.Tags,
		document.Comments,
	)
	fmt.Println(err)
}

// Preload runs the loads after the query's rows are read; Joined fetches a
// belongs-to relation in the parent statement itself.
func ExampleSelectQuery_Preload() {
	var (
		ctx = context.Background()
		db  = momo.New(pgxdriver.New(nil))
	)
	docs, err := momo.Select[models.Document](db).
		Preload(document.Author.Joined(), document.Comments).
		Limit(20).
		All(ctx)
	fmt.Println(docs, err)
}

// The association helpers write the link between a parent and its children.
func ExampleRelation_Replace() {
	var (
		ctx  = context.Background()
		db   = momo.New(pgxdriver.New(nil))
		doc  = models.Document{ID: 1}
		tags = []*models.Tag{{ID: 1}, {ID: 2}}
	)
	_ = document.Tags.Link(ctx, db, &doc, tags...)
	_ = document.Tags.Unlink(ctx, db, &doc, tags[0])
	err := document.Tags.Replace(ctx, db, &doc, tags[1])
	n, cerr := document.Tags.Count(ctx, db, &doc)
	fmt.Println(err, n, cerr)
}

// CreateGraph inserts a parent and the children of the listed relations in one
// transaction.
func ExampleCreateGraph() {
	var (
		ctx = context.Background()
		db  = momo.New(pgxdriver.New(nil))
		doc = models.Document{
			Title:    "Hello",
			Comments: []models.Comment{{Body: "first"}},
		}
	)
	err := momo.CreateGraph(ctx, db, &doc, document.Comments.Include())
	fmt.Println(err)
}

// FindOrCreate needs a unique key; UpsertByPK inserts or updates by primary key.
func ExampleFindOrCreate() {
	var (
		ctx = context.Background()
		db  = momo.New(pgxdriver.New(nil))
		a   = models.Author{ID: 7, Name: "Ada"}
	)
	created, err := momo.FindOrCreate(ctx, db, []momo.ColumnRef{author.ID}, &a)
	fmt.Println(created, err)
	fmt.Println(momo.UpsertByPK(ctx, db, &a))
}
