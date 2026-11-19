// SPDX-License-Identifier: Apache-2.0

//go:build !cgo || windows

package sqlitedriver

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// The stub build exports the same names as the cgo build, so callers compile
// either way; Open fails and New returns a driver that fails.
func TestStubSurface(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, ":memory:", WithConfig(Config{})); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Open err = %v, want ErrUnsupported", err)
	}
	d := New(nil)
	if _, err := d.Query(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("New(nil).Query err = %v, want ErrUnsupported", err)
	}
	if _, err := d.Exec(ctx, "SELECT 1", nil); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("New(nil).Exec err = %v, want ErrUnsupported", err)
	}
	if _, err := d.Begin(ctx, momo.TxOptions{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("New(nil).Begin err = %v, want ErrUnsupported", err)
	}
	in := errors.New("boom")
	if got := ClassifyError(in); got != in {
		t.Fatalf("ClassifyError changed the error: %v", got)
	}
	if ClassifyError(nil) != nil {
		t.Fatal("ClassifyError(nil) != nil")
	}
	if VecVersion != "" {
		t.Fatalf("VecVersion = %q, want the zero value", VecVersion)
	}
}
