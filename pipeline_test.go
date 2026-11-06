// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pipeBase opens a PostgreSQL driver on a fresh fixture schema with seed data.
func pipeBase(t *testing.T) momo.Driver {
	t.Helper()
	base := relfixture.PostgresDriver(t)
	setup := momo.New(base)
	relfixture.CreateSchema(t, setup)
	relfixture.SeedBasic(t, setup)
	return base
}

// pipeDocs reads the seeded documents in id order.
func pipeDocs(t testing.TB, q momo.Querier) []models.Document {
	t.Helper()
	docs, err := momo.Select[models.Document, *models.Document](q).OrderBy(document.ID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

// pipeJSON renders loaded parents so two loads can be compared by value.
func pipeJSON(t testing.TB, docs []models.Document) string {
	t.Helper()
	b, err := json.Marshal(docs)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// batchStub wraps a batching driver and lets a test rewrite or reject the batch.
type batchStub struct {
	momo.Driver
	mutate func([]momo.Compiled) []momo.Compiled
	err    error
	mu     sync.Mutex
	sent   int
}

func (d *batchStub) SendBatch(ctx context.Context, qs []momo.Compiled) ([]momo.Rows, error) {
	d.mu.Lock()
	d.sent++
	d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	if d.mutate != nil {
		qs = d.mutate(append([]momo.Compiled(nil), qs...))
	}
	return d.Driver.(momo.Batcher).SendBatch(ctx, qs)
}

func (d *batchStub) ClassifyError(err error) error {
	if c, ok := d.Driver.(momo.ErrorClassifier); ok {
		return c.ClassifyError(err)
	}
	return err
}

func TestPipelinedEagerLoad(t *testing.T) {
	ctx := context.Background()
	base := pipeBase(t)
	drv, ctr := relfixture.Counting(base)
	db := momo.New(drv)

	docs := pipeDocs(t, db)
	ctr.Reset()
	if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments); err != nil {
		t.Fatal(err)
	}
	if ctr.Batches.Load() != 1 || ctr.Queries.Load() != 0 {
		t.Fatalf("two relations must be one batch and no single statements: batches=%d queries=%d",
			ctr.Batches.Load(), ctr.Queries.Load())
	}
	got := fmt.Sprintf("%s %d", docs[0].Author.Name, len(docs[0].Comments))
	if got != "Ada 3" {
		t.Fatalf("batched result differs: %s", got)
	}

	// The same load without a batching driver yields identical parents.
	seqDrv, seqCtr := relfixture.Counting(relfixture.WithoutBatch(base))
	seqDB := momo.New(seqDrv)
	seqDocs := pipeDocs(t, seqDB)
	seqCtr.Reset()
	if err := momo.LoadAll(ctx, seqDB, seqDocs, document.Author, document.Comments); err != nil {
		t.Fatal(err)
	}
	if seqCtr.Batches.Load() != 0 || seqCtr.Queries.Load() != 2 {
		t.Fatalf("sequential reference: batches=%d queries=%d, want 0 and 2", seqCtr.Batches.Load(), seqCtr.Queries.Load())
	}
	if pipeJSON(t, docs) != pipeJSON(t, seqDocs) {
		t.Fatalf("batched and sequential results differ:\n%s\n%s", pipeJSON(t, docs), pipeJSON(t, seqDocs))
	}
}

// TestPipelinedMatchesSequential runs relation shapes through the batched and
// the sequential path and requires identical parents and the expected number of
// round trips.
func TestPipelinedMatchesSequential(t *testing.T) {
	base := pipeBase(t)

	type loaders = []momo.Loader[models.Document]
	cases := []struct {
		name       string
		maxParams  int
		loaders    loaders
		wantBatch  int64 // batches on the pipelined path
		wantStmts  int64 // statements on the sequential path
		wantSingle int64 // statements outside a batch on the pipelined path
	}{
		{name: "two siblings", loaders: loaders{document.Author, document.Comments}, wantBatch: 1, wantStmts: 2},
		{name: "four siblings incl many-to-many", loaders: loaders{document.Author, document.Comments, document.Tags, document.Images}, wantBatch: 1, wantStmts: 4},
		{name: "single relation stays single", loaders: loaders{document.Comments}, wantStmts: 1, wantSingle: 1},
		{name: "nested level batches siblings", loaders: loaders{
			document.Author.Then(author.Images, author.FirstDocument), document.Comments}, wantBatch: 2, wantStmts: 4},
		{name: "top-n and filtered", loaders: loaders{
			document.Comments.Scope(momo.LoadPerParentLimit[models.Comment](2), momo.LoadOrderBy[models.Comment](comment.ID.Desc())),
			document.Author.Scope(), document.Notes}, wantBatch: 1, wantStmts: 3},
		{name: "chunks of one relation", maxParams: 2, loaders: loaders{document.Comments}, wantBatch: 1, wantStmts: 4},
		{name: "chunks and siblings", maxParams: 2, loaders: loaders{document.Author, document.Comments}, wantBatch: 1, wantStmts: 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []momo.Option
			if tc.maxParams > 0 {
				opts = append(opts, momo.WithMaxParams(tc.maxParams))
			}
			pd, pc := relfixture.Counting(base)
			sd, sc := relfixture.Counting(relfixture.WithoutBatch(base))
			pdb, sdb := momo.New(pd, opts...), momo.New(sd, opts...)
			pdocs, sdocs := pipeDocs(t, pdb), pipeDocs(t, sdb)
			pc.Reset()
			sc.Reset()
			ctx := context.Background()
			if err := momo.LoadAll(ctx, pdb, pdocs, tc.loaders...); err != nil {
				t.Fatal(err)
			}
			if err := momo.LoadAll(ctx, sdb, sdocs, tc.loaders...); err != nil {
				t.Fatal(err)
			}
			if pipeJSON(t, pdocs) != pipeJSON(t, sdocs) {
				t.Fatalf("results differ:\n%s\n%s", pipeJSON(t, pdocs), pipeJSON(t, sdocs))
			}
			if sc.Batches.Load() != 0 || sc.Queries.Load() != tc.wantStmts {
				t.Fatalf("sequential: batches=%d queries=%d, want 0 and %d", sc.Batches.Load(), sc.Queries.Load(), tc.wantStmts)
			}
			if pc.Batches.Load() != tc.wantBatch || pc.Queries.Load() != tc.wantSingle {
				t.Fatalf("pipelined: batches=%d single=%d, want %d and %d", pc.Batches.Load(), pc.Queries.Load(), tc.wantBatch, tc.wantSingle)
			}
		})
	}
}

func TestPipelinedInTx(t *testing.T) {
	ctx := context.Background()
	drv, ctr := relfixture.Counting(pipeBase(t))
	db := momo.New(drv)

	tx, err := db.Begin(ctx, momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// A row only this transaction can see proves the batch runs on its connection.
	if _, err := momo.Exec(ctx, tx, `INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, int64(99), int64(2), "tx-only", true); err != nil {
		t.Fatal(err)
	}
	docs := pipeDocs(t, tx)
	ctr.Reset()
	if err := momo.LoadAll(ctx, tx, docs, document.Author, document.Comments); err != nil {
		t.Fatal(err)
	}
	if ctr.Batches.Load() != 1 || ctr.Queries.Load() != 0 {
		t.Fatalf("in a transaction: batches=%d queries=%d, want 1 and 0", ctr.Batches.Load(), ctr.Queries.Load())
	}
	if len(docs[1].Comments) != 1 || docs[1].Comments[0].Body != "tx-only" {
		t.Fatalf("batch did not see the transaction's own write: %+v", docs[1].Comments)
	}
}

// TestPipelinedHooksMatchSequential requires the same statement events and the
// same relation events, in the same order, as the sequential path.
func TestPipelinedHooksMatchSequential(t *testing.T) {
	base := pipeBase(t)
	ctx := context.Background()

	run := func(drv momo.Driver) *relfixture.Hook {
		h := &relfixture.Hook{}
		db := momo.New(drv, momo.WithHook(h), momo.WithMaxParams(2))
		docs := pipeDocs(t, db)
		h.After, h.Events = nil, nil
		if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments, document.Tags); err != nil {
			t.Fatal(err)
		}
		return h
	}
	pipe, seq := run(base), run(relfixture.WithoutBatch(base))
	if fmt.Sprint(pipe.After) != fmt.Sprint(seq.After) {
		t.Fatalf("statement events differ:\npipelined  %v\nsequential %v", pipe.After, seq.After)
	}
	if len(pipe.After) < 4 {
		t.Fatalf("MaxParams 2 must chunk the loads, got %v", pipe.After)
	}
	if len(pipe.Events) != 3 || len(seq.Events) != 3 {
		t.Fatalf("want 3 relation events each, got %d and %d", len(pipe.Events), len(seq.Events))
	}
	for i := range pipe.Events {
		p, s := pipe.Events[i], seq.Events[i]
		if p.Name != s.Name || p.Queries != s.Queries || p.Children != s.Children || p.Parents != s.Parents || p.Err != nil {
			t.Fatalf("relation event %d differs: pipelined %+v sequential %+v", i, p, s)
		}
	}
}

func TestPipelinedStatementErrorCancelsBatch(t *testing.T) {
	ctx := context.Background()
	base := pipeBase(t)

	stub := &batchStub{Driver: base, mutate: func(qs []momo.Compiled) []momo.Compiled {
		qs[1].SQL = `SELECT * FROM momo_missing_table`
		return qs
	}}
	h := &relfixture.Hook{}
	db := momo.New(stub, momo.WithHook(h))
	docs := pipeDocs(t, db)
	// Stale associations must be cleared, never left half-loaded.
	docs[0].Author = &models.Author{Name: "stale"}
	h.After, h.Events = nil, nil

	err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments, document.Tags)
	if err == nil {
		t.Fatal("a failing statement in the batch must fail the load")
	}
	if !strings.Contains(err.Error(), "statement 1") || !strings.Contains(err.Error(), "momo_missing_table") {
		t.Fatalf("error must name the failing statement and cause: %v", err)
	}
	for i := range docs {
		if docs[i].Author != nil || docs[i].Comments != nil || docs[i].Tags != nil {
			t.Fatalf("docs[%d] kept an association after a failed batch: %+v", i, docs[i])
		}
	}
	if len(h.After) != 3 {
		t.Fatalf("want one after-hook per statement (3), got %v", h.After)
	}
	if len(h.Events) != 3 {
		t.Fatalf("want 3 relation events, got %d", len(h.Events))
	}
	for _, ev := range h.Events {
		if ev.Err == nil {
			t.Fatalf("relation event %s ended without the batch error", ev.Name)
		}
	}
	// The connection went back to the pool and the handle still works.
	if st := base.(momo.PoolStater).PoolStats(); st.AcquiredConns != 0 {
		t.Fatalf("connection leaked after a failed batch: %+v", st)
	}
	stub.mutate = nil
	if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments); err != nil {
		t.Fatalf("handle unusable after a failed batch: %v", err)
	}
	if docs[0].Author == nil || docs[0].Author.Name != "Ada" {
		t.Fatalf("reload after failure wrong: %+v", docs[0].Author)
	}
}

func TestPipelinedContextCanceled(t *testing.T) {
	base := pipeBase(t)
	drv, ctr := relfixture.Counting(base)
	db := momo.New(drv)
	docs := pipeDocs(t, db)
	ctr.Reset()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
	if ctr.Batches.Load() != 0 || ctr.Queries.Load() != 0 {
		t.Fatalf("a canceled context must send nothing: batches=%d queries=%d", ctr.Batches.Load(), ctr.Queries.Load())
	}
	if st := base.(momo.PoolStater).PoolStats(); st.AcquiredConns != 0 {
		t.Fatalf("connection leaked: %+v", st)
	}
}

// TestPipelineFallsBack covers drivers that cannot batch and drivers that
// reject a batch as unsupported: the load runs sequentially with the same result.
func TestPipelineFallsBack(t *testing.T) {
	ctx := context.Background()
	base := pipeBase(t)

	t.Run("driver without Batcher", func(t *testing.T) {
		drv, ctr := relfixture.Counting(relfixture.WithoutBatch(base))
		if _, ok := drv.(momo.Batcher); ok {
			t.Fatal("wrapper must not implement Batcher")
		}
		db := momo.New(drv)
		docs := pipeDocs(t, db)
		ctr.Reset()
		if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments); err != nil {
			t.Fatal(err)
		}
		if ctr.Batches.Load() != 0 || ctr.Queries.Load() != 2 || len(docs[0].Comments) != 3 {
			t.Fatalf("batches=%d queries=%d comments=%d", ctr.Batches.Load(), ctr.Queries.Load(), len(docs[0].Comments))
		}
	})

	t.Run("batch rejected as unsupported", func(t *testing.T) {
		stub := &batchStub{Driver: base, err: &momo.Error{Kind: momo.ErrUnsupported, Op: "batch", Err: errors.New("no batches here")}}
		h := &relfixture.Hook{}
		db := momo.New(stub, momo.WithHook(h))
		docs := pipeDocs(t, db)
		h.After, h.Events = nil, nil
		if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments); err != nil {
			t.Fatalf("an unsupported batch must fall back to sequential loading: %v", err)
		}
		if stub.sent != 1 {
			t.Fatalf("want one rejected batch attempt, got %d", stub.sent)
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" || len(docs[0].Comments) != 3 {
			t.Fatalf("fallback result wrong: %+v", docs[0])
		}
		if len(h.Events) != 2 || h.Events[0].Err != nil || h.Events[1].Err != nil {
			t.Fatalf("relation events after fallback: %+v", h.Events)
		}
	})

	t.Run("other batch errors are not retried", func(t *testing.T) {
		boom := &momo.Error{Kind: momo.ErrBusy, Op: "batch", Err: errors.New("boom")}
		db := momo.New(&batchStub{Driver: base, err: boom})
		docs := pipeDocs(t, db)
		if err := momo.LoadAll(ctx, db, docs, document.Author, document.Comments); !errors.Is(err, boom) {
			t.Fatalf("want the batch error, got %v", err)
		}
	})
}

// TestPipelinedThroughPgBouncer runs concurrent batched loads through a
// transaction-pooling PgBouncer. The batch must not rely on session state.
func TestPipelinedThroughPgBouncer(t *testing.T) {
	ctx := context.Background()
	dbname := testdb.ScratchDatabase(t, "momo_pipe_")
	admin := testopen.PostgresIn(t, dbname)
	relfixture.CreateSchema(t, admin)
	relfixture.SeedBasic(t, admin)

	pool, err := pgxpool.New(ctx, testdb.PgBouncerURLFor(t, dbname))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	drv, ctr := relfixture.Counting(pgxdriver.New(pool))
	db := momo.New(drv)

	docs := pipeDocs(t, db)
	seq := momo.New(relfixture.WithoutBatch(drv))
	if err := momo.LoadAll(ctx, seq, docs, document.Author, document.Comments, document.Tags); err != nil {
		t.Fatal(err)
	}
	want := pipeJSON(t, docs)

	const workers, iters = 8, 20
	ctr.Reset()
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				ds, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
				if err != nil {
					errs <- err
					return
				}
				if err := momo.LoadAll(ctx, db, ds, document.Author, document.Comments, document.Tags); err != nil {
					errs <- err
					return
				}
				if b, _ := json.Marshal(ds); string(b) != want {
					errs <- errors.New("batched result differs from the sequential load")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if got := ctr.Batches.Load(); got != workers*iters {
		t.Fatalf("batches=%d, want %d", got, workers*iters)
	}
}
