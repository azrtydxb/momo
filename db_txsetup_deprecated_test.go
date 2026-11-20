// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// Catches: the deprecated SetConfigLocal doing something else than Tx.SetLocal:
// both refuse a dialect without transaction-scoped settings the same way.
func TestSetConfigLocalForwardsToSetLocal(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.SQLite))
	tx, err := db.Begin(t.Context(), momo.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	want := tx.SetLocal(t.Context(), "app.tenant_id", "1")
	got := momo.SetConfigLocal(t.Context(), tx, "app.tenant_id", "1")
	if !errors.Is(want, momo.ErrUnsupported) || !errors.Is(got, momo.ErrUnsupported) {
		t.Fatalf("SetLocal = %v, SetConfigLocal = %v, want ErrUnsupported from both", want, got)
	}
	if got.Error() != want.Error() {
		t.Fatalf("SetConfigLocal error %q differs from SetLocal error %q", got, want)
	}
}
