// SPDX-License-Identifier: Apache-2.0

package vector

import "testing"

func TestInvalidVectorSentinel(t *testing.T) {
	if ErrInvalidVector.Error() != "momo/vector: invalid vector" {
		t.Fatalf("message = %q", ErrInvalidVector.Error())
	}
	if ErrDialect.Error() != "momo/vector: type not supported by dialect" {
		t.Fatalf("message = %q", ErrDialect.Error())
	}
}
