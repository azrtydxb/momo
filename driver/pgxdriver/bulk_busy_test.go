// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// blockingSource yields one row, then blocks in Next until released.
type blockingSource struct {
	started chan struct{}
	release chan struct{}
	i       int
}

func (s *blockingSource) Next() bool {
	s.i++
	if s.i == 2 {
		close(s.started)
		<-s.release
	}
	return s.i <= 2
}
func (s *blockingSource) Values() ([]any, error) { return []any{int64(s.i)}, nil }
func (s *blockingSource) Err() error             { return nil }

func TestPinnedReleaseIsBusyWhileCopyStreams(t *testing.T) {
	d := New(openPool(t))
	ctx := context.Background()
	pc, err := d.(momo.Pinner).Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Release() }()
	if _, err := pc.Exec(ctx, "CREATE TEMP TABLE t_busy_copy (id bigint)", nil); err != nil {
		t.Fatal(err)
	}
	p := pc.(*pinned)
	src := &blockingSource{started: make(chan struct{}), release: make(chan struct{})}
	type res struct {
		n   int64
		err error
	}
	done := make(chan res, 1)
	go func() {
		n, err := p.copyFromSource(ctx, "t_busy_copy", []string{"id"}, src)
		done <- res{n, err}
	}()
	<-src.started // the copy is in flight and blocked inside the source
	if err := pc.Release(); !errors.Is(err, momo.ErrPinnedBusy) {
		t.Fatalf("Release during CopyFrom = %v, want ErrPinnedBusy", err)
	}
	close(src.release)
	r := <-done
	if r.err != nil || r.n != 2 {
		t.Fatalf("copy: n=%d err=%v", r.n, r.err)
	}
	if err := pc.Release(); err != nil {
		t.Fatalf("Release after the copy finished: %v", err)
	}
}
