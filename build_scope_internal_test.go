// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestPopScopeUnderflowSetsError(t *testing.T) {
	b := NewBuilder(dialect.For(dialect.Postgres))
	b.popScope()
	if err := b.Err(); !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "scope stack underflow") {
		t.Fatalf("popScope on an empty stack must record an internal error, got %v", err)
	}
	// Balanced use stays clean.
	b = NewBuilder(dialect.For(dialect.Postgres))
	b.withScope([]string{"t"}, func() {})
	if b.Err() != nil {
		t.Fatalf("balanced scope: %v", b.Err())
	}
}
