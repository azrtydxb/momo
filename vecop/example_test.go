// SPDX-License-Identifier: Apache-2.0

package vecop_test

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vecop"
	"github.com/azrtydxb/momo/vector"
)

// Cosine renders the pgvector operator on PostgreSQL and the sqlite-vec
// function on SQLite. The query vector is bound as a parameter.
func ExampleCosine() {
	emb := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	query := vector.Vector{1, 0, 0}

	for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
		b := momo.NewBuilder(dialect.For(d))
		vecop.Cosine(emb, query).AppendSQL(b)
		fmt.Println(b.String())
	}
	// Output:
	// ("docs"."emb" <=> $1)
	// vec_distance_cosine("docs"."emb", ?)
}

// Desc orders farthest first; Asc, the default for a distance, nearest first.
func ExampleExpr_Desc() {
	emb := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)

	b := momo.NewBuilder(dialect.For(dialect.Postgres))
	vecop.L2(emb, vector.Vector{1, 2, 3}).Desc().AppendOrder(b)
	fmt.Println(b.String())
	// Output:
	// ("docs"."emb" <-> $1) DESC
}

// A metric the dialect lacks fails the build with momo.ErrUnsupported.
func ExampleInnerProduct() {
	emb := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)

	b := momo.NewBuilder(dialect.For(dialect.SQLite))
	vecop.InnerProduct(emb, vector.Vector{1, 2, 3}).AppendSQL(b)
	fmt.Println(errors.Is(b.Err(), momo.ErrUnsupported))
	// Output:
	// true
}
