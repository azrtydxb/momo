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

func threeRows() [][]any {
	return [][]any{
		{int64(1), int64(1), "a", true, nil},
		{int64(2), int64(1), "b", false, nil},
		{int64(3), int64(1), "c", true, nil},
	}
}

func TestIterClosesCursor(t *testing.T) {
	ctx := context.Background()

	drv := fakedriver.New(dialect.SQLite)
	drv.SetRows(threeRows())
	for _, err := range momo.Select[unitDocument](momo.New(drv)).Iter(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if drv.Closed() != 1 {
		t.Fatalf("early break: closed=%d, want 1", drv.Closed())
	}

	drv2 := fakedriver.New(dialect.SQLite)
	drv2.SetRows(threeRows())
	boom := errors.New("callback failed")
	err := momo.Select[unitDocument](momo.New(drv2)).Each(ctx, func(*unitDocument) error { return boom })
	if !errors.Is(err, boom) || drv2.Closed() != 1 {
		t.Fatalf("callback error: err=%v closed=%d", err, drv2.Closed())
	}

	drv3 := fakedriver.New(dialect.SQLite)
	drv3.SetRows(threeRows())
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = momo.Select[unitDocument](momo.New(drv3)).Each(cctx, func(*unitDocument) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || drv3.Closed() != 1 {
		t.Fatalf("cancel mid-iteration: err=%v closed=%d, want context.Canceled and 1", err, drv3.Closed())
	}
}

func TestEmptyResultNotNotFound(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.SQLite))
	docs, err := momo.Select[unitDocument](db).All(ctx)
	if err != nil || len(docs) != 0 {
		t.Fatalf("All on zero rows = (%v, %v), want empty and nil error", docs, err)
	}
	if _, err := momo.Select[unitDocument](db).One(ctx); !errors.Is(err, momo.ErrNotFound) {
		t.Fatalf("One on zero rows = %v, want ErrNotFound", err)
	}
}
