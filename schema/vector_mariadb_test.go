// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestMariaDBVectorIndexRules(t *testing.T) {
	tbl := func(typ string, nullable bool) *Table {
		return &Table{Name: "docs", Columns: []Column{{Name: "emb", Type: typ, Nullable: nullable}}}
	}
	part := []IndexPart{{Column: "emb"}}
	vec := func(with ...KV) Index {
		return Index{Name: "docs_emb", Method: "vector", Parts: part, With: with}
	}
	cases := []struct {
		name string
		tbl  *Table
		idx  Index
		ok   bool
	}{
		{"defaults", tbl("vector(8)", false), vec(), true},
		{"cosine m 8", tbl("vector(8)", false), vec(KV{"m", "8"}, KV{"distance", "cosine"}), true},
		{"euclidean", tbl("vector(8)", false), vec(KV{"distance", "euclidean"}), true},
		{"m lower bound", tbl("vector(8)", false), vec(KV{"m", "3"}), true},
		{"m upper bound", tbl("vector(8)", false), vec(KV{"m", "200"}), true},
		{"m too small", tbl("vector(8)", false), vec(KV{"m", "2"}), false},
		{"m too large", tbl("vector(8)", false), vec(KV{"m", "201"}), false},
		{"m not a number", tbl("vector(8)", false), vec(KV{"m", "x"}), false},
		{"distance inner product", tbl("vector(8)", false), vec(KV{"distance", "ip"}), false},
		{"unknown key", tbl("vector(8)", false), vec(KV{"ef_construction", "64"}), false},
		{"nullable column", tbl("vector(8)", true), vec(), false},
		{"not a vector column", tbl("text", false), vec(), false},
		{"halfvec column", tbl("halfvec(8)", false), vec(), false},
		{"unsized vector", tbl("vector", false), vec(), false},
		{"hnsw method", tbl("vector(8)", false), Index{Name: "x", Method: "hnsw", Parts: part}, false},
		{"ivfflat method", tbl("vector(8)", false), Index{Name: "x", Method: "ivfflat", Parts: part, With: []KV{{"lists", "10"}}}, false},
		{"opclass is a pgvector notion", tbl("vector(8)", false), Index{Name: "x", Method: "vector", Parts: []IndexPart{{Column: "emb", OpClass: "vector_l2_ops"}}}, false},
		{"expression part", tbl("vector(8)", false), Index{Name: "x", Method: "vector", Parts: []IndexPart{{Expr: "emb"}}}, false},
		{"unique", tbl("vector(8)", false), Index{Name: "x", Method: "vector", Parts: part, Unique: true}, false},
		{"partial", tbl("vector(8)", false), Index{Name: "x", Method: "vector", Parts: part, Where: "id > 1"}, false},
	}
	for _, c := range cases {
		err := ValidateMariaDBVectorIndex(c.tbl, c.idx)
		if c.ok && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrInvalidVectorIndex) {
			t.Errorf("%s: err = %v, want ErrInvalidVectorIndex", c.name, err)
		}
	}
	// The pgvector validator keeps refusing MySQL-family dialects.
	if err := ValidateVectorIndex(dialect.MySQL, tbl("vector(8)", false), vec()); !errors.Is(err, ErrInvalidVectorIndex) {
		t.Errorf("ValidateVectorIndex on mysql: err = %v, want ErrInvalidVectorIndex", err)
	}
}

func TestMariaDBVectorIndexClause(t *testing.T) {
	tbl := &Table{Name: "docs", Columns: []Column{{Name: "emb", Type: "vector(8)"}}}
	idx := Index{Name: "docs_emb", Method: "vector", Parts: []IndexPart{{Column: "emb"}}, With: []KV{{"distance", "cosine"}, {"m", "8"}}}
	got, err := MariaDBVectorIndexClause(tbl, idx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "VECTOR INDEX `docs_emb` (`emb`) M=8 DISTANCE=cosine"; got != want {
		t.Fatalf("clause = %s, want %s", got, want)
	}
	plain, err := MariaDBVectorIndexClause(tbl, Index{Name: "a`b", Method: "vector", Parts: []IndexPart{{Column: "emb"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "VECTOR INDEX `a``b` (`emb`)"; plain != want {
		t.Fatalf("clause = %s, want %s", plain, want)
	}
	bad := Index{Name: "x", Method: "vector", Parts: []IndexPart{{Column: "emb"}}, With: []KV{{"m", "1"}}}
	if _, err := MariaDBVectorIndexClause(tbl, bad); !errors.Is(err, ErrInvalidVectorIndex) {
		t.Fatalf("invalid index: err = %v, want ErrInvalidVectorIndex", err)
	}
}
