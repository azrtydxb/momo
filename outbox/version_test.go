// SPDX-License-Identifier: Apache-2.0

package outbox

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Catches: a MySQL or MariaDB server without SKIP LOCKED being let through, a
// supported one being refused, and a version string that cannot be read being
// guessed at.
func TestSkipLockedSupported(t *testing.T) {
	cases := []struct {
		flavor  dialect.Flavor
		version string
		ok      bool
	}{
		{dialect.FlavorMySQL, "8.4.3", true},
		{dialect.FlavorMySQL, "9.7.0-log", true},
		{dialect.FlavorMySQL, "8.0.36", true},
		{dialect.FlavorMySQL, "5.7.44", false},
		{dialect.FlavorMySQL, "", false},
		{dialect.FlavorMySQL, "garbage", false},
		{dialect.FlavorMariaDB, "11.8.2-MariaDB", true},
		{dialect.FlavorMariaDB, "5.5.5-10.6.18-MariaDB-ubu2204", true},
		{dialect.FlavorMariaDB, "10.5.25-MariaDB", false},
		{dialect.FlavorMariaDB, "5.5.5-10.3.39-MariaDB", false},
	}
	for _, c := range cases {
		err := skipLockedSupported(c.flavor, c.version)
		if c.ok && err != nil {
			t.Errorf("%s %q: %v, want supported", c.flavor, c.version, err)
		}
		if !c.ok && !errors.Is(err, momo.ErrUnsupported) {
			t.Errorf("%s %q: err = %v, want ErrUnsupported", c.flavor, c.version, err)
		}
	}
}
