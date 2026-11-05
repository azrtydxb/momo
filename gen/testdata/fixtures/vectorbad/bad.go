// SPDX-License-Identifier: Apache-2.0

package vectorbad

import "github.com/azrtydxb/momo"

// Vector stands in for vector.Vector.
type Vector []float32

type Doc struct {
	momo.Model `momo:"table:docs"`
	ID         int64
	Embedding  Vector `momo:"dim:3"`
}

func (d *Doc) MomoConfig(c *momo.Config) {
	c.Index("docs_emb_hnsw", &d.Embedding).Method("hnsw").Opclass(&d.Embedding, "vector_cosine_ops").With("m", "200")
}

type Tagged struct {
	momo.Model `momo:"table:tagged"`
	ID         int64
	Embedding  Vector `momo:"dim:3,index,method:hnsw"`
}
