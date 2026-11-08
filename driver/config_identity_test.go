// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/driver/sqlitepure"
)

// sqlitepure.Config is sqlitedriver.Config, so a program can switch between
// the two drivers without converting its settings. This fails to compile if
// the types ever diverge.
var _ sqlitedriver.Config = sqlitepure.Config{}
