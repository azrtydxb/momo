// SPDX-License-Identifier: Apache-2.0

package model

import "github.com/azrtydxb/momo"

type Item struct {
	momo.Model `momo:"table:items"`
	ID         int64  `momo:"pk,identity"`
	Code       string `momo:"read_only"`
	Owner      string `momo:"create_only"`
	Serial     string `momo:"immutable"`
	Name       string
}
