// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

func selectDocs(db *momo.DB) momo.SelectQuery[models.Document] {
	return momo.Select[models.Document, *models.Document](db)
}

func TestJoinLoadToOne(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		h.After = nil
		docs, err := selectDocs(db).OrderBy(document.ID.Asc()).Preload(document.Author.Joined()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("join loading must be one statement, got %d", len(h.After))
		}
		if len(docs) != 4 {
			t.Fatalf("join must not duplicate or drop parents: %d", len(docs))
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" {
			t.Fatalf("doc 1 author: %+v", docs[0].Author)
		}
		if docs[2].Author == nil || docs[2].Author.ID != 2 || docs[2].Author.Name != "Grace" {
			t.Fatalf("doc 3 author: %+v", docs[2].Author)
		}
		if docs[3].Author != nil {
			t.Fatal("NULL foreign key through a LEFT JOIN must leave Author nil, not a zero struct")
		}
		if docs[0].Title != "A1" || docs[3].Title != "Orphan" {
			t.Fatalf("parent columns must still scan in place: %q %q", docs[0].Title, docs[3].Title)
		}
		// Parents sharing a target get their own copy.
		if docs[0].Author == docs[1].Author {
			t.Fatal("two parents must not share one child pointer")
		}
		if len(h.Events) != 0 {
			t.Fatalf("a joined load runs no statement of its own and emits no relation event: %+v", h.Events)
		}
	})
}

// A joined load must follow the parent query's Where, Order, Limit and Offset
// exactly: a to-one join cannot multiply parent rows.
func TestJoinLoadParentQueryClauses(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		h.After = nil
		docs, err := selectDocs(db).Where(document.ID.Gt(1)).OrderBy(document.ID.Desc()).Limit(2).
			Preload(document.Author.Joined()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(docs) != 2 || docs[0].ID != 4 || docs[1].ID != 3 {
			t.Fatalf("limit/order/where: statements=%d docs=%+v", len(h.After), docs)
		}
		if docs[0].Author != nil || docs[1].Author == nil || docs[1].Author.Name != "Grace" {
			t.Fatalf("authors: %+v %+v", docs[0].Author, docs[1].Author)
		}
		page, err := selectDocs(db).OrderBy(document.ID.Asc()).Limit(2).Offset(1).Preload(document.Author.Joined()).All(ctx)
		if err != nil || len(page) != 2 || page[0].ID != 2 || page[1].ID != 3 || page[0].Author == nil {
			t.Fatalf("offset page: %v %+v", err, page)
		}
		doc, err := selectDocs(db).Where(document.ID.Eq(3)).Preload(document.Author.Joined()).One(ctx)
		if err != nil || doc.Author == nil || doc.Author.Name != "Grace" {
			t.Fatalf("One: %v %+v", err, doc)
		}
		orphan, err := selectDocs(db).Where(document.ID.Eq(4)).Preload(document.Author.Joined()).First(ctx)
		if err != nil || orphan.Author != nil {
			t.Fatalf("First on NULL FK: %v %+v", err, orphan)
		}
		if _, err := selectDocs(db).Where(document.ID.Eq(99)).Preload(document.Author.Joined()).One(ctx); !errors.Is(err, momo.ErrNotFound) {
			t.Fatalf("One with no row: %v", err)
		}
		if _, err := selectDocs(db).Preload(document.Author.Joined()).One(ctx); !errors.Is(err, momo.ErrTooManyRows) {
			t.Fatalf("One with several rows: %v", err)
		}
	})
}

// Every column of a self-referencing join collides by name with the parent's;
// the alias keeps the two sides apart.
func TestJoinLoadSelfReference(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).
			Preload(employee.Manager.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(emps) != 3 {
			t.Fatalf("statements=%d employees=%d", len(h.After), len(emps))
		}
		if emps[0].Manager != nil {
			t.Fatalf("boss has no manager: %+v", emps[0].Manager)
		}
		for _, e := range emps[1:] {
			if e.Manager == nil || e.Manager.ID != 1 || e.Manager.Name != "boss" || e.Name == "boss" {
				t.Fatalf("%s: manager %+v", e.Name, e.Manager)
			}
		}
	})
}

// A composite key joins on every column, and a NULL component matches nothing.
func TestJoinLoadComposite(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).
			Preload(store.Region.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(stores) != 4 {
			t.Fatalf("statements=%d stores=%d", len(h.After), len(stores))
		}
		want := []string{"Europe", "America", "", "Europe2"}
		for i, s := range stores {
			switch {
			case want[i] == "" && s.Region != nil:
				t.Fatalf("store %d has a NULL key component: %+v", s.ID, s.Region)
			case want[i] != "" && (s.Region == nil || s.Region.Name != want[i]):
				t.Fatalf("store %d region: %+v want %s", s.ID, s.Region, want[i])
			}
		}
	})
}

// A soft-deleted target is excluded in the ON clause, so the parent row
// survives with a nil relation; a WHERE would drop the parent.
func TestJoinLoadSoftDeletedTargetKeepsParent(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		mustExec(t, db, "INSERT INTO notes (id, document_id, body, deleted) VALUES (1, 1, 'live', FALSE), (3, 3, 'gone', TRUE)")
		notes := map[int64]*models.Note{}
		rel := momo.Relation[models.Comment, models.Note]{
			Name: "Note", Kind: momo.BelongsTo,
			Parent: momo.TableRef{Name: "comments"}, Child: momo.TableRef{Name: "notes"},
			ParentMeta: (*models.Comment)(nil).MomoMeta(), ChildMeta: (*models.Note)(nil).MomoMeta(),
			ParentCols: []string{"document_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
			ParentKey: func(p *models.Comment) ([]any, bool) { return []any{p.DocumentID}, true },
			ChildKey:  func(c *models.Note) ([]any, bool) { return []any{c.ID}, true },
			GetOne:    func(p *models.Comment) *models.Note { return notes[p.ID] },
			SetOne:    func(p *models.Comment, c *models.Note) { notes[p.ID] = c },
		}
		// Comments 1-3 point at note 1 (live); comment 4 points at note 3 (soft-deleted).
		h.After = nil
		cs, err := momo.Select[models.Comment, *models.Comment](db).OrderBy(comment.ID.Asc()).
			Preload(rel.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || len(cs) != 4 {
			t.Fatalf("a soft-deleted target must not drop the parent: statements=%d parents=%d", len(h.After), len(cs))
		}
		for _, id := range []int64{1, 2, 3} {
			if n := notes[id]; n == nil || n.ID != 1 || n.Body != "live" {
				t.Fatalf("comment %d note: %+v", id, n)
			}
		}
		if notes[4] != nil {
			t.Fatalf("comment 4 targets the soft-deleted note 3: %+v", notes[4])
		}
	})
}

// Joined loaders compose with ordinary preloads, which still run as separate
// statements after the parent query, and several joins use distinct aliases.
func TestJoinLoadWithOtherPreloads(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		h.After = nil
		docs, err := selectDocs(db).OrderBy(document.ID.Asc()).
			Preload(document.Author.Joined(), document.Comments, document.Author.Joined()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 2 {
			t.Fatalf("one joined statement plus one comments statement, got %d", len(h.After))
		}
		if docs[0].Author == nil || len(docs[0].Comments) != 3 || docs[3].Author != nil {
			t.Fatalf("doc 1: %+v", docs[0])
		}
		q := selectDocs(db).Preload(document.Author.Joined(), document.Author.Joined())
		c, err := q.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, `AS "momo_j0"`) || !strings.Contains(c.SQL, `AS "momo_j1"`) {
			t.Fatalf("each joined loader needs its own alias: %s", c.SQL)
		}
	})
}

// The SQL puts the soft-delete filter and the key match in ON, never in WHERE.
func TestJoinLoadSQLShape(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		rel := momo.Relation[models.Comment, models.Note]{
			Name: "Note", Kind: momo.BelongsTo,
			Parent: momo.TableRef{Name: "comments"}, Child: momo.TableRef{Name: "notes"},
			ParentMeta: (*models.Comment)(nil).MomoMeta(), ChildMeta: (*models.Note)(nil).MomoMeta(),
			ParentCols: []string{"document_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
			ParentKey: func(p *models.Comment) ([]any, bool) { return []any{p.DocumentID}, true },
			ChildKey:  func(c *models.Note) ([]any, bool) { return []any{c.ID}, true },
			GetOne:    func(*models.Comment) *models.Note { return nil },
			SetOne:    func(*models.Comment, *models.Note) {},
		}
		c, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.Approved.Eq(true)).Preload(rel.Joined()).SQL()
		if err != nil {
			t.Fatal(err)
		}
		on := strings.Index(c.SQL, " ON ")
		where := strings.Index(c.SQL, " WHERE ")
		if on < 0 || where < on {
			t.Fatalf("missing ON before WHERE: %s", c.SQL)
		}
		onClause, whereClause := c.SQL[on:where], c.SQL[where:]
		if !strings.Contains(onClause, `"momo_j0"."deleted" = FALSE`) || !strings.Contains(onClause, `"momo_j0"."id" = "comments"."document_id"`) {
			t.Fatalf("ON clause: %s", onClause)
		}
		if strings.Contains(whereClause, "momo_j0") {
			t.Fatalf("the target's soft-delete filter must not be in WHERE: %s", whereClause)
		}
		if !strings.Contains(c.SQL, `LEFT JOIN "notes" AS "momo_j0"`) {
			t.Fatalf("LEFT JOIN: %s", c.SQL)
		}
	})
}

func TestJoinLoadUnsupported(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		for name, q := range map[string]momo.SelectQuery[models.Document]{
			"has-many":     selectDocs(db).Preload(document.Comments.Joined()),
			"many-to-many": selectDocs(db).Preload(document.Tags.Joined()),
			"group by":     selectDocs(db).Preload(document.Author.Joined()).GroupBy(document.ID),
			"for update":   selectDocs(db).Preload(document.Author.Joined()).ForUpdate(),
		} {
			if _, err := q.All(ctx); !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("%s: All error %v, want ErrUnsupported", name, err)
			}
		}
		q := selectDocs(db).Preload(document.Author.Joined())
		if err := q.Each(ctx, func(*models.Document) error { return nil }); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("Each: %v", err)
		}
		for _, err := range q.Iter(ctx) {
			if !errors.Is(err, momo.ErrUnsupported) {
				t.Fatalf("Iter: %v", err)
			}
		}
		// Load and LoadAll do not take joined loaders.
		docs := allDocs(t, db)
		if err := momo.LoadAll(ctx, db, docs, document.Author.Joined()); !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("LoadAll with a joined loader: %v", err)
		}
		// A user-supplied select list of the wrong width is still a validation error.
		if _, err := selectDocs(db).Columns(document.ID).Preload(document.Author.Joined()).All(ctx); !errors.Is(err, momo.ErrValidation) {
			t.Fatalf("short select list: %v", err)
		}
	})
}

// The base query is not changed by Preload.
func TestJoinLoadPreloadIsImmutable(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, _ *relfixture.Hook) {
		base := selectDocs(db).OrderBy(document.ID.Asc())
		_ = base.Preload(document.Author.Joined())
		c, err := base.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(c.SQL, "JOIN") {
			t.Fatalf("Preload leaked into its receiver: %s", c.SQL)
		}
	})
}

// jpMeta builds the Meta of a hand-written model; ptrs returns the field
// pointers in column order.
func jpMeta[T any](table string, cols []string, ptrs func(*T) []any) momo.Meta[T] {
	cm := make([]momo.ColumnMeta, len(cols))
	for i, c := range cols {
		cm[i] = momo.ColumnMeta{Name: c}
	}
	cm[0].Flags = momo.FlagPK
	return momo.Meta[T]{
		Table: table, Columns: cm,
		Ptr:      func(m *T, i int) any { return ptrs(m)[i] },
		Value:    func(m *T, i int) any { return nil },
		Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		PK: []int{0},
	}
}

// --- a matched joined child with NULL in a non-nullable column ---

type jpBad struct {
	ID   int64
	Name string
}

var jpBadMeta = jpMeta("jp_bad", []string{"id", "name"}, func(m *jpBad) []any { return []any{&m.ID, &m.Name} })

func (*jpBad) MomoMeta() *momo.Meta[jpBad] { return &jpBadMeta }

type jpBadParent struct {
	ID    int64
	BadID *int64
	Bad   *jpBad
}

var jpBadParentMeta = jpMeta("jp_bad_parent", []string{"id", "bad_id"}, func(m *jpBadParent) []any { return []any{&m.ID, &m.BadID} })

func (*jpBadParent) MomoMeta() *momo.Meta[jpBadParent] { return &jpBadParentMeta }

var jpBadRel = momo.Relation[jpBadParent, jpBad]{
	Name: "Bad", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "jp_bad_parent"}, Child: momo.TableRef{Name: "jp_bad"},
	ParentMeta: &jpBadParentMeta, ChildMeta: &jpBadMeta,
	ParentCols: []string{"bad_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *jpBadParent) ([]any, bool) {
		if p.BadID == nil {
			return nil, false
		}
		return []any{*p.BadID}, true
	},
	ChildKey: func(c *jpBad) ([]any, bool) { return []any{c.ID}, true },
	GetOne:   func(p *jpBadParent) *jpBad { return p.Bad },
	SetOne:   func(p *jpBadParent, c *jpBad) { p.Bad = c },
}

func TestJoinLoadNullColumnErrorNamesColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE jp_bad (id bigint PRIMARY KEY, name text)`)
	mustExecPG(t, db, `CREATE TABLE jp_bad_parent (id bigint PRIMARY KEY, bad_id bigint REFERENCES jp_bad(id))`)
	mustExecPG(t, db, `INSERT INTO jp_bad VALUES (1, NULL)`)
	mustExecPG(t, db, `INSERT INTO jp_bad_parent VALUES (1, 1)`)

	_, err := momo.Select[jpBadParent, *jpBadParent](db).Preload(jpBadRel.Joined()).All(ctx)
	var me *momo.Error
	if !errors.As(err, &me) || me.Kind != momo.ErrValidation {
		t.Fatalf("want ErrValidation, got %v", err)
	}
	if me.Column != "name" || !strings.Contains(err.Error(), "column=name") {
		t.Fatalf("the error must name the child column: %v", err)
	}
}

// --- two different Joined relations in one query ---

type jpA struct {
	ID   int64
	Name string
}
type jpB struct {
	ID   int64
	Size int64
}

var (
	jpAMeta = jpMeta("jp_a", []string{"id", "name"}, func(m *jpA) []any { return []any{&m.ID, &m.Name} })
	jpBMeta = jpMeta("jp_b", []string{"id", "size"}, func(m *jpB) []any { return []any{&m.ID, &m.Size} })
)

func (*jpA) MomoMeta() *momo.Meta[jpA] { return &jpAMeta }
func (*jpB) MomoMeta() *momo.Meta[jpB] { return &jpBMeta }

type jpTwo struct {
	ID  int64
	AID *int64
	BID *int64
	A   *jpA
	B   *jpB
}

var jpTwoMeta = jpMeta("jp_two", []string{"id", "a_id", "b_id"}, func(m *jpTwo) []any { return []any{&m.ID, &m.AID, &m.BID} })

func (*jpTwo) MomoMeta() *momo.Meta[jpTwo] { return &jpTwoMeta }

func jpKey(v *int64) ([]any, bool) {
	if v == nil {
		return nil, false
	}
	return []any{*v}, true
}

var jpRelA = momo.Relation[jpTwo, jpA]{
	Name: "A", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "jp_two"}, Child: momo.TableRef{Name: "jp_a"},
	ParentMeta: &jpTwoMeta, ChildMeta: &jpAMeta,
	ParentCols: []string{"a_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *jpTwo) ([]any, bool) { return jpKey(p.AID) },
	ChildKey:  func(c *jpA) ([]any, bool) { return []any{c.ID}, true },
	GetOne:    func(p *jpTwo) *jpA { return p.A },
	SetOne:    func(p *jpTwo, c *jpA) { p.A = c },
}

var jpRelB = momo.Relation[jpTwo, jpB]{
	Name: "B", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "jp_two"}, Child: momo.TableRef{Name: "jp_b"},
	ParentMeta: &jpTwoMeta, ChildMeta: &jpBMeta,
	ParentCols: []string{"b_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *jpTwo) ([]any, bool) { return jpKey(p.BID) },
	ChildKey:  func(c *jpB) ([]any, bool) { return []any{c.ID}, true },
	GetOne:    func(p *jpTwo) *jpB { return p.B },
	SetOne:    func(p *jpTwo, c *jpB) { p.B = c },
}

func TestJoinLoadTwoRelationsInOneQuery(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE jp_a (id bigint PRIMARY KEY, name text NOT NULL)`)
	mustExecPG(t, db, `CREATE TABLE jp_b (id bigint PRIMARY KEY, size bigint NOT NULL)`)
	mustExecPG(t, db, `CREATE TABLE jp_two (id bigint PRIMARY KEY, a_id bigint REFERENCES jp_a(id), b_id bigint REFERENCES jp_b(id))`)
	mustExecPG(t, db, `INSERT INTO jp_a VALUES (1, 'one'), (2, 'two')`)
	mustExecPG(t, db, `INSERT INTO jp_b VALUES (10, 100), (20, 200)`)
	mustExecPG(t, db, `INSERT INTO jp_two VALUES (1, 1, 10), (2, 2, NULL), (3, NULL, 20), (4, NULL, NULL)`)

	rows, err := momo.Select[jpTwo, *jpTwo](db).Preload(jpRelA.Joined(), jpRelB.Joined()).OrderBy(momo.NewColumn[int64]("jp_two", "id", 0).Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Fatalf("rows: %d", len(rows))
	}
	check := func(i int, a string, b int64) {
		t.Helper()
		r := rows[i]
		if (a == "") != (r.A == nil) || (a != "" && r.A.Name != a) {
			t.Errorf("row %d: A = %+v, want %q", r.ID, r.A, a)
		}
		if (b == 0) != (r.B == nil) || (b != 0 && r.B.Size != b) {
			t.Errorf("row %d: B = %+v, want %d", r.ID, r.B, b)
		}
	}
	check(0, "one", 100)
	check(1, "two", 0)
	check(2, "", 200)
	check(3, "", 0)
}

// --- non-pointer UUID and Date and a value-receiver Scanner ---

var jpScans struct {
	sync.Mutex
	log []string
}

func jpScanned() []string {
	jpScans.Lock()
	defer jpScans.Unlock()
	return slices.Clone(jpScans.log)
}

func jpReset() {
	jpScans.Lock()
	defer jpScans.Unlock()
	jpScans.log = nil
}

// jpCode is an sql.Scanner with a value receiver; it records what it was given.
type jpCode struct{}

func (jpCode) Scan(src any) error {
	jpScans.Lock()
	defer jpScans.Unlock()
	jpScans.log = append(jpScans.log, fmt.Sprintf("%v", src))
	return nil
}

type jpVals struct {
	ID   int64
	UID  types.UUID
	D    types.Date
	Code jpCode
}

var jpValsMeta = jpMeta("jp_vals", []string{"id", "uid", "d", "code"}, func(m *jpVals) []any { return []any{&m.ID, &m.UID, &m.D, &m.Code} })

func (*jpVals) MomoMeta() *momo.Meta[jpVals] { return &jpValsMeta }

type jpValsParent struct {
	ID    int64
	ValID *int64
	Val   *jpVals
}

var jpValsParentMeta = jpMeta("jp_vals_parent", []string{"id", "val_id"}, func(m *jpValsParent) []any { return []any{&m.ID, &m.ValID} })

func (*jpValsParent) MomoMeta() *momo.Meta[jpValsParent] { return &jpValsParentMeta }

var jpValsRel = momo.Relation[jpValsParent, jpVals]{
	Name: "Val", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "jp_vals_parent"}, Child: momo.TableRef{Name: "jp_vals"},
	ParentMeta: &jpValsParentMeta, ChildMeta: &jpValsMeta,
	ParentCols: []string{"val_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *jpValsParent) ([]any, bool) { return jpKey(p.ValID) },
	ChildKey:  func(c *jpVals) ([]any, bool) { return []any{c.ID}, true },
	GetOne:    func(p *jpValsParent) *jpVals { return p.Val },
	SetOne:    func(p *jpValsParent, c *jpVals) { p.Val = c },
}

func TestJoinLoadValueTypesMatchPlainLoad(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE jp_vals (id bigint PRIMARY KEY, uid uuid NOT NULL, d date NOT NULL, code text)`)
	mustExecPG(t, db, `CREATE TABLE jp_vals_parent (id bigint PRIMARY KEY, val_id bigint REFERENCES jp_vals(id))`)
	mustExecPG(t, db, `INSERT INTO jp_vals VALUES
		(1, '11111111-2222-3333-4444-555555555555', '2024-02-29', 'x'),
		(2, '66666666-7777-8888-9999-000000000000', '2025-12-31', NULL)`)
	mustExecPG(t, db, `INSERT INTO jp_vals_parent VALUES (1, 1), (2, 2)`)

	jpReset()
	plain, err := momo.Select[jpVals, *jpVals](db).OrderBy(momo.NewColumn[int64]("jp_vals", "id", 0).Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	plainScans := jpScanned()

	jpReset()
	ps, err := momo.Select[jpValsParent, *jpValsParent](db).Preload(jpValsRel.Joined()).OrderBy(momo.NewColumn[int64]("jp_vals_parent", "id", 0).Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	joinedScans := jpScanned()

	if len(plain) != 2 || len(ps) != 2 {
		t.Fatalf("rows: plain %d joined %d", len(plain), len(ps))
	}
	for i := range plain {
		if ps[i].Val == nil || !reflect.DeepEqual(*ps[i].Val, plain[i]) {
			t.Errorf("row %d: joined %+v differs from plain %+v", i+1, ps[i].Val, plain[i])
		}
	}
	if plain[0].UID == (types.UUID{}) || plain[0].D.Time.IsZero() {
		t.Fatalf("plain load lost the UUID or Date: %+v", plain[0])
	}
	if !slices.Equal(plainScans, joinedScans) || len(plainScans) != 2 {
		t.Fatalf("value-receiver Scanner calls differ: plain %v joined %v", plainScans, joinedScans)
	}
}
