// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"errors"
	"testing"
)

func TestSentinelsAreDistinct(t *testing.T) {
	all := []error{ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict, ErrStagedRequired, ErrNoDown, ErrNoSnapshot, ErrRebuildBlocked}
	for i := range all {
		for j := range all {
			if i != j && errors.Is(all[i], all[j]) {
				t.Fatalf("sentinels %d and %d are equal", i, j)
			}
		}
	}
	var d DriftReport
	if !d.Clean() {
		t.Fatal("empty drift report must be clean")
	}
}
