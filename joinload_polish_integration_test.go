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
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
)

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
