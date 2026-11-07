// SPDX-License-Identifier: Apache-2.0

package testopen

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

func TestEachNamesSubtestAfterDialect(t *testing.T) {
	Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		if !strings.HasSuffix(t.Name(), "/"+string(d)) {
			t.Fatalf("subtest %q is not named after dialect %q", t.Name(), d)
		}
		if db.Dialect().Name != d {
			t.Fatalf("db dialect %q, subtest dialect %q", db.Dialect().Name, d)
		}
	})
}
