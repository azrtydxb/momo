// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"context"
	"testing"
)

func nplusoneEvent(sql string) *QueryEvent { return &QueryEvent{Op: "select", SQL: sql} }

func TestNPlusOneReportPanicRecovered(t *testing.T) {
	h := NewNPlusOneHook(1, func(NPlusOneFinding) { panic("boom") })
	ctx := WithQueryScope(context.Background())
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped the hook: %v", r)
		}
	}()
	for i := 0; i < 3; i++ {
		if got := h.BeforeQuery(ctx, nplusoneEvent(`SELECT 1`)); got == nil {
			t.Fatal("BeforeQuery returned a nil context")
		}
	}
}
