// SPDX-License-Identifier: Apache-2.0

package projectionbad

import "github.com/azrtydxb/momo"

type Odd struct {
	momo.Projection
	ID    int64
	Value complex128
}
