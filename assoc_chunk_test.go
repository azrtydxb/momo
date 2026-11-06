// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// eachSmall is relfixture.Each with a tiny bind limit.
func eachSmall(t *testing.T, maxParams int, fn func(t *testing.T, db *momo.DB, h *relfixture.Hook, d dialect.Name)) {
	t.Helper()
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		h.After, h.Events = nil, nil
		fn(t, db, h, d)
	}, momo.WithHook(h), momo.WithMaxParams(maxParams))
}

func mustExec(t *testing.T, db *momo.DB, sql string, args ...any) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), db, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func tagList(from, to int64) []*models.Tag {
	var out []*models.Tag
	for i := from; i <= to; i++ {
		out = append(out, &models.Tag{ID: i})
	}
	return out
}

func empList(from, to int64) []*models.Employee {
	var out []*models.Employee
	for i := from; i <= to; i++ {
		out = append(out, &models.Employee{ID: i})
	}
	return out
}

func seedEmployees(t *testing.T, db *momo.DB, from, to int64) {
	t.Helper()
	for i := from; i <= to; i++ {
		mustExec(t, db, `INSERT INTO employees (id, manager_id, name) VALUES (?, NULL, ?)`, i, fmt.Sprint("e", i))
	}
}

func ids(t *testing.T, db *momo.DB, sql string, args ...any) []int64 {
	t.Helper()
	out, err := momo.Scalars[int64](context.Background(), db, sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sameIDs(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// With room for four binds a join row (two binds) fits twice per statement, a
// has-many UPDATE (one fk bind) three keys. A *DB wraps several chunks in one
// transaction: BEGIN + chunks + COMMIT.
func TestAssociationChunkedManyToMany(t *testing.T) {
	eachSmall(t, 4, func(t *testing.T, db *momo.DB, h *relfixture.Hook, _ dialect.Name) {
		ctx := context.Background()
		doc := &models.Document{ID: 2}

		h.After = nil
		if err := document.Tags.Link(ctx, db, doc, tagList(10, 14)...); err != nil { // 5 rows / 2 per stmt = 3
			t.Fatal(err)
		}
		if got := len(h.After); got != 3+2 {
			t.Fatalf("Link events = %d, want 5 (BEGIN, 3 chunks, COMMIT)", got)
		}
		if got := tagIDs(t, db, 2); !sameIDs(got, 10, 11, 12, 13, 14) {
			t.Fatalf("Link rows: %v", got)
		}

		// A single chunk needs no transaction.
		h.After = nil
		if err := document.Tags.Link(ctx, db, doc, tagList(15, 16)...); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("one-chunk Link events = %d, want 1", len(h.After))
		}

		// Unlink: parent bind + 3 keys per statement. 7 keys -> 3 chunks.
		h.After = nil
		if err := document.Tags.Unlink(ctx, db, doc, tagList(10, 16)...); err != nil {
			t.Fatal(err)
		}
		if got := len(h.After); got != 3+2 {
			t.Fatalf("Unlink events = %d, want 5", got)
		}
		if got := tagIDs(t, db, 2); len(got) != 0 {
			t.Fatalf("Unlink left rows: %v", got)
		}
		if got := tagIDs(t, db, 1); len(got) != 2 {
			t.Fatalf("Unlink touched document 1: %v", got)
		}

		// Replace diffs against the current set: current {20..24}, want {22..27}.
		if err := document.Tags.Link(ctx, db, doc, tagList(20, 24)...); err != nil {
			t.Fatal(err)
		}
		h.After = nil
		if err := document.Tags.Replace(ctx, db, doc, tagList(22, 27)...); err != nil {
			t.Fatal(err)
		}
		// BEGIN, SELECT, DELETE{20,21} (1 chunk), INSERT{25,26,27} (2 chunks), COMMIT.
		if got := len(h.After); got != 6 {
			t.Fatalf("Replace events = %d, want 6", got)
		}
		if got := tagIDs(t, db, 2); !sameIDs(got, 22, 23, 24, 25, 26, 27) {
			t.Fatalf("Replace rows: %v", got)
		}
		if ids := tagIDs(t, db, 3); len(ids) != 1 {
			t.Fatalf("Replace touched document 3: %v", ids)
		}
	})
}

func TestAssociationChunkedHasMany(t *testing.T) {
	eachSmall(t, 4, func(t *testing.T, db *momo.DB, h *relfixture.Hook, _ dialect.Name) {
		ctx := context.Background()
		seedEmployees(t, db, 10, 16)
		boss := &models.Employee{ID: 1}

		h.After = nil
		if err := employee.Reports.Link(ctx, db, boss, empList(10, 16)...); err != nil { // fk bind + 3 keys: 7 -> 3 chunks
			t.Fatal(err)
		}
		if got := len(h.After); got != 3+2 {
			t.Fatalf("Link events = %d, want 5", got)
		}
		if n, _ := employee.Reports.Count(ctx, db, boss); n != 9 {
			t.Fatalf("count after Link = %d, want 9", n)
		}

		h.After = nil
		if err := employee.Reports.Unlink(ctx, db, boss, empList(10, 13)...); err != nil { // 4 keys -> 2 chunks
			t.Fatal(err)
		}
		if got := len(h.After); got != 2+2 {
			t.Fatalf("Unlink events = %d, want 4", got)
		}
		if got := ids(t, db, `SELECT id FROM employees WHERE manager_id = ? ORDER BY id`, int64(1)); !sameIDs(got, 2, 3, 14, 15, 16) {
			t.Fatalf("reports after Unlink: %v", got)
		}

		// Replace: current {2,3,14,15,16}, want {3,14,20}: unlink 2,15,16; link 20.
		seedEmployees(t, db, 20, 20)
		if err := employee.Reports.Replace(ctx, db, boss, empList(3, 3)[0], &models.Employee{ID: 14}, &models.Employee{ID: 20}); err != nil {
			t.Fatal(err)
		}
		if got := ids(t, db, `SELECT id FROM employees WHERE manager_id = ? ORDER BY id`, int64(1)); !sameIDs(got, 3, 14, 20) {
			t.Fatalf("reports after Replace: %v", got)
		}
	})
}

// A *DB multi-chunk Link is atomic: a failure in a later chunk leaves no rows.
func TestAssociationChunkedLinkIsAtomic(t *testing.T) {
	eachSmall(t, 4, func(t *testing.T, db *momo.DB, h *relfixture.Hook, d dialect.Name) {
		if d == dialect.SQLite {
			t.Skip("ADD CONSTRAINT is not available on SQLite")
		}
		ctx := context.Background()

		// Join table: the second chunk ({12, 999}) fails.
		mustExec(t, db, `ALTER TABLE taggings ADD CONSTRAINT no_tag_999 CHECK (tag_id <> 999)`)
		err := document.Tags.Link(ctx, db, &models.Document{ID: 2}, &models.Tag{ID: 10}, &models.Tag{ID: 11}, &models.Tag{ID: 12}, &models.Tag{ID: 999})
		if !errors.Is(err, momo.ErrCheckViolation) {
			t.Fatalf("Link error = %v, want check violation", err)
		}
		if got := tagIDs(t, db, 2); len(got) != 0 {
			t.Fatalf("partial rows after failed Link: %v", got)
		}

		// Has-many: the second chunk ({13..15}) fails on employee 15.
		seedEmployees(t, db, 10, 15)
		mustExec(t, db, `ALTER TABLE employees ADD CONSTRAINT no_emp_15 CHECK (manager_id IS NULL OR id <> 15)`)
		err = employee.Reports.Link(ctx, db, &models.Employee{ID: 1}, empList(10, 15)...)
		if !errors.Is(err, momo.ErrCheckViolation) {
			t.Fatalf("Link error = %v, want check violation", err)
		}
		if got := ids(t, db, `SELECT id FROM employees WHERE manager_id = ? ORDER BY id`, int64(1)); !sameIDs(got, 2, 3) {
			t.Fatalf("partial rows after failed Link: %v", got)
		}
	})
}

// Replace with more tags than one statement can bind works at the real limit.
func TestAssociationReplaceBeyondBindLimit(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 2}
		const n = 70000
		if err := document.Tags.Replace(ctx, db, doc, tagList(1, n)...); err != nil {
			t.Fatal(err)
		}
		if got, _ := document.Tags.Count(ctx, db, doc); got != n {
			t.Fatalf("count = %d, want %d", got, n)
		}
		// Replace with the second half: removes 35000 rows in one DELETE chunk.
		if err := document.Tags.Replace(ctx, db, doc, tagList(n/2+1, n)...); err != nil {
			t.Fatal(err)
		}
		if got, _ := document.Tags.Count(ctx, db, doc); got != n/2 {
			t.Fatalf("count = %d, want %d", got, n/2)
		}
		if err := document.Tags.Unlink(ctx, db, doc, tagList(1, n)...); err != nil {
			t.Fatal(err)
		}
		if got, _ := document.Tags.Count(ctx, db, doc); got != 0 {
			t.Fatalf("count = %d, want 0", got)
		}
	})
}

// Clearing a NOT NULL foreign key reports what is wrong and keeps the driver error.
func TestAssociationUnlinkNotNullForeignKey(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		doc := &models.Document{ID: 1}
		check := func(name string, err error) {
			t.Helper()
			if !errors.Is(err, momo.ErrNotNullViolation) {
				t.Fatalf("%s: %v, want ErrNotNullViolation", name, err)
			}
			if !strings.Contains(err.Error(), "nullable foreign key") {
				t.Fatalf("%s: message does not explain: %v", name, err)
			}
			var st interface{ SQLState() string }
			if errors.As(err, &st) && st.SQLState() != "23502" {
				t.Fatalf("%s: SQLSTATE %s, want 23502", name, st.SQLState())
			}
		}
		check("unlink", document.Comments.Unlink(ctx, db, doc, &models.Comment{ID: 1}))
		check("replace", document.Comments.Replace(ctx, db, doc, &models.Comment{ID: 1}))
		check("replace empty", document.Comments.Replace(ctx, db, doc))
		if n, _ := document.Comments.Count(ctx, db, doc); n != 3 {
			t.Fatalf("failed clears must roll back: count=%d", n)
		}
	})
}

// A join table without a unique key still gets no duplicates from Replace, which
// only inserts the keys that are not there yet.
func TestAssociationReplaceDoesNotRewriteKeptJoinRows(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		mustExec(t, db, `CREATE TABLE taggings_nokey (document_id BIGINT NOT NULL, tag_id BIGINT NOT NULL)`)
		mustExec(t, db, `INSERT INTO taggings_nokey (document_id, tag_id) VALUES (1, 1), (1, 2)`)
		rel := document.Tags
		through := *rel.Through
		through.Table = momo.TableRef{Name: "taggings_nokey"}
		rel.Through = &through
		doc := &models.Document{ID: 1}
		if err := rel.Replace(ctx, db, doc, &models.Tag{ID: 1}, &models.Tag{ID: 2}, &models.Tag{ID: 3}); err != nil {
			t.Fatal(err)
		}
		got := ids(t, db, `SELECT tag_id FROM taggings_nokey WHERE document_id = 1 ORDER BY tag_id`)
		if !sameIDs(got, 1, 2, 3) {
			t.Fatalf("rows = %v, want 1 2 3 without duplicates", got)
		}
	})
}

// The discriminator scopes the clearing modes: image 2 is owned by author 1,
// which collides with document 1 on owner_id.
func TestAssociationPolymorphicClearKeepsOtherOwnerType(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		ctx := context.Background()
		relfixture.SeedBasic(t, db)
		if _, err := momo.Exec(ctx, db, `ALTER TABLE images ALTER COLUMN owner_id DROP NOT NULL`); err != nil {
			t.Skipf("cannot make images.owner_id nullable: %v", err)
		}
		doc := &models.Document{ID: 1}
		authorOwner := func() []int64 {
			return ids(t, db, `SELECT id FROM images WHERE owner_type = 'author' AND owner_id = 1`)
		}

		// Unlink names image 2 explicitly; the discriminator keeps it untouched.
		if err := document.Images.Unlink(ctx, db, doc, &models.Image{ID: 2}); err != nil {
			t.Fatal(err)
		}
		if !sameIDs(authorOwner(), 2) {
			t.Fatal("Unlink cleared an image owned by another owner type")
		}
		if n, _ := document.Images.Count(ctx, db, doc); n != 1 {
			t.Fatalf("document image count = %d, want 1", n)
		}

		// Replace with the kept child: the author's image is not in the current
		// set, so nothing is removed or added: BEGIN, select, COMMIT.
		h.After = nil
		if err := document.Images.Replace(ctx, db, doc, &models.Image{ID: 1}); err != nil {
			t.Fatal(err)
		}
		if got := len(h.After); got != 3 {
			t.Fatalf("Replace of an unchanged set issued %d events, want 3", got)
		}
		if !sameIDs(authorOwner(), 2) {
			t.Fatal("Replace cleared an image owned by another owner type")
		}

		// Replace with a new child: only document 1's image 1 goes.
		mustExec(t, db, `INSERT INTO images (id, owner_type, owner_id, url) VALUES (3, 'document', NULL, 'u3')`)
		if err := document.Images.Replace(ctx, db, doc, &models.Image{ID: 3}); err != nil {
			t.Fatal(err)
		}
		if !sameIDs(authorOwner(), 2) {
			t.Fatal("Replace cleared an image owned by another owner type")
		}
		if got := ids(t, db, `SELECT id FROM images WHERE owner_type = 'document' AND owner_id = 1`); !sameIDs(got, 3) {
			t.Fatalf("document images = %v, want [3]", got)
		}

		// Replace with nothing clears the whole set of document 1 only.
		if err := document.Images.Replace(ctx, db, doc); err != nil {
			t.Fatal(err)
		}
		if !sameIDs(authorOwner(), 2) {
			t.Fatal("Replace(empty) cleared an image owned by another owner type")
		}
		if n, _ := document.Images.Count(ctx, db, doc); n != 0 {
			t.Fatalf("document image count = %d, want 0", n)
		}
	})
}
