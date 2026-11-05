// SPDX-License-Identifier: Apache-2.0

package unsupportedtype

import "github.com/azrtydxb/momo"

type Odd struct {
	momo.Model
	ID    int64
	Value complex128
}
