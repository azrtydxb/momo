// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

// TestIDGenReconcileDoesNotRunAfterLoad: the write path reads the generated
// ids back from internal RETURNING rows into half-filled models. The user's
// AfterLoad hook must not run on them: a hook that normalises a conflict-target
// column would break the row matching, and a failing one would fail the write.
func TestIDGenReconcileDoesNotRunAfterLoad(t *testing.T) {
	name := momo.NewColumn[string]("id_rows", "name", 0)
	hooks := map[string]func(*int) func(context.Context, *idRow) error{
		"mutating": func(n *int) func(context.Context, *idRow) error {
			return func(_ context.Context, m *idRow) error { *n++; m.Name = strings.ToUpper(m.Name); return nil }
		},
		"erroring": func(n *int) func(context.Context, *idRow) error {
			return func(context.Context, *idRow) error { *n++; return errors.New("hook failed") }
		},
	}
	for label, mk := range hooks {
		t.Run(label, func(t *testing.T) {
			var calls int
			idRowMeta.Hooks = momo.HookSet[idRow]{AfterLoad: mk(&calls)}
			t.Cleanup(func() { idRowMeta.Hooks = momo.HookSet[idRow]{} })

			drv := fakedriver.New(dialect.Postgres)
			drv.SetRows([][]any{{"id-from-db", "a"}})
			db := momo.New(drv)
			m := idRow{Name: "a"}
			if _, err := momo.Insert[idRow](db).Model(&m).OnConflict(name).DoNothing().Exec(t.Context()); err != nil {
				t.Fatalf("insert with a %s AfterLoad: %v", label, err)
			}
			if calls != 0 {
				t.Fatalf("AfterLoad ran %d times on internal RETURNING rows", calls)
			}
			if m.ID != "id-from-db" {
				t.Fatalf("id not written back: %q", m.ID)
			}
		})
	}
}
