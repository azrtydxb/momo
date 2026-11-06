// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/types"
	"github.com/azrtydxb/momo/vector"
)

type jlStatus string

// jlChild covers every destination shape the parent scan path accepts.
type jlChild struct {
	ID   int64
	I8   int8
	PI8  *int8
	U8   uint8
	St   jlStatus
	UID  *types.UUID
	D    *types.Date
	Emb  *vector.Vector
	JS   types.JSON[[]int]
	Name string
	T    *time.Time
}

var jlChildMeta = momo.Meta[jlChild]{
	Table: "jl_child",
	Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "i8"}, {Name: "pi8"}, {Name: "u8"}, {Name: "st"},
		{Name: "uid"}, {Name: "d"}, {Name: "emb"}, {Name: "js"}, {Name: "name"}, {Name: "t"}},
	Ptr: func(m *jlChild, i int) any {
		return []any{&m.ID, &m.I8, &m.PI8, &m.U8, &m.St, &m.UID, &m.D, &m.Emb, &m.JS, &m.Name, &m.T}[i]
	},
	Value:    func(m *jlChild, i int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*jlChild) MomoMeta() *momo.Meta[jlChild] { return &jlChildMeta }

type jlParent struct {
	ID      int64
	ChildID *int64
	Child   *jlChild
}

var jlParentMeta = momo.Meta[jlParent]{
	Table:    "jl_parent",
	Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "child_id"}},
	Ptr:      func(m *jlParent, i int) any { return []any{&m.ID, &m.ChildID}[i] },
	Value:    func(m *jlParent, i int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
	PK: []int{0},
}

func (*jlParent) MomoMeta() *momo.Meta[jlParent] { return &jlParentMeta }

var jlRel = momo.Relation[jlParent, jlChild]{
	Name: "Child", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "jl_parent"}, Child: momo.TableRef{Name: "jl_child"},
	ParentMeta: &jlParentMeta, ChildMeta: &jlChildMeta,
	ParentCols: []string{"child_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *jlParent) ([]any, bool) {
		if p.ChildID == nil {
			return nil, false
		}
		return []any{*p.ChildID}, true
	},
	ChildKey: func(c *jlChild) ([]any, bool) { return []any{c.ID}, true },
	GetOne:   func(p *jlParent) *jlChild { return p.Child },
	SetOne:   func(p *jlParent, c *jlChild) { p.Child = c },
}

func TestJoinLoadScanMatchesPlainLoad(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE jl_child (id bigint PRIMARY KEY, i8 smallint NOT NULL, pi8 smallint, u8 smallint NOT NULL,
		st text NOT NULL, uid uuid, d date, emb vector(3), js jsonb NOT NULL, name text NOT NULL, t timestamptz)`)
	mustExecPG(t, db, `CREATE TABLE jl_parent (id bigint PRIMARY KEY, child_id bigint REFERENCES jl_child(id))`)
	// Child 1 has every nullable column set; child 2 has them all NULL.
	mustExecPG(t, db, `INSERT INTO jl_child VALUES
		(1, -7, 12, 200, 'open', '11111111-2222-3333-4444-555555555555', '2024-02-29', '[1,2,3]', '[1,2]', 'full', '2024-01-02T03:04:05Z'),
		(2, 5, NULL, 0, 'closed', NULL, NULL, NULL, '[]', 'sparse', NULL)`)
	mustExecPG(t, db, `INSERT INTO jl_parent VALUES (1, 1), (2, 2), (3, NULL)`)

	plain, err := momo.Select[jlChild, *jlChild](db).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(plain, func(a, b jlChild) int { return int(a.ID - b.ID) })
	if len(plain) != 2 {
		t.Fatalf("plain rows: %d", len(plain))
	}
	if plain[0].UID == nil || plain[0].D == nil || plain[0].Emb == nil || plain[0].PI8 == nil {
		t.Fatalf("plain load lost values: %+v", plain[0])
	}
	if plain[1].UID != nil || plain[1].D != nil || plain[1].Emb != nil || plain[1].PI8 != nil || plain[1].T != nil {
		t.Fatalf("plain load of NULLs: %+v", plain[1])
	}

	ps, err := momo.Select[jlParent, *jlParent](db).Preload(jlRel.Joined()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 3 {
		t.Fatalf("parents: %d", len(ps))
	}
	byID := map[int64]*jlParent{}
	for i := range ps {
		byID[ps[i].ID] = &ps[i]
	}
	for i, want := range plain {
		got := byID[int64(i+1)].Child
		if got == nil {
			t.Fatalf("parent %d: child nil", i+1)
		}
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("parent %d: joined child differs from plain load:\n joined %+v\n plain  %+v", i+1, *got, want)
		}
	}
	if byID[3].Child != nil {
		t.Fatalf("all-NULL LEFT JOIN must leave the relation nil, got %+v", byID[3].Child)
	}
}

// A raw unqualified column in the parent's WHERE is ambiguous with the joined
// child's column of the same name; qualifying it works.
func TestJoinLoadRawUnqualifiedColumn(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE jl_child (id bigint PRIMARY KEY, i8 smallint NOT NULL, pi8 smallint, u8 smallint NOT NULL,
		st text NOT NULL, uid uuid, d date, emb vector(3), js jsonb NOT NULL, name text NOT NULL, t timestamptz)`)
	mustExecPG(t, db, `CREATE TABLE jl_parent (id bigint PRIMARY KEY, child_id bigint REFERENCES jl_child(id))`)
	mustExecPG(t, db, `INSERT INTO jl_child VALUES (1, 1, NULL, 1, 'a', NULL, NULL, NULL, '[]', 'n', NULL)`)
	mustExecPG(t, db, `INSERT INTO jl_parent VALUES (1, 1)`)

	q := momo.Select[jlParent, *jlParent](db).Preload(jlRel.Joined())
	if _, err := q.Where(momo.SQL("id = ?", 1)).All(ctx); err == nil {
		t.Fatal("unqualified raw id with a joined relation is expected to be ambiguous")
	} else {
		t.Logf("unqualified raw id: %v", err)
	}
	ps, err := q.Where(momo.SQL("jl_parent.id = ?", 1)).All(ctx)
	if err != nil || len(ps) != 1 || ps[0].Child == nil {
		t.Fatalf("qualified raw id: %v %+v", err, ps)
	}
}
