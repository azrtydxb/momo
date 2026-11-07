// SPDX-License-Identifier: Apache-2.0

package testopen

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// countHook counts statements reaching the hook.
type countHook struct{ n int }

func (c *countHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *countHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {}

func TestEachNamesSubtestAfterDialect(t *testing.T) {
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !strings.HasSuffix(t.Name(), "/"+string(d)) {
			t.Fatalf("subtest %q is not named after dialect %q", t.Name(), d)
		}
		if db.Dialect().Name != d {
			t.Fatalf("db dialect %q, subtest dialect %q", db.Dialect().Name, d)
		}
	})
}

// poolWaitHook keeps the largest PoolWait it sees.
type poolWaitHook struct{ maxNS atomic.Int64 }

func (h *poolWaitHook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context {
	return ctx
}

func (h *poolWaitHook) AfterQuery(ctx context.Context, e *momo.QueryEvent) {
	for {
		cur := h.maxNS.Load()
		if int64(e.PoolWait) <= cur || h.maxNS.CompareAndSwap(cur, int64(e.PoolWait)) {
			return
		}
	}
}
