// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestVectorIndexRules(t *testing.T) {
	tbl := func(typ string) *Table {
		return &Table{Name: "docs", Columns: []Column{{Name: "emb", Type: typ}}}
	}
	hnsw := func(op string, with ...KV) Index {
		return Index{Name: "docs_emb", Method: "hnsw", Parts: []IndexPart{{Column: "emb", OpClass: op}}, With: with}
	}
	ivf := func(op string, with ...KV) Index {
		return Index{Name: "docs_emb", Method: "ivfflat", Parts: []IndexPart{{Column: "emb", OpClass: op}}, With: with}
	}
	cases := []struct {
		name string
		typ  string
		idx  Index
		ok   bool
	}{
		{"hnsw vector cosine ok", "vector(1536)", hnsw("vector_cosine_ops"), true},
		{"hnsw vector over 2000 dims", "vector(2001)", hnsw("vector_cosine_ops"), false},
		{"hnsw vector with halfvec opclass", "vector(1536)", hnsw("halfvec_cosine_ops"), false},
		{"hnsw halfvec up to 4000", "halfvec(4000)", hnsw("halfvec_l2_ops"), true},
		{"hnsw halfvec over 4000", "halfvec(4001)", hnsw("halfvec_l2_ops"), false},
		{"hnsw bit hamming", "bit(64000)", hnsw("bit_hamming_ops"), true},
		{"hnsw bit with vector opclass", "bit(8)", hnsw("vector_cosine_ops"), false},
		{"ivfflat vector with lists", "vector(768)", ivf("vector_l2_ops", KV{"lists", "100"}), true},
		{"ivfflat vector without lists", "vector(768)", ivf("vector_l2_ops"), false},
		{"ivfflat sparsevec rejected", "sparsevec(100)", ivf("sparsevec_l2_ops", KV{"lists", "10"}), false},
		{"ivfflat bit jaccard rejected", "bit(8)", ivf("bit_jaccard_ops", KV{"lists", "10"}), false},
		{"unknown with key", "vector(3)", hnsw("vector_l2_ops", KV{"ef", "10"}), false},
	}
	for _, c := range cases {
		err := ValidateVectorIndex(dialect.Postgres, tbl(c.typ), c.idx)
		if c.ok && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrInvalidVectorIndex) {
			t.Errorf("%s: err = %v, want ErrInvalidVectorIndex", c.name, err)
		}
	}
}

func TestVectorIndexRulesMore(t *testing.T) {
	tbl := func(typ string) *Table {
		return &Table{Name: "docs", Columns: []Column{{Name: "emb", Type: typ}}}
	}
	part := func(op string) []IndexPart { return []IndexPart{{Column: "emb", OpClass: op}} }
	pg := dialect.Postgres
	l2 := part("vector_l2_ops")
	cases := []struct {
		name string
		d    dialect.Name
		typ  string
		idx  Index
		ok   bool
	}{
		{"sqlite rejected", dialect.SQLite, "vector(3)", Index{Method: "hnsw", Parts: l2}, false},
		{"mysql rejected", dialect.MySQL, "vector(3)", Index{Method: "hnsw", Parts: l2}, false},
		{"hnsw sparsevec ok", pg, "sparsevec(100)", Index{Method: "hnsw", Parts: part("sparsevec_l2_ops")}, true},
		{"hnsw vector l1 ok", pg, "vector(3)", Index{Method: "hnsw", Parts: part("vector_l1_ops")}, true},
		{"hnsw bit over 64000", pg, "bit(64001)", Index{Method: "hnsw", Parts: part("bit_hamming_ops")}, false},
		{"hnsw missing opclass", pg, "vector(3)", Index{Method: "hnsw", Parts: part("")}, false},
		{"hnsw params ok", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "16"}, {"ef_construction", "64"}}}, true},
		{"hnsw m too small", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "1"}}}, false},
		{"hnsw m too large", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "101"}}}, false},
		{"hnsw ef below 2m", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "16"}, {"ef_construction", "31"}}}, false},
		{"hnsw ef over 1000", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"ef_construction", "1001"}}}, false},
		{"hnsw m not a number", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "x"}}}, false},
		{"hnsw with lists", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"lists", "10"}}}, false},
		{"ivfflat with m", pg, "vector(3)", Index{Method: "ivfflat", Parts: l2, With: []KV{{"lists", "10"}, {"m", "16"}}}, false},
		{"ivfflat lists zero", pg, "vector(3)", Index{Method: "ivfflat", Parts: l2, With: []KV{{"lists", "0"}}}, false},
		{"ivfflat lists over 32768", pg, "vector(3)", Index{Method: "ivfflat", Parts: l2, With: []KV{{"lists", "32769"}}}, false},
		{"ivfflat default opclass empty", pg, "vector(3)", Index{Method: "ivfflat", Parts: part(""), With: []KV{{"lists", "10"}}}, true},
		{"ivfflat halfvec ok", pg, "halfvec(4000)", Index{Method: "ivfflat", Parts: part("halfvec_cosine_ops"), With: []KV{{"lists", "10"}}}, true},
		{"ivfflat halfvec over 4000", pg, "halfvec(4001)", Index{Method: "ivfflat", Parts: part("halfvec_l2_ops"), With: []KV{{"lists", "10"}}}, false},
		{"ivfflat vector over 2000", pg, "vector(2001)", Index{Method: "ivfflat", Parts: l2, With: []KV{{"lists", "10"}}}, false},
		{"ivfflat bit hamming", pg, "bit(8)", Index{Method: "ivfflat", Parts: part("bit_hamming_ops"), With: []KV{{"lists", "10"}}}, true},
		{"ivfflat vector with halfvec ops", pg, "vector(3)", Index{Method: "ivfflat", Parts: part("halfvec_l2_ops"), With: []KV{{"lists", "10"}}}, false},
		{"expression part", pg, "vector(3)", Index{Method: "hnsw", Parts: []IndexPart{{Expr: "emb::halfvec(3)", OpClass: "halfvec_l2_ops"}}}, false},
		{"two parts", pg, "vector(3)", Index{Method: "hnsw", Parts: []IndexPart{{Column: "emb", OpClass: "vector_l2_ops"}, {Column: "emb", OpClass: "vector_l2_ops"}}}, false},
		{"not a vector column", pg, "text", Index{Method: "hnsw", Parts: l2}, false},
		{"unknown column", pg, "vector(3)", Index{Method: "hnsw", Parts: []IndexPart{{Column: "nope", OpClass: "vector_l2_ops"}}}, false},
		{"non vector method", pg, "vector(3)", Index{Method: "gin", Parts: part("")}, false},
		{"unsized vector type", pg, "vector", Index{Method: "hnsw", Parts: l2}, false},
		{"unsized halfvec type", pg, "halfvec", Index{Method: "ivfflat", Parts: part("halfvec_l2_ops"), With: []KV{{"lists", "10"}}}, false},
		{"unsized sparsevec type", pg, "sparsevec", Index{Method: "hnsw", Parts: part("sparsevec_l2_ops")}, false},
		{"unsized bit hnsw", pg, "bit", Index{Method: "hnsw", Parts: part("bit_hamming_ops")}, true},
		{"unsized bit ivfflat", pg, "bit", Index{Method: "ivfflat", Parts: part("bit_hamming_ops"), With: []KV{{"lists", "10"}}}, true},
		{"hnsw m 100 default ef", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "100"}}}, false},
		{"hnsw m 32 default ef", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "32"}}}, true},
		{"hnsw m 2 ef 4", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "2"}, {"ef_construction", "4"}}}, true},
		{"hnsw m 2 default ef", pg, "vector(3)", Index{Method: "hnsw", Parts: l2, With: []KV{{"m", "2"}}}, true},
	}
	for _, c := range cases {
		err := ValidateVectorIndex(c.d, tbl(c.typ), c.idx)
		if c.ok && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrInvalidVectorIndex) {
			t.Errorf("%s: err = %v, want ErrInvalidVectorIndex", c.name, err)
		}
	}
}

func TestVectorIndexOpClass(t *testing.T) {
	cases := []struct {
		typ  string
		m    dialect.Metric
		want string
		ok   bool
	}{
		{"vector(1536)", dialect.MetricCosine, "vector_cosine_ops", true},
		{"halfvec(8)", dialect.MetricInnerProduct, "halfvec_ip_ops", true},
		{"sparsevec(8)", dialect.MetricL2, "sparsevec_l2_ops", true},
		{"bit(8)", dialect.MetricHamming, "bit_hamming_ops", true},
		{"bit(8)", dialect.MetricJaccard, "bit_jaccard_ops", true},
		{"vector(3)", dialect.MetricL1, "vector_l1_ops", true},
		{"bit(8)", dialect.MetricCosine, "", false},
		{"vector(3)", dialect.MetricHamming, "", false},
		{"text", dialect.MetricL2, "", false},
	}
	for _, c := range cases {
		got, err := VectorIndexOpClass(c.typ, c.m)
		if c.ok != (err == nil) || got != c.want {
			t.Errorf("%s/%s: got %q, %v", c.typ, c.m, got, err)
		}
		if err != nil && !errors.Is(err, ErrInvalidVectorIndex) {
			t.Errorf("%s/%s: err %v does not wrap ErrInvalidVectorIndex", c.typ, c.m, err)
		}
	}
}
