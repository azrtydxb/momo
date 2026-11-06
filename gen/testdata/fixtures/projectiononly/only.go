// SPDX-License-Identifier: Apache-2.0

package projectiononly

import "github.com/azrtydxb/momo"

type Count struct {
	momo.Projection
	N int64
}
