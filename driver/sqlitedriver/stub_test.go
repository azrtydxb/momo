// SPDX-License-Identifier: Apache-2.0

//go:build !cgo || windows

package sqlitedriver

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// The stub build exports the same names as the cgo build, so callers compile
// either way; Open is the only one that fails.
func TestStubSurface(t *testing.T) {
	if _, _, err := Open(":memory:", Config{}); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("Open err = %v, want ErrUnsupported", err)
	}
	in := errors.New("boom")
	if got := ClassifyError(in); got != in {
		t.Fatalf("ClassifyError changed the error: %v", got)
	}
	if ClassifyError(nil) != nil {
		t.Fatal("ClassifyError(nil) != nil")
	}
	if SQLiteVecVersion != "" {
		t.Fatalf("SQLiteVecVersion = %q, want the zero value", SQLiteVecVersion)
	}
}
