// SPDX-License-Identifier: Apache-2.0

package sqlerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsNoSuchTable(t *testing.T) {
	for _, c := range []struct {
		err  error
		name string
		want bool
	}{
		{nil, "t", false},
		{errors.New("no such table: t"), "t", true},
		{fmt.Errorf("wrapped: %w", errors.New("no such table: main.t")), "t", true},
		{errors.New("no such table: other"), "t", false},
	} {
		if got := IsNoSuchTable(c.err, c.name); got != c.want {
			t.Errorf("IsNoSuchTable(%v, %q) = %v, want %v", c.err, c.name, got, c.want)
		}
	}
}
