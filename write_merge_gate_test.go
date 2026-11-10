// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"testing"
)

func TestMergeGate(t *testing.T) {
	cases := []struct {
		version   int
		returning bool
		ok        bool
	}{
		{140013, false, false}, // below PostgreSQL 15: no MERGE
		{150000, false, true},  // MERGE without RETURNING
		{150000, true, false},  // RETURNING needs 17
		{160004, true, false},
		{170000, true, true},
		{170000, false, true},
	}
	for _, c := range cases {
		err := mergeGate(c.version, c.returning)
		if (err == nil) != c.ok {
			t.Fatalf("mergeGate(%d, %v) = %v, want ok=%v", c.version, c.returning, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrUnsupported) {
			t.Fatalf("mergeGate(%d, %v) error %v is not ErrUnsupported", c.version, c.returning, err)
		}
	}
}
