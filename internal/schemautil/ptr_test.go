// SPDX-License-Identifier: Apache-2.0

package schemautil_test

import (
	"testing"

	"github.com/azrtydxb/momo/internal/schemautil"
)

// Catches: Int64 returning a shared pointer or the wrong value.
func TestInt64(t *testing.T) {
	a, b := schemautil.Int64(7), schemautil.Int64(7)
	if *a != 7 || a == b {
		t.Fatalf("Int64(7) = %d, same pointer %v", *a, a == b)
	}
}
