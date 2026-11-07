// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo/migrate"
)

func TestNewEngineRequiresDialect(t *testing.T) {
	_, err := migrate.NewEngine(nil, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "migrate.WithDialect") {
		t.Fatalf("want an error naming migrate.WithDialect, got %v", err)
	}
}
