// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"
)

func TestMigrateSentinelMessages(t *testing.T) {
	for _, err := range []error{ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict} {
		if !strings.HasPrefix(err.Error(), "migrate: ") {
			t.Errorf("%q lacks the migrate: prefix", err.Error())
		}
	}
}
