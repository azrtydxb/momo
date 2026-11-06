// SPDX-License-Identifier: Apache-2.0

// Command relations shows how to declare relations (see models/models.go),
// load them without N+1 queries, filter by them, change them and write whole
// graphs.
//
// Run it against a database that has the migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/relations
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/relations/models"
	"github.com/azrtydxb/momo/examples/relations/models/author"
	"github.com/azrtydxb/momo/examples/relations/models/comment"
	"github.com/azrtydxb/momo/examples/relations/models/document"
	"github.com/azrtydxb/momo/examples/relations/models/tag"
)

func main() {
	if err := start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func start(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	return run(ctx, momo.New(pgxdriver.New(pool)), os.Stdout)
}

// run walks through every step and prints what it finds. The test compares
// the output, so the printed lines are the contract of this example.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	steps := []func(context.Context, momo.Querier, io.Writer) error{
		writeGraphs,
		loadWithScope,
		preloadAndJoin,
		filterByRelation,
		changeAssociations,
	}
	for _, step := range steps {
		if err := step(ctx, db, out); err != nil {
			return err
		}
	}
	return nil
}

// writeGraphs creates rows together with their relations.
//
// FindOrCreate is get-or-insert on a unique key, safe against concurrent
// callers. CreateGraph inserts a parent plus the children of the relations
// you list with Include, in one transaction, and copies the generated keys
// into the children for you. Nothing is saved that you did not list.
func writeGraphs(ctx context.Context, q momo.Querier, out io.Writer) error {
	ada := models.Author{Name: "Ada"}
	created, err := momo.FindOrCreate(ctx, q, []momo.ColumnRef{author.Name}, &ada)
	if err != nil {
		return err
	}
	again := models.Author{Name: "Ada"}
	createdAgain, err := momo.FindOrCreate(ctx, q, []momo.ColumnRef{author.Name}, &again)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "find or create: first=%t second=%t same row=%t\n", created, createdAgain, ada.ID == again.ID)

	engines := models.Document{
		AuthorID: &ada.ID,
		Title:    "Engines",
		Comments: []models.Comment{
			{Body: "first", Approved: true},
			{Body: "second", Approved: false},
			{Body: "third", Approved: true},
			{Body: "fourth", Approved: true},
		},
		Tags: []models.Tag{{Name: "go"}, {Name: "orm"}},
	}
	if err := momo.CreateGraph(ctx, q, &engines, document.Comments.Include(), document.Tags.Include()); err != nil {
		return err
	}

	// Insert is enough for a row without children.
	quiet := models.Document{AuthorID: &ada.ID, Title: "Quiet"}
	if _, err := momo.Insert[models.Document](q).Model(&quiet).Exec(ctx); err != nil {
		return err
	}

	// A belongs_to child listed in the graph is inserted first and its key is
	// copied into the parent: the new author and the document in one go.
	note := models.Document{
		Title:    "Note",
		Author:   &models.Author{Name: "Grace"},
		Comments: []models.Comment{{Body: "nice", Approved: true}},
	}
	if err := momo.CreateGraph(ctx, q, &note, document.Author.Include(), document.Comments.Include()); err != nil {
		return err
	}
	fmt.Fprintf(out, "graph: note.AuthorID set=%t, comment.DocumentID matches=%t\n",
		note.AuthorID != nil && *note.AuthorID == note.Author.ID, note.Comments[0].DocumentID == note.ID)
	return nil
}

func allDocuments(ctx context.Context, q momo.Querier) ([]models.Document, error) {
	return momo.Select[models.Document](q).OrderBy(document.ID.Asc()).All(ctx)
}

// loadWithScope is eager loading: one statement per relation for all
// parents, never one per parent. Load takes load options that shape the
// child statement; LoadAll loads several relations, and Relation.Scope
// carries the options of one of them.
func loadWithScope(ctx context.Context, q momo.Querier, out io.Writer) error {
	docs, err := allDocuments(ctx, q)
	if err != nil {
		return err
	}

	// The two newest approved comments of every document. LoadWhere filters
	// first, then LoadPerParentLimit keeps the top n per parent.
	if err := momo.Load(ctx, q, docs, document.Comments,
		momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
		momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
		momo.LoadPerParentLimit[models.Comment](2),
	); err != nil {
		return err
	}
	for _, d := range docs {
		fmt.Fprintf(out, "load %-7s comments=%s\n", d.Title, bodies(d.Comments))
	}

	// Several relations at once; Scope is how a relation passed to LoadAll
	// gets its options. A has_many load clears the field first, so a parent
	// with no children ends up with an empty slice, never stale data.
	if err := momo.LoadAll(ctx, q, docs,
		document.Author,
		document.Tags,
		document.Comments.Scope(momo.LoadPerParentLimit[models.Comment](1)),
	); err != nil {
		return err
	}
	for _, d := range docs {
		fmt.Fprintf(out, "loadall %-5s by %s, tags=%s, comments=%d\n", d.Title, authorName(d), tagNames(d.Tags), len(d.Comments))
	}
	return nil
}

// preloadAndJoin attaches the loaders to the query itself, so the result is
// complete when All returns. Joined folds a belongs_to into the parent
// statement as a LEFT JOIN: no second round trip.
func preloadAndJoin(ctx context.Context, q momo.Querier, out io.Writer) error {
	docs, err := momo.Select[models.Document](q).
		Preload(
			document.Author.Joined(),
			document.Comments.Scope(momo.LoadPerParentLimit[models.Comment](1)),
		).
		OrderBy(document.ID.Asc()).
		All(ctx)
	if err != nil {
		return err
	}
	for _, d := range docs {
		fmt.Fprintf(out, "preload %-6s by %s, first comment=%s\n", d.Title, authorName(d), bodies(d.Comments))
	}
	return nil
}

// filterByRelation filters and orders parents by their children without
// loading them. Has and HasNo render EXISTS and NOT EXISTS subqueries, so
// they never duplicate parent rows the way a join would. Inside Has the child
// table is named momo_r: build its predicates with As("momo_r").
func filterByRelation(ctx context.Context, q momo.Querier, out io.Writer) error {
	titles := func(label string, sel momo.SelectQuery[models.Document]) error {
		docs, err := sel.All(ctx)
		if err != nil {
			return err
		}
		names := make([]string, len(docs))
		for i, d := range docs {
			names[i] = d.Title
		}
		fmt.Fprintf(out, "%s: %s\n", label, strings.Join(names, ", "))
		return nil
	}
	docs := momo.Select[models.Document](q)
	if err := titles("has an unapproved comment",
		docs.Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(false)))); err != nil {
		return err
	}
	if err := titles("has no comments", docs.Where(document.Comments.HasNo())); err != nil {
		return err
	}
	return titles("most commented first",
		docs.OrderBy(document.Comments.CountOrder(momo.DirDesc), document.ID.Asc()))
}

// changeAssociations edits the link itself, with no models in between. Link
// adds pairs (existing pairs are skipped), Unlink removes some, Replace makes
// the given list the whole set by diffing against the database, and Count
// counts. These write SQL directly: model hooks do not run.
func changeAssociations(ctx context.Context, q momo.Querier, out io.Writer) error {
	quiet, err := momo.Select[models.Document](q).Where(document.Title.Eq("Quiet")).One(ctx)
	if err != nil {
		return err
	}
	tags, err := momo.Select[models.Tag](q).OrderBy(tag.Name.Asc()).All(ctx)
	if err != nil {
		return err
	}
	goTag, ormTag := &tags[0], &tags[1]

	count := func(label string) error {
		n, err := document.Tags.Count(ctx, q, &quiet)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %d tags\n", label, n)
		return nil
	}
	if err := document.Tags.Link(ctx, q, &quiet, goTag, ormTag); err != nil {
		return err
	}
	if err := count("link go+orm"); err != nil {
		return err
	}
	if err := document.Tags.Unlink(ctx, q, &quiet, goTag); err != nil {
		return err
	}
	if err := count("unlink go"); err != nil {
		return err
	}
	if err := document.Tags.Replace(ctx, q, &quiet, goTag); err != nil {
		return err
	}
	return count("replace with go")
}

func bodies(cs []models.Comment) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.Body
	}
	return "[" + strings.Join(names, " ") + "]"
}

func tagNames(ts []models.Tag) string {
	names := make([]string, len(ts))
	for i, t := range ts {
		names[i] = t.Name
	}
	return "[" + strings.Join(names, " ") + "]"
}

// authorName reads a loaded belongs_to. It is nil when the foreign key is
// NULL or the relation was not loaded.
func authorName(d models.Document) string {
	if d.Author == nil {
		return "nobody"
	}
	return d.Author.Name
}
