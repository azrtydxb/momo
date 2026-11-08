// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo/dialect"
)

// ErrInvalidVectorIndex is wrapped by every ValidateVectorIndex and
// VectorIndexOpClass error.
var ErrInvalidVectorIndex = errors.New("schema: invalid vector index")

var vectorTypeRE = regexp.MustCompile(`^(vector|halfvec|sparsevec|bit)\((\d+)\)$`)

// pgvector limits (0.8): the dimension an index can cover per column type,
// and the ranges of the build parameters.
const (
	maxIndexDimsVector  = 2000
	maxIndexDimsHalfvec = 4000
	maxIndexDimsBit     = 64000
	minHNSWM            = 2
	maxHNSWM            = 100
	defaultHNSWM        = 16
	minHNSWEF           = 4
	defaultHNSWEF       = 64
	maxHNSWEF           = 1000
	maxIVFLists         = 32768
)

// metricSuffix maps a metric to its opclass suffix.
var metricSuffix = map[dialect.Metric]string{
	dialect.MetricL2:           "l2",
	dialect.MetricInnerProduct: "ip",
	dialect.MetricCosine:       "cosine",
	dialect.MetricL1:           "l1",
	dialect.MetricHamming:      "hamming",
	dialect.MetricJaccard:      "jaccard",
}

// opClassSuffixes lists, per access method and column type, the opclass
// suffixes pgvector provides for it.
var opClassSuffixes = map[string]map[string][]string{
	"hnsw": {
		"vector":    {"l2", "ip", "cosine", "l1"},
		"halfvec":   {"l2", "ip", "cosine", "l1"},
		"sparsevec": {"l2", "ip", "cosine", "l1"},
		"bit":       {"hamming", "jaccard"},
	},
	"ivfflat": {
		"vector":  {"l2", "ip", "cosine"},
		"halfvec": {"l2", "ip", "cosine"},
		"bit":     {"hamming"},
	},
}

// ValidateVectorIndex checks an index whose Method is "hnsw" or "ivfflat"
// against the column type, the opclass and the dialect.
// Errors wrap ErrInvalidVectorIndex.
func ValidateVectorIndex(d dialect.Name, t *Table, idx Index) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: index %q: %s", ErrInvalidVectorIndex, idx.Name, fmt.Sprintf(format, args...))
	}
	if d != dialect.Postgres {
		return bad("%s indexes are only supported on %s, not %s", idx.Method, dialect.Postgres, d)
	}
	if idx.Method != "hnsw" && idx.Method != "ivfflat" {
		return bad("method %q is not a vector index method (want hnsw or ivfflat)", idx.Method)
	}
	if len(idx.Parts) != 1 || idx.Parts[0].Column == "" {
		return bad("a vector index takes exactly one column part, expression parts are not allowed")
	}
	part := idx.Parts[0]
	col, ok := findColumn(t, part.Column)
	if !ok {
		return bad("column %q is not in table %q", part.Column, t.Name)
	}
	kind, dims, err := parseVectorType(col.Type)
	if err != nil {
		return bad("column %q has type %q: %v", col.Name, col.Type, err)
	}
	suffixes, ok := opClassSuffixes[idx.Method][kind]
	if !ok {
		return bad("%s is not supported on %s columns", idx.Method, kind)
	}
	if dims == 0 && kind != "bit" {
		return bad("%s column %q has no dimensions, pgvector cannot index it", kind, col.Name)
	}
	if limit := indexDimLimit(kind); limit > 0 && dims > limit {
		return bad("%s(%d) exceeds the %d dimension limit of %s indexes", kind, dims, limit, idx.Method)
	}
	if err := checkOpClass(idx.Method, kind, suffixes, part.OpClass); err != nil {
		return bad("%v", err)
	}
	return checkWith(idx, bad)
}

// MariaDB VECTOR INDEX limits (11.8): M is the number of neighbours per HNSW
// node and DISTANCE the metric the index serves.
const (
	minMariaDBM        = 3
	maxMariaDBM        = 200
	mariaDBIndexMethod = "vector"
)

// mariaDBDistances lists the DISTANCE values of a VECTOR INDEX.
var mariaDBDistances = []string{"euclidean", "cosine"}

// ValidateMariaDBVectorIndex checks an index whose Method is "vector"
// against the column. MariaDB takes exactly one NOT NULL vector(N) column per
// index, M in 3..200 and DISTANCE euclidean or cosine; it has no operator
// classes, no partial or unique vector index, and neither hnsw nor ivfflat.
// Errors wrap ErrInvalidVectorIndex.
func ValidateMariaDBVectorIndex(t *Table, idx Index) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: index %q: %s", ErrInvalidVectorIndex, idx.Name, fmt.Sprintf(format, args...))
	}
	if idx.Method != mariaDBIndexMethod {
		return bad("method %q is not a MariaDB vector index method (want %s; hnsw and ivfflat are pgvector methods)", idx.Method, mariaDBIndexMethod)
	}
	if len(idx.Parts) != 1 || idx.Parts[0].Column == "" {
		return bad("a vector index takes exactly one column part, expression parts are not allowed")
	}
	if idx.Parts[0].OpClass != "" {
		return bad("MariaDB has no operator classes, the distance is the DISTANCE parameter")
	}
	if idx.Unique || idx.Where != "" {
		return bad("a vector index cannot be unique or partial")
	}
	col, ok := findColumn(t, idx.Parts[0].Column)
	if !ok {
		return bad("column %q is not in table %q", idx.Parts[0].Column, t.Name)
	}
	kind, dims, err := parseVectorType(col.Type)
	if err != nil {
		return bad("column %q has type %q: %v", col.Name, col.Type, err)
	}
	if kind != "vector" {
		return bad("MariaDB has only the vector type, not %s", kind)
	}
	if dims == 0 {
		return bad("vector column %q has no dimensions, MariaDB cannot index it", col.Name)
	}
	if col.Nullable {
		return bad("column %q is nullable, MariaDB requires NOT NULL for a vector index", col.Name)
	}
	for _, kv := range idx.With {
		switch kv.Key {
		case "m":
			m, err := strconv.Atoi(kv.Value)
			if err != nil {
				return bad("parameter m=%q is not an integer", kv.Value)
			}
			if m < minMariaDBM || m > maxMariaDBM {
				return bad("m=%d is outside %d..%d", m, minMariaDBM, maxMariaDBM)
			}
		case "distance":
			if !slices.Contains(mariaDBDistances, kv.Value) {
				return bad("distance=%q is not one of %s", kv.Value, joinSuffixes(mariaDBDistances))
			}
		default:
			return bad("parameter %q is not valid for a MariaDB vector index (allowed: m|distance)", kv.Key)
		}
	}
	return nil
}

// MariaDBVectorIndexClause renders a validated index as the clause MariaDB
// accepts inside CREATE TABLE and ALTER TABLE ... ADD, for example
// "VECTOR INDEX `docs_emb` (`emb`) M=8 DISTANCE=cosine". M and DISTANCE are
// written only when the index sets them. The migration engine for MySQL calls
// this to render a vector index.
func MariaDBVectorIndexClause(t *Table, idx Index) (string, error) {
	if err := ValidateMariaDBVectorIndex(t, idx); err != nil {
		return "", err
	}
	q := func(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }
	var sb strings.Builder
	sb.WriteString("VECTOR INDEX " + q(idx.Name) + " (" + q(idx.Parts[0].Column) + ")")
	var m, dist string
	for _, kv := range idx.With {
		if kv.Key == "m" {
			m = kv.Value
		} else {
			dist = kv.Value
		}
	}
	if m != "" {
		sb.WriteString(" M=" + m)
	}
	if dist != "" {
		sb.WriteString(" DISTANCE=" + dist)
	}
	return sb.String(), nil
}

func findColumn(t *Table, name string) (Column, bool) {
	for _, c := range t.Columns {
		if c.Name == name {
			return c, true
		}
	}
	return Column{}, false
}

// parseVectorType splits "vector(3)" into its kind and dimension. A type
// without a dimension ("vector") has dimension 0, which no limit rejects.
func parseVectorType(typ string) (string, int, error) {
	switch typ {
	case "vector", "halfvec", "sparsevec", "bit":
		return typ, 0, nil
	}
	m := vectorTypeRE.FindStringSubmatch(typ)
	if m == nil {
		return "", 0, errors.New("not a vector, halfvec, sparsevec or bit type")
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return "", 0, err
	}
	return m[1], n, nil
}

func indexDimLimit(kind string) int {
	switch kind {
	case "vector":
		return maxIndexDimsVector
	case "halfvec":
		return maxIndexDimsHalfvec
	case "bit":
		return maxIndexDimsBit
	}
	return 0
}

func checkOpClass(method, kind string, suffixes []string, op string) error {
	if op == "" {
		// Only ivfflat on vector has a default class (vector_l2_ops), and
		// inspection reports a default class as empty.
		if method == "ivfflat" && kind == "vector" {
			return nil
		}
		return fmt.Errorf("%s on %s has no default operator class, declare one such as %s_%s_ops", method, kind, kind, suffixes[0])
	}
	for _, s := range suffixes {
		if op == kind+"_"+s+"_ops" {
			return nil
		}
	}
	return fmt.Errorf("operator class %q does not fit %s on %s (want %s_{%s}_ops)", op, method, kind, kind, joinSuffixes(suffixes))
}

func joinSuffixes(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += "|"
		}
		out += v
	}
	return out
}

func checkWith(idx Index, bad func(string, ...any) error) error {
	allowed := map[string][]string{
		"hnsw":    {"m", "ef_construction"},
		"ivfflat": {"lists"},
	}[idx.Method]
	vals := map[string]int{}
	for _, kv := range idx.With {
		if !slices.Contains(allowed, kv.Key) {
			return bad("storage parameter %q is not valid for %s (allowed: %s)", kv.Key, idx.Method, joinSuffixes(allowed))
		}
		n, err := strconv.Atoi(kv.Value)
		if err != nil {
			return bad("storage parameter %s=%q is not an integer", kv.Key, kv.Value)
		}
		vals[kv.Key] = n
	}
	if idx.Method == "ivfflat" {
		// The server defaults lists to 100; momo requires it to be explicit.
		lists, ok := vals["lists"]
		if !ok {
			return bad("ivfflat requires the lists storage parameter")
		}
		if lists < 1 || lists > maxIVFLists {
			return bad("lists=%d is outside 1..%d", lists, maxIVFLists)
		}
		return nil
	}
	m, hasM := vals["m"]
	if hasM && (m < minHNSWM || m > maxHNSWM) {
		return bad("m=%d is outside %d..%d", m, minHNSWM, maxHNSWM)
	}
	if !hasM {
		m = defaultHNSWM
	}
	ef, hasEF := vals["ef_construction"]
	if !hasEF {
		ef = defaultHNSWEF
	}
	if hasEF {
		if ef < minHNSWEF || ef > maxHNSWEF {
			return bad("ef_construction=%d is outside %d..%d", ef, minHNSWEF, maxHNSWEF)
		}
	}
	if ef < 2*m {
		return bad("ef_construction=%d must be at least 2*m (%d)", ef, 2*m)
	}
	return nil
}

// VectorIndexOpClass returns the opclass for a column type and metric,
// e.g. ("vector(1536)", dialect.MetricCosine) -> "vector_cosine_ops".
// The metric must be one pgvector provides for the type.
func VectorIndexOpClass(columnType string, m dialect.Metric) (string, error) {
	kind, _, err := parseVectorType(columnType)
	if err != nil {
		return "", fmt.Errorf("%w: type %q: %v", ErrInvalidVectorIndex, columnType, err)
	}
	suffix, ok := metricSuffix[m]
	if !ok {
		return "", fmt.Errorf("%w: unknown metric %q", ErrInvalidVectorIndex, m)
	}
	if !slices.Contains(opClassSuffixes["hnsw"][kind], suffix) {
		return "", fmt.Errorf("%w: metric %q has no operator class on %s", ErrInvalidVectorIndex, m, kind)
	}
	return kind + "_" + suffix + "_ops", nil
}
