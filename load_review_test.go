// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

type lrChild struct{ PID, Seq int64 }

type lrParent struct {
	ID   int64
	Kids []lrChild
}

var lrChildMeta = momo.Meta[lrChild]{
	Table:      "lr_kids",
	SoftDelete: -1,
	Columns:    []momo.ColumnMeta{{Name: "pid"}, {Name: "seq"}},
	Ptr: func(m *lrChild, i int) any {
		if i == 0 {
			return &m.PID
		}
		return &m.Seq
	},
}

var lrKids = momo.Relation[lrParent, lrChild]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "lr_parents"}, Child: momo.TableRef{Name: "lr_kids"},
	ChildMeta:  &lrChildMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"seq"},
	ParentKey: func(p *lrParent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *lrChild) ([]any, bool) { return []any{c.PID}, true },
	GetMany:   func(p *lrParent) []lrChild { return p.Kids },
	SetMany:   func(p *lrParent, cs []lrChild) { p.Kids = cs },
}

// lrParents returns n parents; with the SQLite dialect (32766 binds) 33000
// keys need two statements.
func lrParents(n int, stale bool) []lrParent {
	ps := make([]lrParent, n)
	for i := range ps {
		ps[i].ID = int64(i + 1)
		if stale {
			ps[i].Kids = []lrChild{{PID: 0, Seq: 99}}
		}
	}
	return ps
}

// lrHook records relation events and cancels the context after a statement.
type lrHook struct {
	cancel func()
	events []momo.RelationEvent
}

func (h *lrHook) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }
func (h *lrHook) AfterQuery(context.Context, *momo.QueryEvent) {
	if h.cancel != nil {
		h.cancel()
	}
}
func (h *lrHook) BeforeRelation(ctx context.Context, _ *momo.RelationEvent) context.Context {
	return ctx
}
func (h *lrHook) AfterRelation(_ context.Context, e *momo.RelationEvent) {
	h.events = append(h.events, *e)
}

func TestLoadContextCanceledBetweenChunks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &lrHook{cancel: cancel}
	drv := fakedriver.New(dialect.SQLite)
	db := momo.New(drv, momo.WithHook(h))
	parents := lrParents(33000, true)

	err := momo.Load(ctx, db, parents, lrKids)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if n := len(drv.Calls()); n != 1 {
		t.Fatalf("the second chunk must not run after cancellation, ran %d statements", n)
	}
	if len(h.events) != 1 {
		t.Fatalf("want one relation event, got %d", len(h.events))
	}
	e := h.events[0]
	if !errors.Is(e.Err, context.Canceled) || e.Parents != 33000 || e.Queries != 2 || e.Children != 0 || e.Parent != "lr_parents" || e.Name != "Kids" {
		t.Fatalf("event on error: %+v", e)
	}
	for i := range parents {
		if parents[i].Kids != nil {
			t.Fatalf("parent %d: associations are cleared up front even when the load fails", i)
		}
	}
}

// txCountDriver counts statements issued directly on the driver versus on a
// transaction it began.
type txCountDriver struct {
	*fakedriver.Driver
	direct, inTx int
}

func (d *txCountDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	d.direct++
	return d.Driver.Query(ctx, sql, args)
}

func (d *txCountDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, o)
	return &countTx{DriverTx: tx, d: d}, err
}

type countTx struct {
	momo.DriverTx
	d *txCountDriver
}

func (t *countTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	t.d.inTx++
	return t.DriverTx.Query(ctx, sql, args)
}

func TestLoadRunsEveryChunkOnThePassedTx(t *testing.T) {
	ctx := context.Background()
	drv := &txCountDriver{Driver: fakedriver.New(dialect.SQLite)}
	h := &lrHook{}
	db := momo.New(drv, momo.WithHook(h))
	err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		return momo.Load(ctx, tx, lrParents(33000, false), lrKids)
	})
	if err != nil {
		t.Fatal(err)
	}
	if drv.inTx != 2 || drv.direct != 0 {
		t.Fatalf("both chunks must run on the tx: inTx=%d direct=%d", drv.inTx, drv.direct)
	}
	if len(h.events) != 1 || h.events[0].Queries != 2 || h.events[0].Err != nil {
		t.Fatalf("events: %+v", h.events)
	}
}

func TestLoadPerParentLimitReturnsUnsupported(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	err := momo.Load(context.Background(), db, lrParents(2, false), lrKids, momo.LoadPerParentLimit[lrChild](1))
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
}
