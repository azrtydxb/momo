// SPDX-License-Identifier: Apache-2.0

//go:build integration && cgo && !windows

package sqlshared_test

import "github.com/azrtydxb/momo/driver/sqlitedriver"

func init() {
	openers = append(openers, opener{"sqlitedriver", sqlitedriver.Open})
}
