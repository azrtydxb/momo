// SPDX-License-Identifier: Apache-2.0

// Package vectorperm exercises vector fields under each write permission,
// vector dimensions taken from explicit type tags, and an alias to a pointer
// vector type.
package vectorperm

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// MaybeVector is an alias to a pointer type.
type MaybeVector = *vector.Vector

type Chunk struct {
	momo.Model `momo:"table:chunks"`
	ID         int64               `momo:"pk,identity"`
	Generated  vector.Vector       `momo:"read_only,dim:3"`
	GenNull    *vector.Vector      `momo:"read_only,dim:3"`
	Once       vector.Vector       `momo:"create_only,dim:3"`
	Frozen     vector.Vector       `momo:"immutable,dim:3"`
	Plain      vector.Vector       `momo:"dim:3"`
	Maybe      *vector.Vector      `momo:"dim:3"`
	ViaAlias   MaybeVector         `momo:"dim:3"`
	TypeVec    vector.Vector       `momo:"type:vector(3)"`
	TypeHalf   vector.HalfVector   `momo:"type:halfvec(4)"`
	TypeSparse vector.SparseVector `momo:"type:sparsevec(5)"`
	TypeBits   vector.Bits         `momo:"type:bit(8)"`
}
