// SPDX-License-Identifier: Apache-2.0

package dialect

// Metric names a vector distance metric.
type Metric string

const (
	MetricL2           Metric = "l2"      // <-> on PostgreSQL; vec_distance_l2 on SQLite
	MetricInnerProduct Metric = "ip"      // <#> (negative inner product) on PostgreSQL only
	MetricCosine       Metric = "cosine"  // <=> on PostgreSQL; vec_distance_cosine on SQLite
	MetricL1           Metric = "l1"      // <+> on PostgreSQL; vec_distance_l1 on SQLite
	MetricHamming      Metric = "hamming" // <~> on PostgreSQL; vec_distance_hamming on SQLite (bit vectors)
	MetricJaccard      Metric = "jaccard" // <%> on PostgreSQL only
)

// VectorCaps describes a dialect's vector support.
type VectorCaps struct {
	Storage bool     // vector column types can be declared
	Metrics []Metric // distance metrics this dialect renders
	Indexes []string // ANN index methods available: "hnsw", "ivfflat", "vector"
}
