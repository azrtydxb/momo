// SPDX-License-Identifier: Apache-2.0

//go:build cgo && !windows

package sqlitedriver

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

// Every allowed key must be single-valued and carry a value mattn can read; a
// malformed one would otherwise surface as a late driver error or be ignored.
func TestBuildDSNValidatesAllowedValues(t *testing.T) {
	for _, q := range []string{
		"_txlock=bogus", "_txlock=", "_busy_timeout=abc", "_busy_timeout=-1", "_busy_timeout=", "_timeout=1.5", "_timeout=-5",
		"_cache_size=abc", "_cache_size=", "_loc=No/Such_Zone",
		"_txlock=deferred&_txlock=immediate", "_busy_timeout=1&_busy_timeout=2", "_timeout=1&_timeout=2",
		"_loc=auto&_loc=UTC", "_cache_size=1&_cache_size=2",
	} {
		_, err := buildDSN("x.db?"+q, Config{})
		if !errors.Is(err, momo.ErrValidation) {
			t.Errorf("buildDSN(%q) err = %v, want ErrValidation", q, err)
		}
	}
	for _, q := range []string{"_txlock=EXCLUSIVE", "_busy_timeout=0", "_cache_size=-2000", "_cache_size=0", "_loc=auto", "_loc=UTC"} {
		if _, err := buildDSN("x.db?"+q, Config{}); err != nil {
			t.Errorf("buildDSN(%q) refused: %v", q, err)
		}
	}
}
