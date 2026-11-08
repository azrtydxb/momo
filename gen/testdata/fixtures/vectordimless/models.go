// SPDX-License-Identifier: Apache-2.0

// Package vectordimless exercises a vector field declared without dim: a
// store whose embedding size depends on the configured model, with the rows
// filtered by a dims column.
package vectordimless

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

type Chunk struct {
	momo.Model `momo:"table:kb_chunks"`
	ID         int64 `momo:"pk,identity"`
	Dims       int
	Embedding  vector.Vector
}
