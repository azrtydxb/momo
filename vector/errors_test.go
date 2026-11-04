// SPDX-License-Identifier: Apache-2.0

package vector

import "testing"

func TestInvalidVectorSentinel(t *testing.T) {
	if ErrInvalidVector.Error() != "vector: invalid vector" {
		t.Fatalf("message = %q", ErrInvalidVector.Error())
	}
}
