// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/region"
)

func TestAssociationHelpers(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Tags of document 2: none. Link tag 1 (one statement).
		p := &models.Document{ID: 2}
		tag := &models.Tag{ID: 1}
		h.After = nil
		if err := document.Tags.Link(ctx, db, p, tag); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Link must issue exactly one statement, got %d", len(h.After))
		}
		h.After = nil
		n, err := document.Tags.Count(ctx, db, p)
		if err != nil || n != 1 {
			t.Fatalf("Count after Link: n=%d err=%v", n, err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Count must issue exactly one statement, got %d", len(h.After))
		}

		// Linking the same pair again is a no-op, not a duplicate or an error.
		if err := document.Tags.Link(ctx, db, p, tag); err != nil {
			t.Fatal(err)
		}
		if n, _ = document.Tags.Count(ctx, db, p); n != 1 {
			t.Fatalf("relinking must not duplicate the join row: count=%d", n)
		}

		// Replace with {2}: tag 1 removed, tag 2 added, inside one transaction.
		h.After = nil
		if err := document.Tags.Replace(ctx, db, p, &models.Tag{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if got := len(h.After); got != 5+lockEvents(db) { // BEGIN, [lock], select, clear, link, COMMIT
			t.Fatalf("Replace must be one transaction (select, clear, link), got %d events", got)
		}
		n, err = document.Tags.Count(ctx, db, p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("Replace must leave exactly the new set: count=%d", n)
		}
		ids := tagIDs(t, db, 2)
		if len(ids) != 1 || ids[0] != 2 {
			t.Fatalf("Replace kept the wrong tag: %v", ids)
		}
		// Other documents are untouched.
		if ids := tagIDs(t, db, 1); len(ids) != 2 {
			t.Fatalf("document 1 tags changed: %v", ids)
		}

		boss := &models.Employee{ID: 1}
		h.After = nil
		if err := employee.Reports.Unlink(ctx, db, boss, &models.Employee{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Unlink must issue exactly one statement, got %d", len(h.After))
		}
		n, err = employee.Reports.Count(ctx, db, boss)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("Unlink must NULL only the listed child FK: count=%d", n)
		}
	})
}

func tagIDs(t *testing.T, db *momo.DB, doc int64) []int64 {
	t.Helper()
	ids, err := momo.Scalars[int64](context.Background(), db,
		`SELECT tag_id FROM taggings WHERE document_id = ? ORDER BY tag_id`, doc)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestAssociationManyToManyUnlinkAndReplaceEmpty(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 1}

		// Unlink removes only the listed pair; other documents' pairs stay.
		h.After = nil
		if err := document.Tags.Unlink(ctx, db, doc, &models.Tag{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Unlink must issue one statement, got %d", len(h.After))
		}
		if ids := tagIDs(t, db, 1); len(ids) != 1 || ids[0] != 1 {
			t.Fatalf("Unlink removed the wrong rows: %v", ids)
		}
		if ids := tagIDs(t, db, 3); len(ids) != 1 {
			t.Fatalf("Unlink touched document 3: %v", ids)
		}

		// Replace with nothing clears the set and skips the link statement.
		h.After = nil
		if err := document.Tags.Replace(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		if got := len(h.After); got != 3+lockEvents(db) { // BEGIN, [lock], clear, COMMIT
			t.Fatalf("Replace(empty) must issue one statement in a transaction, got %d events", got)
		}
		if n, _ := document.Tags.Count(ctx, db, doc); n != 0 {
			t.Fatalf("count after Replace(empty) = %d", n)
		}
		if ids := tagIDs(t, db, 3); len(ids) != 1 {
			t.Fatalf("Replace touched document 3: %v", ids)
		}

		// No children: no statement.
		h.After = nil
		if err := document.Tags.Link(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		if err := document.Tags.Unlink(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 0 {
			t.Fatalf("empty Link/Unlink issued %d statements", len(h.After))
		}
	})
}

func TestAssociationHasManyReplaceInTx(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		boss := &models.Employee{ID: 1}

		// Inside a *Tx Replace uses a savepoint and the outer tx stays open.
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := employee.Reports.Replace(ctx, tx, boss, &models.Employee{ID: 3}); err != nil {
				return err
			}
			n, err := employee.Reports.Count(ctx, tx, boss)
			if err != nil {
				return err
			}
			if n != 1 {
				t.Errorf("count in tx = %d, want 1", n)
			}
			return errors.New("rollback")
		})
		if err == nil {
			t.Fatal("expected rollback error")
		}
		if n, _ := employee.Reports.Count(ctx, db, boss); n != 2 {
			t.Fatalf("outer rollback must undo Replace: count=%d", n)
		}

		// On a *DB: dev1 (2) leaves, dev2 (3) stays, employee 1 itself is linked.
		h.After = nil
		if err := employee.Reports.Replace(ctx, db, boss, &models.Employee{ID: 3}); err != nil {
			t.Fatal(err)
		}
		if got := len(h.After); got != 4+lockEvents(db) {
			t.Fatalf("Replace events = %d, want %d", got, 4+lockEvents(db))
		}
		if n, _ := employee.Reports.Count(ctx, db, boss); n != 1 {
			t.Fatalf("Replace must leave one report: %d", n)
		}
		mgr, err := momo.Scalars[int64](ctx, db, `SELECT id FROM employees WHERE manager_id = ? ORDER BY id`, int64(1))
		if err != nil || len(mgr) != 1 || mgr[0] != 3 {
			t.Fatalf("reports = %v err=%v", mgr, err)
		}

		// Link sets the FK of exactly the listed child.
		h.After = nil
		if err := employee.Reports.Link(ctx, db, boss, &models.Employee{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Link statements = %d, want 1", len(h.After))
		}
		if n, _ := employee.Reports.Count(ctx, db, boss); n != 2 {
			t.Fatalf("count after Link = %d", n)
		}

		// Unlink does not clear a child that belongs to another parent.
		other := &models.Employee{ID: 2}
		if err := employee.Reports.Unlink(ctx, db, other, &models.Employee{ID: 3}); err != nil {
			t.Fatal(err)
		}
		if n, _ := employee.Reports.Count(ctx, db, boss); n != 2 {
			t.Fatalf("Unlink under the wrong parent changed rows: count=%d", n)
		}
	})
}

func TestAssociationPolymorphicAndComposite(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Link writes the discriminator with the key: image 2 belongs to author 1
		// and becomes document 2's image.
		doc := &models.Document{ID: 2}
		if err := document.Images.Link(ctx, db, doc, &models.Image{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if n, err := document.Images.Count(ctx, db, doc); err != nil || n != 1 {
			t.Fatalf("polymorphic count: n=%d err=%v", n, err)
		}
		types, err := momo.Scalars[string](ctx, db, `SELECT owner_type FROM images WHERE id = ?`, int64(2))
		if err != nil || len(types) != 1 || types[0] != "document" {
			t.Fatalf("discriminator not written: %v err=%v", types, err)
		}
		// Document 1 still has its own image; the count is scoped by discriminator.
		if n, _ := document.Images.Count(ctx, db, &models.Document{ID: 1}); n != 1 {
			t.Fatalf("document 1 images = %d", n)
		}

		// Composite key: store 3 (no region) joins region (1, EU).
		eu := &models.Region{TenantID: 1, Code: "EU"}
		if n, _ := region.Stores.Count(ctx, db, eu); n != 1 {
			t.Fatalf("EU stores before = %d", n)
		}
		h.After = nil
		if err := region.Stores.Link(ctx, db, eu, &models.Store{ID: 3}); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("composite Link statements = %d", len(h.After))
		}
		if n, _ := region.Stores.Count(ctx, db, eu); n != 2 {
			t.Fatalf("EU stores after = %d", n)
		}
		// Tenant 2's EU region is a different key.
		if n, _ := region.Stores.Count(ctx, db, &models.Region{TenantID: 2, Code: "EU"}); n != 1 {
			t.Fatalf("tenant 2 EU stores = %d", n)
		}
	})
}

func TestAssociationBelongsToUnsupported(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		d := &models.Document{ID: 1}
		a := &models.Author{ID: 1}
		h.After = nil
		errs := map[string]error{
			"link":    document.Author.Link(ctx, db, d, a),
			"unlink":  document.Author.Unlink(ctx, db, d, a),
			"replace": document.Author.Replace(ctx, db, d, a),
			"empty":   document.Author.Link(ctx, db, d),
		}
		_, errs["count"] = document.Author.Count(ctx, db, d)
		for name, err := range errs {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Errorf("%s on BelongsTo: %v, want ErrUnsupported", name, err)
			}
		}
		if len(h.After) != 0 {
			t.Fatalf("BelongsTo helpers issued %d statements", len(h.After))
		}
	})
}

func TestAssociationNullKeys(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		// Store 3 has a NULL region code, so the reverse key of a Store-as-parent
		// would be NULL; use a parent whose key function reports NULL.
		rel := region.Stores
		rel.ParentKey = func(*models.Region) ([]any, bool) { return nil, false }
		h.After = nil
		if n, err := rel.Count(ctx, db, &models.Region{}); err != nil || n != 0 {
			t.Fatalf("NULL parent count: n=%d err=%v", n, err)
		}
		if err := rel.Link(ctx, db, &models.Region{}, &models.Store{ID: 1}); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Link with NULL parent key: %v", err)
		}
		if err := rel.Replace(ctx, db, &models.Region{}); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Replace with NULL parent key: %v", err)
		}
		tags := document.Tags
		tags.ChildKey = func(*models.Tag) ([]any, bool) { return nil, false }
		if err := tags.Link(ctx, db, &models.Document{ID: 1}, &models.Tag{ID: 1}); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("Link with NULL child key: %v", err)
		}
		if len(h.After) != 0 {
			t.Fatalf("NULL keys issued %d statements", len(h.After))
		}
	})
}
