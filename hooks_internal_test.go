// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"testing"
)

type allocModel struct{ N int }

// TestModelHookDispatchAllocations asserts the hook call path neither
// reflects nor allocates: a set hook, an unset hook and the AfterLoad
// wrapper all run without a heap allocation.
func TestModelHookDispatchAllocations(t *testing.T) {
	ctx := context.Background()
	var m allocModel
	set := func(_ context.Context, m *allocModel) error { m.N++; return nil }
	meta := &Meta[allocModel]{Table: "t", Hooks: HookSet[allocModel]{AfterLoad: set}}
	empty := &Meta[allocModel]{Table: "t"}

	cases := []struct {
		name string
		run  func()
	}{
		{"set hook", func() { _ = runModelHook(ctx, "insert", "t", set, &m) }},
		{"nil hook", func() { _ = runModelHook[allocModel](ctx, "insert", "t", nil, &m) }},
		{"afterLoad set", func() { _ = afterLoad(ctx, meta, &m) }},
		{"afterLoad unset", func() { _ = afterLoad(ctx, empty, &m) }},
	}
	for _, c := range cases {
		if n := testing.AllocsPerRun(100, c.run); n != 0 {
			t.Errorf("%s: %v allocations per call, want 0", c.name, n)
		}
	}
}
