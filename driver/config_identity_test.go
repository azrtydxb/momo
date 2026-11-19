// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/driver/sqlitepure"
)

// sqlitepure.Config and sqlitedriver.Config are separate types with the same
// fields, so a program that switches between the two drivers converts its
// settings in one expression. This fails to compile if the fields ever
// diverge.
var (
	_ = sqlitedriver.Config(sqlitepure.Config{})
	_ = sqlitepure.Config(sqlitedriver.Config{})
)
