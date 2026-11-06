// SPDX-License-Identifier: Apache-2.0

// Package models is the schema of the vector-search example: items with an
// embedding, indexed for nearest-neighbour search.
package models

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

// Item is a searchable thing. The embedding has 3 dimensions to keep the
// example readable; set dim to the output size of your model, for example
// 1536. A vector(n) column holds up to 16000 dimensions, an HNSW or IVFFlat
// index up to 2000 (use vector.HalfVector for up to 4000).
type Item struct {
	momo.Model `momo:"table:items"`
	ID         int64         `momo:"pk,identity"`
	Title      string        `momo:"size:200,notnull"`
	Category   string        `momo:"size:50,notnull,index"`
	Embedding  vector.Vector `momo:"dim:3"`
}

// MomoConfig declares the vector index. pgvector's hnsw method has no default
// operator class, so name the one that matches the distance you query with:
// vector_cosine_ops serves CosineDistance, vector_l2_ops serves L2Distance,
// vector_ip_ops serves InnerProductDistance. An index whose operator class
// differs from the query's distance is silently not used.
//
// For an IVFFlat index (faster to build, smaller, needs data first) write
// Method("ivfflat").Opclass(&i.Embedding, "vector_cosine_ops").With("lists",
// "100"): ivfflat requires an explicit lists value.
//
// The generator reads this method statically; it never runs.
func (i *Item) MomoConfig(c *momo.Config) {
	c.Index("items_embedding_hnsw", &i.Embedding).
		Method("hnsw").
		Opclass(&i.Embedding, "vector_cosine_ops").
		With("m", "16").
		With("ef_construction", "64")
}

// Hit is a projection: an item's title together with its distance from the
// query, which no column holds. See the projection-pagination example for
// how projections work; the generator does not emit them yet.
type Hit struct {
	momo.Projection
	Title    string
	Distance float64
}

// MomoMeta describes Hit to Momo.
func (*Hit) MomoMeta() *momo.Meta[Hit] { return &hitMeta }

var hitMeta = momo.Meta[Hit]{
	Columns: []momo.ColumnMeta{{Name: "title"}, {Name: "distance"}},
	Ptr: func(m *Hit, i int) any {
		if i == 0 {
			return &m.Title
		}
		return &m.Distance
	},
	Value:    func(*Hit, int) any { return nil },
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
}
