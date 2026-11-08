// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestServerVersionSQLDBRejectedArgument(t *testing.T) {
	_, err := FromSQLDB(nil).(momo.ServerVersioner).ServerVersion(context.Background())
	if err == nil {
		t.Fatal("ServerVersion on a rejected *sql.DB returned nil error")
	}
}
