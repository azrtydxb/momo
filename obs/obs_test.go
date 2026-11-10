// SPDX-License-Identifier: Apache-2.0

package obs_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/obs"
)

const secret = "hunter2-sensitive"

type kid struct{ PID, Seq int64 }

type parent struct {
	ID   int64
	Kids []kid
}

var kidMeta = momo.Meta[kid]{
	Table:      "obs_kids",
	SoftDelete: -1,
	Columns:    []momo.ColumnMeta{{Name: "pid"}, {Name: "seq"}},
	Ptr: func(m *kid, i int) any {
		if i == 0 {
			return &m.PID
		}
		return &m.Seq
	},
}

var kids = momo.Relation[parent, kid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "obs_parents"}, Child: momo.TableRef{Name: "obs_kids"},
	ChildMeta:  &kidMeta,
	ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"seq"},
	ParentKey: func(p *parent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *kid) ([]any, bool) { return []any{c.PID}, true },
	GetMany:   func(p *parent) []kid { return p.Kids },
	SetMany:   func(p *parent, cs []kid) { p.Kids = cs },
}

func parents(n int) []parent {
	ps := make([]parent, n)
	for i := range ps {
		ps[i].ID = int64(i + 1)
	}
	return ps
}

func insertSecret(t *testing.T, db *momo.DB) {
	t.Helper()
	_, err := momo.Insert[suite.Document](db).Models([]suite.Document{{TenantID: 1, Title: "t", Secret: secret}}).Exec(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

// loadEach loads the relation once per parent, the shape of an N+1.
func loadEach(t *testing.T, ctx context.Context, db *momo.DB, n int) {
	t.Helper()
	for _, p := range parents(n) {
		if err := momo.Load(ctx, db, []parent{p}, kids); err != nil {
			t.Error(err)
			return
		}
	}
}

func TestRecorderCapturesQueryAndRelationEvents(t *testing.T) {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec))
	if err := momo.Load(context.Background(), db, parents(3), kids); err != nil {
		t.Fatal(err)
	}
	evs := rec.Events()
	if len(evs) != 1 {
		t.Fatalf("want 1 query event, got %d", len(evs))
	}
	if evs[0].Relation != "obs_parents.Kids" || evs[0].Op != "select" || evs[0].SQL == "" || evs[0].CtxSQL != evs[0].SQL {
		t.Fatalf("query event %+v", evs[0])
	}
	if got := rec.SQLs(); len(got) != 1 || got[0] != evs[0].SQL {
		t.Fatalf("SQLs %v", got)
	}
	rels := rec.Relations()
	if len(rels) != 1 || rels[0].Parent != "obs_parents" || rels[0].Name != "Kids" || rels[0].Parents != 3 {
		t.Fatalf("relation events %+v", rels)
	}
	rec.Reset()
	if len(rec.Events()) != 0 || len(rec.SQLs()) != 0 || len(rec.Relations()) != 0 {
		t.Fatal("Reset left events behind")
	}
}

func TestRecorderNeverExposesSensitiveArgs(t *testing.T) {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
	insertSecret(t, db)
	masked := false
	for _, e := range rec.Events() {
		if slices.Contains(e.Args, any(secret)) {
			t.Fatalf("recorder holds the sensitive value in %q", e.SQL)
		}
		if slices.Contains(e.Args, any(momo.RedactedArg)) {
			masked = true
		}
	}
	if !masked {
		t.Fatal("no recorded event carries the redaction placeholder")
	}
}

func TestRecorderEventsAreCopies(t *testing.T) {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec), momo.WithArgLogging())
	insertSecret(t, db)
	first := rec.Events()
	if len(first) == 0 || len(first[0].Args) == 0 {
		t.Fatal("no arguments recorded")
	}
	for i := range first[0].Args {
		first[0].Args[i] = "tampered"
	}
	for _, e := range rec.Events() {
		if slices.Contains(e.Args, any("tampered")) {
			t.Fatal("changing a returned event changed the recorder")
		}
	}
}

func TestRecorderKeepsRelationError(t *testing.T) {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := momo.Load(ctx, db, parents(2), kids)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	rels := rec.Relations()
	if len(rels) != 1 || !errors.Is(rels[0].Err, context.Canceled) {
		t.Fatalf("relation event must carry the load error, got %+v", rels)
	}
}

func TestRecorderConcurrent(t *testing.T) {
	rec := &obs.Recorder{}
	db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(rec))
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 25; i++ {
				if err := momo.Load(context.Background(), db, parents(2), kids); err != nil {
					t.Error(err)
					return
				}
				_ = rec.Events()
				_ = rec.Relations()
			}
		}()
	}
	wg.Wait()
	if n := len(rec.Events()); n != 200 {
		t.Fatalf("want 200 query events, got %d", n)
	}
	if n := len(rec.Relations()); n != 200 {
		t.Fatalf("want 200 relation events, got %d", n)
	}
}

func detector(threshold int) (*momo.DB, *[]obs.Finding, *sync.Mutex) {
	var mu sync.Mutex
	var found []obs.Finding
	h := obs.NewNPlusOne(threshold, func(f obs.Finding) {
		mu.Lock()
		found = append(found, f)
		mu.Unlock()
	})
	return momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(h)), &found, &mu
}

func TestNPlusOne(t *testing.T) {
	t.Run("a loop over the relation load is reported once at the first excess", func(t *testing.T) {
		db, found, _ := detector(3)
		loadEach(t, obs.WithScope(context.Background()), db, 6)
		if len(*found) != 1 {
			t.Fatalf("want 1 finding, got %+v", *found)
		}
		if f := (*found)[0]; f.Count != 4 || f.Relation != "obs_parents.Kids" {
			t.Fatalf("finding %+v, want Count 4 for obs_parents.Kids", f)
		}
	})
	t.Run("exactly the threshold stays silent", func(t *testing.T) {
		db, found, _ := detector(3)
		loadEach(t, obs.WithScope(context.Background()), db, 3)
		if len(*found) != 0 {
			t.Fatalf("got %+v", *found)
		}
	})
	t.Run("one batched load stays silent", func(t *testing.T) {
		db, found, _ := detector(1)
		if err := momo.Load(obs.WithScope(context.Background()), db, parents(500), kids); err != nil {
			t.Fatal(err)
		}
		if len(*found) != 0 {
			t.Fatalf("batched load reported %+v", *found)
		}
	})
	t.Run("a context without a scope is not tracked", func(t *testing.T) {
		db, found, _ := detector(1)
		loadEach(t, context.Background(), db, 6)
		if len(*found) != 0 {
			t.Fatalf("got %+v", *found)
		}
	})
	t.Run("scopes are independent", func(t *testing.T) {
		db, found, _ := detector(3)
		loadEach(t, obs.WithScope(context.Background()), db, 3)
		loadEach(t, obs.WithScope(context.Background()), db, 3)
		if len(*found) != 0 {
			t.Fatalf("got %+v", *found)
		}
	})
	t.Run("a panic in the report function does not break the load", func(t *testing.T) {
		h := obs.NewNPlusOne(1, func(obs.Finding) { panic("boom") })
		db := momo.New(fakedriver.New(dialect.Postgres), momo.WithHook(h))
		loadEach(t, obs.WithScope(context.Background()), db, 4)
	})
	t.Run("concurrent loops in one scope report once", func(t *testing.T) {
		db, found, mu := detector(5)
		ctx := obs.WithScope(context.Background())
		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				loadEach(t, ctx, db, 5)
			}()
		}
		wg.Wait()
		mu.Lock()
		defer mu.Unlock()
		if len(*found) != 1 {
			t.Fatalf("want 1 finding, got %d", len(*found))
		}
	})
}
