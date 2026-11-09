// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/note"
	"github.com/azrtydxb/momo/internal/relfixture/region"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// graphCount returns the single integer a COUNT(*) statement selects.
func graphCount(t *testing.T, q momo.Querier, sql string, args ...any) int64 {
	t.Helper()
	out, err := momo.Scalars[int64](context.Background(), q, sql, args...)
	if err != nil || len(out) != 1 {
		t.Fatalf("count %q: %v %v", sql, out, err)
	}
	return out[0]
}

func TestCreateGraphAndFindOrCreate(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 10, Title: "new", Comments: []models.Comment{{ID: 20, Body: "hi", Approved: true}}, Tags: []models.Tag{{ID: 3, Name: "x"}}}
		// Only Comments is listed: Tags must not be written.
		if err := momo.CreateGraph(ctx, db, doc, document.Comments.Include()); err != nil {
			t.Fatal(err)
		}
		n, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.ID.Eq(20)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("listed child not written: %d", n)
		}
		got, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.ID.Eq(20)).One(ctx)
		if err != nil || got.DocumentID != 10 {
			t.Fatalf("child foreign key must be the parent's key: %+v err=%v", got, err)
		}
		tags, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(3)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tags != 0 {
			t.Fatal("unlisted relation was written")
		}

		// FindOrCreate race: N goroutines create the same key; exactly one creates.
		var wg sync.WaitGroup
		var mu sync.Mutex
		created := 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m := &models.Tag{ID: 99, Name: "race"}
				c, err := momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{tag.ID}, m)
				if err != nil {
					t.Error(err)
					return
				}
				if c {
					mu.Lock()
					created++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if created != 1 {
			t.Fatalf("exactly one creator expected, got %d", created)
		}
	})
}

func TestFindOrCreateReturnsStoredRow(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		m := &models.Tag{ID: 1, Name: "ignored"}
		created, err := momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{tag.ID}, m)
		if err != nil || created {
			t.Fatalf("existing key: created=%v err=%v", created, err)
		}
		if m.Name != "go" {
			t.Fatalf("m must hold the stored row, got %+v", m)
		}
		// A key column the model does not map is a validation error, not SQL.
		_, err = momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{comment.Body}, &models.Tag{ID: 5})
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("foreign key column: %v", err)
		}
		if _, err = momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, nil, &models.Tag{ID: 5}); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("empty key: %v", err)
		}
	})
}

func TestUpsertByPK(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		if err := momo.UpsertByPK[models.Tag, *models.Tag](ctx, db, &models.Tag{ID: 1, Name: "golang"}); err != nil {
			t.Fatal(err)
		}
		tg, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(1)).One(ctx)
		if err != nil || tg.Name != "golang" {
			t.Fatalf("upsert must update every non-key column: %+v err=%v", tg, err)
		}
		// A new key inserts.
		if err := momo.UpsertByPK[models.Tag, *models.Tag](ctx, db, &models.Tag{ID: 7, Name: "new"}); err != nil {
			t.Fatal(err)
		}
		n, err := momo.Select[models.Tag, *models.Tag](db).Count(ctx)
		if err != nil || n != 3 {
			t.Fatalf("tags after upsert: %d err=%v", n, err)
		}
	})
}

func TestCreateGraphBelongsToFirstAndManyToMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		a := models.Author{ID: 50, Name: "Linus"}
		doc := &models.Document{
			ID: 60, Title: "kernel", Author: &a,
			Tags:     []models.Tag{{ID: 5, Name: "c"}, {ID: 6, Name: "os"}},
			Comments: []models.Comment{{ID: 70, Body: "unlisted"}},
		}
		if err := momo.CreateGraph(ctx, db, doc, document.Author.Include(), document.Tags.Include()); err != nil {
			t.Fatal(err)
		}
		if doc.AuthorID == nil || *doc.AuthorID != 50 {
			t.Fatalf("belongs-to key must be copied into the parent: %v", doc.AuthorID)
		}
		got, err := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(60)).One(ctx)
		if err != nil || got.AuthorID == nil || *got.AuthorID != 50 {
			t.Fatalf("stored document: %+v err=%v", got, err)
		}
		n, err := document.Tags.Count(ctx, db, doc)
		if err != nil || n != 2 {
			t.Fatalf("join rows: %d err=%v", n, err)
		}
		nc, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.ID.Eq(70)).Count(ctx)
		if err != nil || nc != 0 {
			t.Fatalf("unlisted comment written: %d err=%v", nc, err)
		}
	})
}

func TestCreateGraphPolymorphicCompositeAndSelfReference(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 61, Title: "pics", Images: []models.Image{{ID: 80, URL: "a"}, {ID: 81, URL: "b"}}}
		if err := momo.CreateGraph(ctx, db, doc, document.Images.Include()); err != nil {
			t.Fatal(err)
		}
		imgs, err := document.Images.Count(ctx, db, doc)
		if err != nil || imgs != 2 {
			t.Fatalf("polymorphic children: %d err=%v", imgs, err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM images WHERE owner_type = 'document' AND owner_id = 61`) != 2 {
			t.Fatal("discriminator not written")
		}

		reg := &models.Region{TenantID: 3, Code: "AP", Name: "Asia", Stores: []models.Store{{ID: 90, Name: "Tokyo"}}}
		if err := momo.CreateGraph(ctx, db, reg, region.Stores.Include()); err != nil {
			t.Fatal(err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM stores WHERE id = 90 AND tenant_id = 3 AND region_code = 'AP'`) != 1 {
			t.Fatal("composite key not propagated to the child")
		}

		boss := &models.Employee{ID: 10, Name: "ceo", Reports: []models.Employee{{ID: 11, Name: "r1"}, {ID: 12, Name: "r2"}}}
		if err := momo.CreateGraph(ctx, db, boss, employee.Reports.Include()); err != nil {
			t.Fatal(err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM employees WHERE manager_id = 10`) != 2 {
			t.Fatal("self-referencing children not linked")
		}
		dev := &models.Employee{ID: 13, Name: "dev", Manager: &models.Employee{ID: 14, Name: "mgr"}}
		if err := momo.CreateGraph(ctx, db, dev, employee.Manager.Include()); err != nil {
			t.Fatal(err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM employees WHERE id = 13 AND manager_id = 14`) != 1 {
			t.Fatal("self-referencing parent not linked")
		}

		// A model listing itself as its own child is rejected before any write.
		loop := &models.Employee{ID: 15, Name: "loop"}
		loop.Manager = loop
		err = momo.CreateGraph(ctx, db, loop, employee.Manager.Include())
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("cycle: %v", err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM employees WHERE id = 15`) != 0 {
			t.Fatal("cycle wrote a row")
		}
	})
}

func TestCreateGraphRollsBackWholeGraph(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// The second comment collides with seeded comment 1: the parent and the
		// first comment must not survive.
		doc := &models.Document{ID: 62, Title: "bad", Comments: []models.Comment{{ID: 91, Body: "ok"}, {ID: 1, Body: "dup"}}}
		err := momo.CreateGraph(ctx, db, doc, document.Comments.Include())
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("want unique violation, got %v", err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM documents WHERE id = 62`)+graphCount(t, db, `SELECT COUNT(*) FROM comments WHERE id = 91`) != 0 {
			t.Fatal("graph is not atomic")
		}

		// On a *Tx the graph runs in a savepoint: the failure undoes the graph,
		// the caller's earlier work stays and the transaction is still usable.
		err = db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := momo.UpsertByPK[models.Tag, *models.Tag](ctx, tx, &models.Tag{ID: 40, Name: "kept"}); err != nil {
				return err
			}
			bad := &models.Document{ID: 63, Title: "bad2", Comments: []models.Comment{{ID: 1, Body: "dup"}}}
			if err := momo.CreateGraph(ctx, tx, bad, document.Comments.Include()); !errors.Is(err, momo.ErrUniqueViolation) {
				t.Errorf("want unique violation, got %v", err)
			}
			good := &models.Document{ID: 64, Title: "good", Comments: []models.Comment{{ID: 92, Body: "ok"}}}
			return momo.CreateGraph(ctx, tx, good, document.Comments.Include())
		})
		if err != nil {
			t.Fatal(err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM documents WHERE id = 63`) != 0 ||
			graphCount(t, db, `SELECT COUNT(*) FROM documents WHERE id = 64`) != 1 ||
			graphCount(t, db, `SELECT COUNT(*) FROM tags WHERE id = 40`) != 1 {
			t.Fatal("savepoint did not isolate the failed graph")
		}
	})
}

func TestCreateGraphRunsFieldHooksOnChildren(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		relfixture.CreateSchema(t, db)
		doc := &models.Document{ID: 65, Title: "t", Comments: []models.Comment{{ID: 93, Body: ""}}}
		err := momo.CreateGraph(context.Background(), db, doc, document.Comments.Include())
		if !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("child field hook must run: %v", err)
		}
		if graphCount(t, db, `SELECT COUNT(*) FROM documents WHERE id = 65`) != 0 {
			t.Fatal("parent survived a failing child validator")
		}
	}, momo.WithHook(h), momo.WithFieldHooks[models.Comment, *models.Comment]([]momo.FieldHook[models.Comment]{{
		Column: 2, // body
		Validate: func(v any) error {
			if s, _ := v.(string); s == "" {
				return errors.New("body required")
			}
			return nil
		},
	}}))
}

// A soft-deleted row is not "found": FindOrCreate fails with
// ErrUniqueViolation, leaves the deleted row as it was and creates no second one.
func TestFindOrCreateSoftDeletedRowIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for _, s := range []struct {
			id      int64
			deleted bool
		}{{1, true}, {2, false}} {
			if _, err := momo.Exec(ctx, db, `INSERT INTO notes (id, document_id, body, deleted) VALUES (?, 1, ?, ?)`, s.id, "old", s.deleted); err != nil {
				t.Fatal(err)
			}
		}
		key := []momo.ColumnRef{note.ID}

		m := &models.Note{ID: 1, DocumentID: 1, Body: "new"}
		created, err := momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, m)
		if created || !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("soft-deleted match: created=%v err=%v, want ErrUniqueViolation", created, err)
		}
		if m.Body != "new" || m.Deleted {
			t.Fatalf("m must not be overwritten by the deleted row: %+v", m)
		}
		rows, err := momo.Select[models.Note, *models.Note](db).WithDeleted().Where(note.ID.Eq(1)).All(ctx)
		if err != nil || len(rows) != 1 || !rows[0].Deleted || rows[0].Body != "old" {
			t.Fatalf("the deleted row must stay as it was: %+v err=%v", rows, err)
		}

		// A live row is found, and an unused key is created.
		live := &models.Note{ID: 2, DocumentID: 1, Body: "ignored"}
		if created, err = momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, live); err != nil || created || live.Body != "old" {
			t.Fatalf("live row: created=%v err=%v %+v", created, err, live)
		}
		fresh := &models.Note{ID: 3, DocumentID: 1, Body: "fresh"}
		if created, err = momo.FindOrCreate[models.Note, *models.Note](ctx, db, key, fresh); err != nil || !created {
			t.Fatalf("unused key: created=%v err=%v", created, err)
		}
	})
}

// A many-to-many child that is already stored is inserted again, not linked:
// the graph fails with ErrUniqueViolation and is undone. Link is the way to
// associate existing children.
func TestCreateGraphPersistedManyToManyChildIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 20, Title: "graph", Tags: []models.Tag{{ID: 1, Name: "go"}}}
		err := momo.CreateGraph(ctx, db, doc, document.Tags.Include())
		if !errors.Is(err, momo.ErrUniqueViolation) {
			t.Fatalf("persisted many-to-many child: want ErrUniqueViolation, got %v", err)
		}
		n, err := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(20)).Count(ctx)
		if err != nil || n != 0 {
			t.Fatalf("the failed graph must be undone: %d documents, err=%v", n, err)
		}

		// Link associates the stored tag with a stored document.
		doc = &models.Document{ID: 20, Title: "graph"}
		if err := momo.CreateGraph(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		existing := &models.Tag{ID: 1, Name: "go"}
		if err := document.Tags.Link(ctx, db, doc, existing); err != nil {
			t.Fatal(err)
		}
		if n, err := document.Tags.Count(ctx, db, doc); err != nil || n != 1 {
			t.Fatalf("Link: %d join rows, err=%v", n, err)
		}
		if c, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(1)).Count(ctx); err != nil || c != 1 {
			t.Fatalf("Link must not insert the tag again: %d, err=%v", c, err)
		}
	})
}

// 1000 children are written and read back through the bulk path.
func TestCreateGraphThousandChildrenIntegration(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 30, Title: "big", Comments: make([]models.Comment, 1000)}
		for i := range doc.Comments {
			doc.Comments[i] = models.Comment{ID: int64(1000 + i), Body: "b", Approved: i%2 == 0}
		}
		if err := momo.CreateGraph(ctx, db, doc, document.Comments.Include()); err != nil {
			t.Fatal(err)
		}
		n, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.DocumentID.Eq(30)).Count(ctx)
		if err != nil || n != 1000 {
			t.Fatalf("stored children: %d, err=%v", n, err)
		}
		if doc.Comments[999].DocumentID != 30 {
			t.Fatalf("child key not written back: %+v", doc.Comments[999])
		}
	})
}
