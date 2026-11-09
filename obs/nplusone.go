// SPDX-License-Identifier: Apache-2.0

package obs

import (
	"context"
	"sync"

	"github.com/azrtydxb/momo"
)

// Finding reports a relation that was loaded more often than the threshold
// within one scope.
type Finding struct {
	// Relation is the parent table and relation name, "documents.Author".
	Relation string
	// Count is the number of loads when the threshold was first exceeded.
	Count int
	// Parents is the number of parent rows of the load that crossed the
	// threshold. A value of 1 is the usual sign of a load inside a loop.
	Parents int
}

// maxScopeRelations bounds the memory one scope can hold.
const maxScopeRelations = 1024

type scopeKey struct{}

type scope struct {
	mu       sync.Mutex
	counts   map[string]int
	reported map[string]bool
}

// WithScope returns a context that groups relation loads for NPlusOne. Wrap
// one unit of work (an HTTP request, a job) and pass the context to every
// load in it. A context without a scope is not tracked.
func WithScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, scopeKey{}, &scope{counts: map[string]int{}, reported: map[string]bool{}})
}

// NPlusOne is a development hook that finds the N+1 pattern: one query loads
// the parents, then a loop loads a relation once per parent. It counts relation
// loads (momo.Load, LoadAll, Preload) per relation in a scope created with
// WithScope and reports a Finding once per relation and scope when the count
// exceeds the threshold. A single batched load is one load however many
// parents it covers, so it never triggers a finding.
//
// It implements momo.Hook and momo.RelationHook, so one momo.WithHook call
// registers it. It is safe for concurrent use.
type NPlusOne struct {
	threshold int
	report    func(Finding)
}

var (
	_ momo.Hook         = (*NPlusOne)(nil)
	_ momo.RelationHook = (*NPlusOne)(nil)
)

// NewNPlusOne returns a detector that calls report when a relation has been
// loaded more than threshold times in one scope (a threshold below 1 counts as
// 1). report runs synchronously on the loading goroutine and must not query
// the database; a panic in report is recovered and dropped. A scope tracks at
// most 1024 distinct relations.
func NewNPlusOne(threshold int, report func(Finding)) *NPlusOne {
	if threshold < 1 {
		threshold = 1
	}
	return &NPlusOne{threshold: threshold, report: report}
}

// BeforeQuery does nothing.
func (n *NPlusOne) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context { return ctx }

// AfterQuery does nothing.
func (n *NPlusOne) AfterQuery(context.Context, *momo.QueryEvent) {}

// BeforeRelation counts the load and reports when the threshold is exceeded.
func (n *NPlusOne) BeforeRelation(ctx context.Context, e *momo.RelationEvent) context.Context {
	sc, ok := ctx.Value(scopeKey{}).(*scope)
	if !ok {
		return ctx
	}
	key := e.Parent + "." + e.Name
	sc.mu.Lock()
	if _, seen := sc.counts[key]; !seen && len(sc.counts) >= maxScopeRelations {
		sc.mu.Unlock()
		return ctx
	}
	sc.counts[key]++
	count := sc.counts[key]
	fire := count > n.threshold && !sc.reported[key]
	if fire {
		sc.reported[key] = true
	}
	sc.mu.Unlock()
	if fire {
		n.safeReport(Finding{Relation: key, Count: count, Parents: e.Parents})
	}
	return ctx
}

// AfterRelation does nothing.
func (n *NPlusOne) AfterRelation(context.Context, *momo.RelationEvent) {}

// safeReport calls report and drops a panic, so a faulty reporter cannot break
// the load that triggered it.
func (n *NPlusOne) safeReport(f Finding) {
	defer func() { _ = recover() }()
	if n.report != nil {
		n.report(f)
	}
}
