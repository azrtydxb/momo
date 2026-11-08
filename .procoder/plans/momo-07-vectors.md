# momo-07-vectors — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

Milestone: M1 (PostgreSQL + pgvector) for Tasks 1 to 10 and the PostgreSQL sections of Task 12; Task 11 (sqlite-vec) lands in M2, and the SQLite sections of Task 12 complete with it.

## Task 1: Vector value types and validation

Covers: S-15 (types and validation), S-15 acceptance `TestVectorCodec`, `FuzzVectorDecode`.

Files:

- Create `vector/vector.go`: `Vector`, `HalfVector`, `SparseVector`, `Bits`, `ErrInvalidVector`, `MaxDim`, `Validate`, `Value`, `Scan`.
- Create `vector/codec_test.go`: `TestVectorCodec`.
- Create `vector/fuzz_test.go`: `FuzzVectorDecode`.

Interfaces (exact):

```go
package vector

var ErrInvalidVector = errors.New("momo/vector: invalid vector")
var ErrDialect = errors.New("momo/vector: type not supported by dialect")

const MaxDim = 16000 // storage limit for vector, halfvec, sparsevec (pgvector README)

type Vector []float32
type HalfVector []float32
type SparseVector struct {
    Dim     int
    Indices []int32   // 0-based, strictly increasing, each in [0, Dim)
    Values  []float32
}
type Bits struct {
    Len  int
    Data []byte      // ceil(Len/8) bytes; unused trailing bits must be zero
}

func (v Vector) Validate(dim int) error
func (h HalfVector) Validate(dim int) error     // additionally |x| <= 65504
func (s SparseVector) Validate(dim int) error
func (b Bits) Validate(dim int) error
func (v Vector) Value() (driver.Value, error)   // "[1,2.5,-3]"
func (v *Vector) Scan(src any) error            // string: pgvector text; []byte: sqlite-vec little-endian float32 blob
// HalfVector, SparseVector, Bits: Value/Scan with pgvector text forms "[1,2]", "{1:1.5,3:2}/5", "0101".
```

- [ ] Step 1. Write the failing test `vector/codec_test.go`:

```go
package vector

import (
	"errors"
	"math"
	"testing"
)

func TestVectorCodec(t *testing.T) {
	v := Vector{1, 2.5, -3}
	val, err := v.Value()
	if err != nil {
		t.Fatal(err)
	}
	if val != "[1,2.5,-3]" {
		t.Fatalf("text encoding = %v, want [1,2.5,-3]", val)
	}
	var got Vector
	if err := got.Scan("[1,2.5,-3]"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 2.5 || got[2] != -3 {
		t.Fatalf("round trip = %v", got)
	}
	if err := v.Validate(2); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("dimension 3 against 2: err = %v, want ErrInvalidVector", err)
	}
	if err := (Vector{float32(math.NaN())}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("NaN: err = %v, want ErrInvalidVector", err)
	}
	if err := (Vector{float32(math.Inf(1))}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("Inf: err = %v, want ErrInvalidVector", err)
	}
	if err := (HalfVector{70000}).Validate(1); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("halfvec out of range: err = %v, want ErrInvalidVector", err)
	}
	s := SparseVector{Dim: 5, Indices: []int32{1, 3}, Values: []float32{1, 2}}
	sv, err := s.Value()
	if err != nil {
		t.Fatal(err)
	}
	if sv != "{2:1,4:2}/5" {
		t.Fatalf("sparse text = %v, want {2:1,4:2}/5 (pgvector literals are 1-based)", sv)
	}
	if err := (SparseVector{Dim: 5, Indices: []int32{3, 1}, Values: []float32{1, 2}}).Validate(5); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("unsorted sparse indices: err = %v, want ErrInvalidVector", err)
	}
	b := Bits{Len: 4, Data: []byte{0xA0}}
	bv, err := b.Value()
	if err != nil {
		t.Fatal(err)
	}
	if bv != "1010" {
		t.Fatalf("bits text = %v, want 1010", bv)
	}
	if err := (Bits{Len: 4, Data: []byte{0xA1}}).Validate(4); !errors.Is(err, ErrInvalidVector) {
		t.Fatalf("nonzero trailing bits: err = %v, want ErrInvalidVector", err)
	}
}
```

- [ ] Step 2. Run `go test ./vector -run TestVectorCodec -count=1`; expect FAIL with `undefined: Vector`.
- [ ] Step 3. Implement `vector/vector.go`. Key code:

```go
func (v Vector) Validate(dim int) error {
	if len(v) != dim {
		return fmt.Errorf("%w: dimension %d, want %d", ErrInvalidVector, len(v), dim)
	}
	if dim < 1 || dim > MaxDim {
		return fmt.Errorf("%w: dimension %d outside 1..%d", ErrInvalidVector, dim, MaxDim)
	}
	for i, x := range v {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return fmt.Errorf("%w: element %d is not finite", ErrInvalidVector, i)
		}
	}
	return nil
}

func (v Vector) Value() (driver.Value, error) { return formatDense(v), nil }

func (v *Vector) Scan(src any) error {
	switch s := src.(type) {
	case string:
		out, err := parseDense(s)
		if err != nil {
			return err
		}
		*v = out
		return nil
	case []byte:
		out, err := decodeFloat32LE(s) // sqlite-vec blob: len%4 == 0
		if err != nil {
			return err
		}
		*v = out
		return nil
	case nil:
		*v = nil
		return nil
	}
	return fmt.Errorf("%w: cannot scan %T", ErrInvalidVector, src)
}
```

`formatDense` writes `[` + comma-separated `strconv.FormatFloat(float64(x), 'g', -1, 32)` + `]`. `parseDense` trims brackets, splits on commas, and rejects NaN/Inf and empty input. `decodeFloat32LE` checks `len(b)%4 == 0` and reads `binary.LittleEndian.Uint32` per element. Scan rule: a `string` is always pgvector text (PostgreSQL); a `[]byte` is always a sqlite-vec blob (SQLite). The driver never returns text as `[]byte` for PostgreSQL in this plan; TestVectorCodec and the integration tests in Task 11 catch a violation. For Bits, a `[]byte` on SQLite is the packed bit blob: Len becomes 8 times its length, and the column's declared dimension is authoritative.
HalfVector, SparseVector and Bits implement the same method set; SparseVector text is `{i+1:v,...}/Dim`; Bits text is a string of `0` and `1`.

- [ ] Step 4. Run `go test ./vector -run TestVectorCodec -count=1`; expect `ok  	github.com/azrtydxb/momo/vector`.
- [ ] Step 5. Write `vector/fuzz_test.go`:

```go
package vector

import "testing"

func FuzzVectorDecode(f *testing.F) {
	f.Add("[1,2,3]")
	f.Add("[]")
	f.Add("[NaN]")
	f.Add("{1:1,3:2}/5")
	f.Fuzz(func(t *testing.T, in string) {
		var v Vector
		if err := v.Scan(in); err != nil {
			return
		}
		enc, err := v.Value()
		if err != nil {
			t.Fatalf("Value after successful Scan: %v", err)
		}
		var again Vector
		if err := again.Scan(enc); err != nil {
			t.Fatalf("re-scan of %q failed: %v", enc, err)
		}
		if len(again) != len(v) {
			t.Fatalf("round trip changed length: %d -> %d", len(v), len(again))
		}
	})
}
```

Run `go test ./vector -run=^$ -fuzz=FuzzVectorDecode -fuzztime=20s`; expect `PASS`. Also `go test ./vector -run FuzzVectorDecode` (seed corpus) expects `ok`.

- [ ] Step 6. Run `go test ./vector -count=1 -race`; expect `ok`.
- [ ] Step 7. Commit: `feat(vector): add vector value types with validation`.

## Task 2: Dialect vector capabilities and dialect-specific argument encoding

Covers: S-15 (distance support per dialect), S-16 (sqlite-vec encodings and the metric list, including bit vectors with Hamming distance). Acceptance: `TestVectorCodec` (dimension rejected before any SQL runs, exercised through the builder); `TestSQLiteVecLoaded` and `TestSQLiteVecMetrics` (Task 11) confirm the SQLite list against the pinned library.

Files:

- Create `dialect/vector.go`: `Metric`, `VectorCaps`.
- Create `dialect/vector_test.go`: `TestSQLiteVectorCaps`.
- Modify `dialect/dialect.go`: add field `Vector VectorCaps`; set it in the `postgres` and `sqlite` literals.
- Create `vector/dialect.go`: `DialectValue` methods.
- Create `vector/dialect_test.go`: `TestDialectValues`.
- Create `vector_builder_test.go` (package `momo`): `TestVectorArgBeforeSQL`.
- Modify `builder.go` (root): map `vector.ErrDialect` to `ErrUnsupported` inside `Arg`.

Interfaces (exact):

```go
// dialect/vector.go
package dialect

type Metric string

const (
	MetricL2           Metric = "l2"       // <-> on PostgreSQL; vec_distance_l2 on SQLite
	MetricInnerProduct Metric = "ip"       // <#> on PostgreSQL only
	MetricCosine       Metric = "cosine"   // <=> on PostgreSQL; vec_distance_cosine on SQLite
	MetricL1           Metric = "l1"       // <+> on PostgreSQL; vec_distance_l1 on SQLite
	MetricHamming      Metric = "hamming"  // <~> on PostgreSQL; vec_distance_hamming on SQLite (bit vectors)
	MetricJaccard      Metric = "jaccard"  // <%> on PostgreSQL only
)

type VectorCaps struct {
	Storage bool      // vector column types can be declared
	Metrics []Metric // distance metrics this dialect renders
	Indexes []string // ANN index methods available: "hnsw", "ivfflat", "vector"
}

// dialect.go literals:
//   postgres: Vector: VectorCaps{Storage: true, Metrics: []Metric{MetricL2, MetricInnerProduct, MetricCosine, MetricL1, MetricHamming, MetricJaccard}, Indexes: []string{"hnsw", "ivfflat"}}
//   sqlite:   Vector: VectorCaps{Storage: true, Metrics: []Metric{MetricL2, MetricL1, MetricCosine, MetricHamming}, Indexes: nil}
// The SQLite list is the set of scalar distance functions registered by sqlite-vec v0.1.6.
// Source: the aFunc[] table in cgo/sqlite-vec.c and SQLITE_VEC_VERSION in cgo/sqlite-vec.h of
// github.com/asg017/sqlite-vec-go-bindings/cgo v0.1.6, read on 2026-11-04. The table has no
// inner-product or Jaccard function.

// vector/dialect.go
func (v Vector) DialectValue(d dialect.Name) (any, error)     // postgres: v (the driver encodes text or binary); sqlite: little-endian float32 blob; others: ErrDialect
func (h HalfVector) DialectValue(d dialect.Name) (any, error) // postgres: h; everything else: ErrDialect
func (s SparseVector) DialectValue(d dialect.Name) (any, error) // postgres: s; everything else: ErrDialect
func (b Bits) DialectValue(d dialect.Name) (any, error)       // postgres: b; sqlite: b.Data packed bytes (SQL wraps them in vec_bit); others: ErrDialect
```

- [ ] Step 1. Write `dialect/vector_test.go`:

```go
package dialect

import (
	"slices"
	"testing"
)

func TestSQLiteVectorCaps(t *testing.T) {
	caps := For(SQLite).Vector
	want := []Metric{MetricL2, MetricL1, MetricCosine, MetricHamming}
	if len(caps.Metrics) != len(want) {
		t.Fatalf("sqlite metrics = %v, want %v", caps.Metrics, want)
	}
	for _, m := range want {
		if !slices.Contains(caps.Metrics, m) {
			t.Errorf("sqlite metric %s missing", m)
		}
	}
	if slices.Contains(caps.Metrics, MetricInnerProduct) || slices.Contains(caps.Metrics, MetricJaccard) {
		t.Errorf("sqlite must not advertise ip or jaccard: %v", caps.Metrics)
	}
	if len(caps.Indexes) != 0 {
		t.Errorf("sqlite ANN indexes = %v, want none", caps.Indexes)
	}
}
```

Expect FAIL with `For(SQLite).Vector undefined` (or `Metric` undefined).

- [ ] Step 2. Implement `dialect/vector.go` and the `Vector` field in `dialect/dialect.go`. Run `go test ./dialect -run TestSQLiteVectorCaps -count=1`; expect `ok`.
- [ ] Step 3. Write `vector/dialect_test.go`:

```go
package vector

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestDialectValues(t *testing.T) {
	v := Vector{1, 2}
	got, err := v.DialectValue(dialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 8)
	binary.LittleEndian.PutUint32(want[0:], 0x3f800000) // float32(1)
	binary.LittleEndian.PutUint32(want[4:], 0x40000000) // float32(2)
	if b, ok := got.([]byte); !ok || !bytes.Equal(b, want) {
		t.Fatalf("sqlite encoding = %x, want %x", got, want)
	}
	pg, err := v.DialectValue(dialect.Postgres)
	if err != nil {
		t.Fatal(err)
	}
	if pg == nil {
		t.Fatal("postgres value must be passed through to the driver")
	}
	bits, err := (Bits{Len: 8, Data: []byte{0x80}}).DialectValue(dialect.SQLite)
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := bits.([]byte); !ok || !bytes.Equal(b, []byte{0x80}) {
		t.Fatalf("sqlite bits = %v, want packed bytes 80", bits)
	}
	if _, err := (HalfVector{1}).DialectValue(dialect.SQLite); !errors.Is(err, ErrDialect) {
		t.Fatalf("halfvec on sqlite: err = %v, want ErrDialect", err)
	}
	if _, err := (SparseVector{Dim: 2, Indices: []int32{0}, Values: []float32{1}}).DialectValue(dialect.SQLite); !errors.Is(err, ErrDialect) {
		t.Fatalf("sparsevec on sqlite: err = %v, want ErrDialect", err)
	}
}
```

Expect FAIL with `v.DialectValue undefined`.

- [ ] Step 4. Implement `vector/dialect.go`. Run `go test ./vector -run TestDialectValues -count=1`; expect `ok`.
- [ ] Step 5. Write `vector_builder_test.go`:

```go
package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorArgBeforeSQL(t *testing.T) {
	_, err := compile(dialect.For(dialect.SQLite), func(b *Builder) { b.Arg(vector.HalfVector{1}) })
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("halfvec on sqlite: err = %v, want ErrUnsupported", err)
	}
	c, err := compile(dialect.For(dialect.SQLite), func(b *Builder) { b.Arg(vector.Bits{Len: 8, Data: []byte{0x80}}) })
	if err != nil {
		t.Fatalf("bits on sqlite must compile: %v", err)
	}
	if len(c.Args) != 1 {
		t.Fatalf("args = %v, want one bound bit blob", c.Args)
	}
}
```

Expect FAIL on the first assertion: the spike maps DialectValue errors to a plain error, so `errors.Is(err, ErrUnsupported)` is false until Step 6.

- [ ] Step 6. Modify `builder.go` `Arg`: when `DialectValue` returns an error wrapping `vector.ErrDialect`, call `b.SetErr(&Error{Kind: ErrUnsupported, Err: err})`. Run `go test . -run TestVectorArgBeforeSQL -count=1`; expect `ok`.
- [ ] Step 7. Run `go test ./dialect ./vector . -count=1`; expect `ok` for all three packages.
- [ ] Step 8. Commit: `feat(vector): add dialect vector capabilities and argument encoding`.

## Task 3: VectorColumn and typed distance expressions

Covers: S-15 (distance expressions per pgvector operators; dimension validation), S-16 (sqlite-vec mapping of L2, L1, cosine and Hamming). Acceptance: `TestVectorDistances`.

Files:

- Create `vector_column.go`: `VectorColumn[V]`, `NewVectorColumn`.
- Create `vector_expr.go`: distance constructors, `distExpr`, operator and function maps.
- Create `vector_expr_test.go`: `TestVectorDistanceSQL`, `TestVectorDistanceDimensionMismatch`, `TestSQLiteVectorDistanceSQL`.
- Create `vector_integration_test.go` (`//go:build integration`): `TestVectorDistances` against PostgreSQL.
- Create `testdata/compilefail/vector_metric_mismatch/main.go`.

Interfaces (exact):

```go
// vector_column.go (package momo)
type VectorValue interface {
	vector.Vector | vector.HalfVector | vector.SparseVector | vector.Bits
}
type VectorColumn[V VectorValue] struct {
	table string
	name  string
	dim   int
	flags uint16
}
func NewVectorColumn[V VectorValue](table, name string, dim int, flags uint16) VectorColumn[V] // used by generated code
func (c VectorColumn[V]) As(alias string) VectorColumn[V]
func (c VectorColumn[V]) AppendSQL(b *Builder)
func (c VectorColumn[V]) ColumnName() string
func (c VectorColumn[V]) Dim() int
func (c VectorColumn[V]) IsNull() Predicate
func (c VectorColumn[V]) Set(v V) Assignment

// vector_expr.go (package momo)
type denseVector interface{ vector.Vector | vector.HalfVector | vector.SparseVector }
type bitVector interface{ vector.Bits }

func L2Distance[V denseVector](c VectorColumn[V], v V) Expression[float64]
func InnerProductDistance[V denseVector](c VectorColumn[V], v V) Expression[float64]
func CosineDistance[V denseVector](c VectorColumn[V], v V) Expression[float64]
func L1Distance[V denseVector](c VectorColumn[V], v V) Expression[float64]
func HammingDistance[V bitVector](c VectorColumn[V], v V) Expression[float64]
func JaccardDistance[V bitVector](c VectorColumn[V], v V) Expression[float64]

// Operator and function maps (vector_expr.go)
var pgOperators = map[dialect.Metric]string{
	dialect.MetricL2: "<->", dialect.MetricInnerProduct: "<#>", dialect.MetricCosine: "<=>",
	dialect.MetricL1: "<+>", dialect.MetricHamming: "<~>", dialect.MetricJaccard: "<%>",
}
var sqliteFunctions = map[dialect.Metric]string{
	dialect.MetricL2:      "vec_distance_l2",
	dialect.MetricL1:      "vec_distance_l1",
	dialect.MetricCosine:  "vec_distance_cosine",
	dialect.MetricHamming: "vec_distance_hamming",
}

// Test helpers (vector_expr_test.go, package momo). They fail the test on build errors.
func renderForTest(t *testing.T, d *dialect.Dialect, e Expression[float64]) string
func renderArgs(t *testing.T, d *dialect.Dialect, e Expression[float64]) []any
func renderErr(d *dialect.Dialect, e Expression[float64]) (string, error) // for negative tests
```

`Expression[float64]` values implement `Orderable`, so `.Asc()` and `.Desc()` work in `OrderBy`. SQLite Hamming wraps both operands in `vec_bit()`: sqlite-vec checks that both operands have the same element type through the bit subtype that `vec_bit()` sets. Passing a `vector.Bits` to `CosineDistance` is a compile error because the constraint excludes it.

- [ ] Step 1. Write `vector_expr_test.go`:

```go
package momo

import (
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorDistanceSQL(t *testing.T) {
	pg := dialect.For(dialect.Postgres)
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	q := vector.Vector{4, 5, 6}
	if got := renderForTest(t, pg, L2Distance(col, q)); got != `("docs"."emb" <-> $1)` {
		t.Fatalf("L2 SQL = %s", got)
	}
	if args := renderArgs(t, pg, L2Distance(col, q)); args[0].(vector.Vector)[0] != 4 {
		t.Fatalf("bound arg = %v", args[0])
	}
	if got := renderForTest(t, pg, InnerProductDistance(col, q)); got != `("docs"."emb" <#> $1)` {
		t.Fatalf("IP SQL = %s", got)
	}
	if got := renderForTest(t, pg, CosineDistance(col, q)); got != `("docs"."emb" <=> $1)` {
		t.Fatalf("cosine SQL = %s", got)
	}
	if got := renderForTest(t, pg, L1Distance(col, q)); got != `("docs"."emb" <+> $1)` {
		t.Fatalf("L1 SQL = %s", got)
	}
	sq := dialect.For(dialect.SQLite)
	if got := renderForTest(t, sq, CosineDistance(col, q)); got != `vec_distance_cosine("docs"."emb", ?)` {
		t.Fatalf("sqlite cosine SQL = %s", got)
	}
	if got := renderForTest(t, sq, L1Distance(col, q)); got != `vec_distance_l1("docs"."emb", ?)` {
		t.Fatalf("sqlite L1 SQL = %s", got)
	}
	// Reference values computed in Go without SQL.
	a := []float64{1, 2, 3}
	b := []float64{4, 5, 6}
	var dot, na, nb, l1, sumSq float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
		d := a[i] - b[i]
		l1 += math.Abs(d)
		sumSq += d * d
	}
	refL2, refCos, refIP := math.Sqrt(sumSq), 1-dot/(math.Sqrt(na)*math.Sqrt(nb)), -dot
	if math.Abs(refL2-math.Sqrt(27)) > 1e-5 || math.Abs(l1-9) > 1e-5 || math.Abs(refIP+32) > 1e-5 || math.Abs(refCos-0.0253682) > 1e-5 {
		t.Fatalf("reference values: l2=%v l1=%v ip=%v cos=%v", refL2, l1, refIP, refCos)
	}
}

func TestVectorDistanceDimensionMismatch(t *testing.T) {
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	_, err := renderErr(dialect.For(dialect.Postgres), L2Distance(col, vector.Vector{1, 2}))
	if !errors.Is(err, vector.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector", err)
	}
}

func TestSQLiteVectorDistanceSQL(t *testing.T) {
	sq := dialect.For(dialect.SQLite)
	bits := NewVectorColumn[vector.Bits]("docs", "bits", 8, 0)
	q := vector.Bits{Len: 8, Data: []byte{0xB0}}
	if got := renderForTest(t, sq, HammingDistance(bits, q)); got != `vec_distance_hamming(vec_bit("docs"."bits"), vec_bit(?))` {
		t.Fatalf("sqlite hamming SQL = %s", got)
	}
	if args := renderArgs(t, sq, HammingDistance(bits, q)); len(args) != 1 {
		t.Fatalf("sqlite hamming args = %v, want one bit blob", args)
	}
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	if _, err := renderErr(sq, InnerProductDistance(col, vector.Vector{1, 2, 3})); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("sqlite ip: err = %v, want ErrUnsupported", err)
	}
	if _, err := renderErr(sq, JaccardDistance(bits, q)); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("sqlite jaccard: err = %v, want ErrUnsupported", err)
	}
}
```

Expect FAIL with `undefined: NewVectorColumn`.

- [ ] Step 2. Run `go test . -run 'TestVectorDistanceSQL|TestVectorDistanceDimensionMismatch|TestSQLiteVectorDistanceSQL' -count=1`; expect FAIL with `undefined: NewVectorColumn`.
- [ ] Step 3. Implement `vector_expr.go`. Key code:

```go
func (e distExpr[V]) AppendSQL(b *Builder) {
	if !slices.Contains(b.d.Vector.Metrics, e.metric) {
		b.SetErr(&Error{Kind: ErrUnsupported, Table: e.col.table, Column: e.col.name,
			Err: fmt.Errorf("%s distance not available on %s", e.metric, b.d.Name)})
		return
	}
	if err := validateValue(e.arg, e.col.dim); err != nil {
		b.SetErr(err)
		return
	}
	if b.d.Name == dialect.Postgres {
		b.WriteString("(")
		e.col.AppendSQL(b)
		b.WriteString(" " + pgOperators[e.metric] + " ")
		b.Arg(e.arg)
		b.WriteString(")")
		return
	}
	bitOperands := e.metric == dialect.MetricHamming
	b.WriteString(sqliteFunctions[e.metric] + "(")
	if bitOperands {
		b.WriteString("vec_bit(")
	}
	e.col.AppendSQL(b)
	if bitOperands {
		b.WriteString(")")
	}
	b.WriteString(", ")
	if bitOperands {
		b.WriteString("vec_bit(")
	}
	b.Arg(e.arg)
	if bitOperands {
		b.WriteString(")")
	}
	b.WriteString(")")
}
```

`validateValue` type-asserts `any(v)` to an interface with `Validate(int) error` and calls it. `distExpr` also implements the `Expression[float64]` method set of expr.go (`AppendSQL(*Builder)` and `resultType() float64`). Run Step 2's command; expect `ok`.

- [ ] Step 4. Write `vector_integration_test.go` (`//go:build integration`, package `momo_test` because it opens a `*momo.DB` through `testopen`). `TestVectorDistances` builds the query with the typed API, runs the compiled SQL on real PostgreSQL, and checks the row order of the L2 query and then, for each dense distance function (`L2Distance`, `InnerProductDistance`, `CosineDistance`, `L1Distance`), every row's database-computed value against a value computed independently in Go, to 1e-5 (S-15):

```go
//go:build integration

package momo_test

import (
	"math"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

// vecDoc is hand-written: SQL() needs only the table name and the primary key.
type vecDoc struct {
	ID  int64
	Emb vector.Vector
}

func (d *vecDoc) MomoMeta() *momo.Meta[vecDoc] {
	return &momo.Meta[vecDoc]{Table: "docs", PK: []int{0}, Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}}}
}

func TestVectorDistances(t *testing.T) {
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(3))")
	rows := []vector.Vector{{1, 2, 3}, {4, 5, 6}, {10, 10, 10}}
	for i, v := range rows {
		mustExecPG(t, db, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i+1), v)
	}
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	c, err := momo.Select[vecDoc, *vecDoc](db).OrderBy(momo.L2Distance(col, vector.Vector{4, 5, 6}).Asc()).Limit(3).SQL()
	if err != nil {
		t.Fatal(err)
	}
	// Reference distances to {4,5,6}: id 2 is 0, id 1 is sqrt(27), id 3 is sqrt(75).
	if got, want := queryIDs(t, pgPool(t), c), []int64{2, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	// Reference math in plain Go. Inner product is pgvector's `<#>`, which
	// returns the NEGATIVE dot product so that ascending order means "closest".
	q := vector.Vector{4, 5, 6}
	l2 := func(a, b vector.Vector) float64 {
		var sum float64
		for i := range a {
			d := float64(a[i]) - float64(b[i])
			sum += d * d
		}
		return math.Sqrt(sum)
	}
	l1 := func(a, b vector.Vector) float64 {
		var sum float64
		for i := range a {
			sum += math.Abs(float64(a[i]) - float64(b[i]))
		}
		return sum
	}
	negDot := func(a, b vector.Vector) float64 {
		var dot float64
		for i := range a {
			dot += float64(a[i]) * float64(b[i])
		}
		return -dot
	}
	cosine := func(a, b vector.Vector) float64 {
		var dot, na, nb float64
		for i := range a {
			dot += float64(a[i]) * float64(b[i])
			na += float64(a[i]) * float64(a[i])
			nb += float64(b[i]) * float64(b[i])
		}
		return 1 - dot/(math.Sqrt(na)*math.Sqrt(nb))
	}
	idCol := momo.ColumnOf[int64]("docs", "id")
	cases := []struct {
		name string
		expr momo.Expression[float64]
		ref  func(a, b vector.Vector) float64
	}{
		{"L2Distance", momo.L2Distance(col, q), l2},
		{"InnerProductDistance", momo.InnerProductDistance(col, q), negDot},
		{"CosineDistance", momo.CosineDistance(col, q), cosine},
		{"L1Distance", momo.L1Distance(col, q), l1},
	}
	for _, tc := range cases {
		// The select list must match the two mapped fields of vecDoc, so it is id plus the distance.
		c, err := momo.Select[vecDoc, *vecDoc](db).Columns(idCol, tc.expr).SQL()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := queryIDFloat(t, pgPool(t), c)
		if len(got) != len(rows) {
			t.Fatalf("%s: got %d rows, want %d", tc.name, len(got), len(rows))
		}
		for i, v := range rows {
			id, want := int64(i+1), tc.ref(v, q)
			if g, ok := got[id]; !ok || math.Abs(g-want) > 1e-5 {
				t.Errorf("%s id %d = %v, want %v (1e-5)", tc.name, id, g, want)
			}
		}
	}
}
```

`helpers_integration_test.go` (package `momo_test`, build tag `integration`) defines the four helpers used by this and the later integration steps: `mustExecPG(t, q momo.Querier, sql string, args ...any)` fails the test on any error from `momo.Exec`; `pgPool(t) *pgxpool.Pool` opens a pool on `testdb.PostgresURL(t)` and closes it with `t.Cleanup`; `queryIDs(t, pool, c momo.Compiled) []int64` runs `c.SQL` with `c.Args` and returns the first column; `queryIDFloat(t, pool *pgxpool.Pool, c momo.Compiled) map[int64]float64` runs `c.SQL` with `c.Args`, scans two columns per row (an `int64` id, then a `float64` value) and returns them as a map keyed by id, failing the test on any query or scan error. Expect FAIL with `undefined: NewVectorColumn` before Task 3 Step 3. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestVectorDistances -count=1`; expect `ok`.

- [ ] Step 5. Write `testdata/compilefail/vector_metric_mismatch/main.go` with `CosineDistance(col, vector.Bits{Len: 1, Data: []byte{0}})` where `col` is `VectorColumn[vector.Bits]`. Run `go build ./testdata/compilefail/vector_metric_mismatch`; expect a non-zero exit with `does not satisfy`.
- [ ] Step 6. Commit: `feat(momo): add VectorColumn and typed distance expressions for pgvector and sqlite-vec`.

## Task 4: Vector index declarations and validation

Covers: S-15 (HNSW and IVFFlat index config with matching opclasses), S-12 (index method and opclass in schema). Acceptance: `TestVectorIndexUsed` depends on this task's index creation (Task 7 runs it); this task adds `TestVectorIndexRules`.

Files:

- Create `schema/vector.go`: `ValidateVectorIndex`, `VectorIndexOpClass`.
- Create `schema/vector_test.go`: `TestVectorIndexRules`.
- Modify nothing else. The generator companion syntax (`c.Index(...)`) is owned by momo-02; this plan only supplies `schema.Index` values and the validator it calls.

Interfaces (exact):

```go
// schema/vector.go
package schema

import "github.com/azrtydxb/momo/dialect"

// ValidateVectorIndex checks an index whose Method is "hnsw" or "ivfflat"
// against the column type, the opclass and the dialect.
// Errors wrap ErrInvalidVectorIndex.
func ValidateVectorIndex(d dialect.Name, t *Table, idx Index) error

var ErrInvalidVectorIndex = errors.New("schema: invalid vector index")

// VectorIndexOpClass returns the opclass for a column type and metric,
// e.g. ("vector(1536)", dialect.MetricCosine) -> "vector_cosine_ops".
func VectorIndexOpClass(columnType string, m dialect.Metric) (string, error)
```

Rules (enforced by ValidateVectorIndex; each is a row in TestVectorIndexRules):

- `hnsw` on `vector(N)`: opclass in {vector_l2_ops, vector_ip_ops, vector_cosine_ops, vector_l1_ops}; N <= 2000.
- `hnsw` on `halfvec(N)`: opclass prefix `halfvec_`; N <= 4000.
- `hnsw` on `sparsevec(N)`: opclass prefix `sparsevec_`; nnz limit (1000) is enforced by the server and not by this validator.
- `hnsw` on `bit(N)`: opclass in {bit_hamming_ops, bit_jaccard_ops}; N <= 64000.
- `ivfflat` on `vector(N)`: N <= 2000; opclass prefix `vector_`; `lists` in With required and > 0.
- `ivfflat` on `halfvec(N)`: N <= 4000.
- `ivfflat` on `sparsevec(N)`: rejected (pgvector: IVFFlat not supported for sparsevec).
- `ivfflat` on `bit(N)`: opclass `bit_hamming_ops` only (bit_jaccard_ops on IVFFlat is not documented by pgvector).
- Index Parts must be exactly one column part (expression parts are rejected for vector indexes).
- Index `With` keys other than `m`, `ef_construction`, `lists` are rejected.
- Unsized `vector`, `halfvec` and `sparsevec` columns are rejected (the server requires dimensions); unsized `bit` is valid. (Superseded by #29: an unsized `vector` or `halfvec` column is valid as a dimensionless column; only an ANN index on it is rejected.) `ef_construction` defaults to 64, so `m` above 32 needs an explicit `ef_construction` of at least `2*m`. `ivfflat` without `lists` is rejected by plan choice although the server defaults it to 100.
- `gen/schema.go` `addIndex` calls `ValidateVectorIndex` for hnsw and ivfflat indexes on PostgreSQL and reports its error as a positioned diagnostic.

- [ ] Step 1. Write `schema/vector_test.go` with a table of cases named by intent:

```go
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
```

Expect FAIL with `undefined: ValidateVectorIndex`.

- [ ] Step 2. Run `go test ./schema -run TestVectorIndexRules -count=1`; expect FAIL with `undefined: ValidateVectorIndex`.
- [ ] Step 3. Implement `schema/vector.go`: parse `<type>(<N>)` with a regexp `^(vector|halfvec|sparsevec|bit)\((\d+)\)$`; apply the rules above; return `fmt.Errorf("%w: ...", ErrInvalidVectorIndex)` with the rule text. `VectorIndexOpClass` maps the type prefix and metric to `<prefix>_<suffix>_ops` with suffixes `l2` (MetricL2), `ip` (MetricInnerProduct), `cosine`, `l1`, `hamming`, `jaccard`.
- [ ] Step 4. Run `go test ./schema -run TestVectorIndexRules -count=1`; expect `ok`.
- [ ] Step 5. Run `go test ./schema -run 'TestVectorIndexRules|TestSchemaJSONRoundTrip' -count=1`; expect `ok`.
- [ ] Step 6. Commit: `feat(schema): validate pgvector HNSW and IVFFlat index declarations`.

## Task 5: PostgreSQL vector DDL round trip and extension-missing error

Covers: S-15 (vector storage types in migrations), S-12 (extension, index method and opclass), failure mode "Extension missing". Depends on momo-04 (`migrate/postgres` Render and Inspect) and momo-01 (`schema.NormalizeType`). This plan adds tests and the vector-specific normalization cases; the Render and Inspect implementation is momo-04's.

Files:

- Modify `schema/normalize_test.go` (owned by momo-01): add `TestNormalizeVectorTypes`.
- Create `migrate/postgres/vector_roundtrip_test.go` (`//go:build integration`): `TestVectorMigrationRoundTrip`, `TestVectorExtensionMissing`.

Interfaces (exact, consumed from other plans): `migrate.Engine` methods from the contract (`Diff`, `Apply`, `Status`); `migrate/postgres.Dialect` implementing `migrate.Dialect{Inspect, Render, Lock}`; `schema.Equal(a, b *schema.Schema) bool`.

- [ ] Step 1. Write `TestNormalizeVectorTypes` in `schema/normalize_test.go`:

```go
func TestNormalizeVectorTypes(t *testing.T) {
	cases := map[string]string{
		"vector(1536)":    "vector(1536)",
		"VECTOR(1536)":    "vector(1536)",
		"vector( 3 )":     "vector(3)",
		"halfvec(3072)":   "halfvec(3072)",
		"sparsevec(1000)": "sparsevec(1000)",
		"bit(8)":          "bit(8)",
		"BIT(8)":          "bit(8)",
	}
	for in, want := range cases {
		if got := NormalizeType(dialect.Postgres, in); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}
```

Expect FAIL with `NormalizeType("VECTOR(1536)") = "VECTOR(1536)", want "vector(1536)"` (or the first mismatching case).

- [ ] Step 2. Implement the lowercasing and whitespace removal for the `vector|halfvec|sparsevec|bit` family inside `NormalizeType` (momo-01's file, this plan's cases only). Run `go test ./schema -run TestNormalizeVectorTypes -count=1`; expect `ok`.
- [ ] Step 3. Write `migrate/postgres/vector_roundtrip_test.go` (`//go:build integration`, package `postgres_test`). `TestVectorMigrationRoundTrip` applies the vector schema through the engine, asserts no drift, and asserts that a second diff of the same schema yields no statements:

```go
//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/migrate"
	mpg "github.com/azrtydxb/momo/migrate/postgres"
	"github.com/azrtydxb/momo/migrate/migratetest"
	"github.com/azrtydxb/momo/schema"
)

func vectorSchema() *schema.Schema {
	return &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{
			{
				Name:       "docs",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "title", Type: "text", Nullable: true},
					{Name: "emb", Type: "vector(1536)"},
					{Name: "half", Type: "halfvec(3072)"},
					{Name: "sp", Type: "sparsevec(1000)"},
					{Name: "bits", Type: "bit(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_emb_hnsw",
					Method: "hnsw",
					Parts:  []schema.IndexPart{{Column: "emb", OpClass: "vector_cosine_ops"}},
					With:   []schema.KV{{Key: "m", Value: "16"}, {Key: "ef_construction", Value: "64"}},
				}},
			},
			{
				Name:       "docs_ivf",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint"},
					{Name: "half", Type: "halfvec(64)"},
				},
				Indexes: []schema.Index{{
					Name:   "docs_half_ivf",
					Method: "ivfflat",
					Parts:  []schema.IndexPart{{Column: "half", OpClass: "halfvec_l2_ops"}},
					With:   []schema.KV{{Key: "lists", Value: "100"}},
				}},
			},
		},
	}
}

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// writeMigration writes the files that a Diff returned into dir.
func writeMigration(t *testing.T, dir string, m *migrate.Migration) {
	t.Helper()
	files := make(map[string]string, len(m.Files))
	for name, body := range m.Files {
		files[name] = string(body)
	}
	migratetest.WriteFiles(t, dir, files)
}

func TestVectorMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	eng := migratetest.NewEngine(t, mpg.Dialect{}, pgxdriver.New(newPool(t)), os.DirFS(dir))
	m, err := eng.Diff(ctx, vectorSchema(), "vectors", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	writeMigration(t, dir, m)
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	drift, err := eng.Drift(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !drift.Clean() {
		t.Fatalf("drift after apply: %+v", drift)
	}
	again, err := eng.Diff(ctx, vectorSchema(), "noop", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if again != nil && len(again.Changes) != 0 {
		t.Fatalf("second diff produced changes: %+v", again.Changes)
	}
}
```

Expect FAIL before momo-04 lands with a non-empty diff or a missing symbol. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./migrate/postgres -run TestVectorMigrationRoundTrip -count=1`; expect `ok`. The helpers `migratetest.NewEngine` and `migratetest.WriteFiles`, `DriftReport.Clean()` and `Migration.Changes` are the names plan momo-04 publishes; `Diff` returns the files and the test writes them.

- [ ] Step 4. `TestVectorExtensionMissing` (same file, package `postgres_test`): runs in a scratch database that has no pgvector extension (never `DROP EXTENSION` in the shared test database: it is database-wide, and `CASCADE` would drop the vector columns of every concurrent test), applies a migration that declares `vector(3)`, and asserts the unsupported-capability error:

```go
func TestVectorExtensionMissing(t *testing.T) {
	ctx := context.Background()
	admin := newPool(t)
	scratch := testdb.Schema(t) + "_noext"
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+scratch+`"`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+scratch+`" WITH (FORCE)`); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + scratch
	q := u.Query()
	q.Del("search_path")
	u.RawQuery = q.Encode()
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	dir := t.TempDir()
	eng := migratetest.NewEngine(t, mpg.Dialect{}, pgxdriver.New(pool), os.DirFS(dir))
	sch := &schema.Schema{
		Dialect:    "postgres",
		Extensions: []schema.Extension{{Name: "vector"}},
		Tables: []schema.Table{{
			Name:    "t",
			Columns: []schema.Column{{Name: "emb", Type: "vector(3)"}},
		}},
	}
	m, err := eng.Diff(ctx, sch, "needs_vector", migrate.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	writeMigration(t, dir, m)
	_, err = eng.Apply(ctx, migrate.ApplyOptions{})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want momo.ErrUnsupported", err)
	}
	if !strings.Contains(err.Error(), `extension "vector" is not installed`) {
		t.Fatalf("err = %v, want the missing-extension message", err)
	}
}
```

The file's import block gains `errors`, `net/url`, `strings` and `github.com/azrtydxb/momo`. Expect FAIL with a plain driver error (no `momo.ErrUnsupported`) before the Task 5 render check exists. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./migrate/postgres -run TestVectorExtensionMissing -count=1`; expect `ok`.

- [ ] Step 5. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./migrate/postgres -run 'TestVectorMigrationRoundTrip|TestVectorExtensionMissing' -count=1`; expect `ok` with momo-04 merged.
- [ ] Step 6. Commit: `test(migrate): cover pgvector migration round trip and missing extension`.

## Task 6: Scoped search settings, exact search, and iterative scans

Covers: S-15 (exact vs approximate controls; transaction-scoped search settings), S-29 (iterative index scans, max scan tuples). Acceptance: `TestSearchSettingsScoped` (S-15), `TestFilteredANNIterativeScan` (S-29).

Files:

- Create `search.go`: `IterativeScan`, `SearchSettings`, `Validate`, `DB.WithSearch`, `Tx.SetSearch`, `searchSettingPairs`.
- Create `search_test.go`: `TestSearchSettingsValidation`, `TestSearchSQL`. Create `search_fake_test.go` (package `momo_test`): `TestSearchSetLocalBinds`.
- Create `search_integration_test.go` (`//go:build integration`): `TestSearchSettingsScoped`, `TestFilteredANNIterativeScan`.

Interfaces (exact):

```go
type IterativeScan string

const (
	IterativeOff          IterativeScan = "off"
	IterativeRelaxedOrder IterativeScan = "relaxed_order"
	IterativeStrictOrder  IterativeScan = "strict_order" // hnsw only
)

// SearchSettings applies pgvector session settings for one transaction only.
// Zero values mean "do not change the server default".
type SearchSettings struct {
	ExactSearch          bool          // enable_indexscan = off and enable_bitmapscan = off
	HNSWEfSearch         int           // hnsw.ef_search, 1..1000
	HNSWIterativeScan    IterativeScan // hnsw.iterative_scan
	HNSWMaxScanTuples    int           // hnsw.max_scan_tuples, 1..2147483647 (server default 20000)
	IVFFlatProbes        int           // ivfflat.probes, 1..32768
	IVFFlatIterativeScan IterativeScan // ivfflat.iterative_scan: off or relaxed_order only
	IVFFlatMaxProbes     int           // ivfflat.max_probes, 1..32768
}

func (s SearchSettings) Validate() error // wraps ErrValidation
func (db *DB) WithSearch(ctx context.Context, s SearchSettings, fn func(*Tx) error) error
func (tx *Tx) SetSearch(ctx context.Context, s SearchSettings) error

type settingPair struct{ Name, Value string }

// searchSettingPairs returns the non-zero settings in this fixed order: enable_indexscan,
// enable_bitmapscan, hnsw.ef_search, hnsw.iterative_scan, hnsw.max_scan_tuples, ivfflat.probes,
// ivfflat.iterative_scan, ivfflat.max_probes. Integers are formatted with strconv.Itoa.
func searchSettingPairs(s SearchSettings) []settingPair
```

Binding rule: every setting is applied with `tx.SetLocal(ctx, pair.Name, pair.Value)` (plan momo-04 Task 23, contract method `Tx.SetLocal(ctx, name, value string) error`), which renders `SELECT set_config($1, $2, true)`. Name and value are bound parameters, and nothing is interpolated into SQL text. `SetSearch` returns `ErrUnsupported` on SQLite when any field other than `ExactSearch` is non-zero; `ExactSearch` alone is a no-op on SQLite.

- [ ] Step 1. Write `search_test.go`:

```go
package momo

import (
	"errors"
	"slices"
	"testing"

)

func TestSearchSettingsValidation(t *testing.T) {
	bad := []SearchSettings{
		{HNSWEfSearch: 1001},
		{HNSWEfSearch: -1},
		{HNSWIterativeScan: "sometimes"},
		{IVFFlatIterativeScan: IterativeStrictOrder}, // ivfflat has no strict_order
		{HNSWMaxScanTuples: -5},
	}
	for _, s := range bad {
		if err := s.Validate(); !errors.Is(err, ErrValidation) {
			t.Errorf("%+v: err = %v, want ErrValidation", s, err)
		}
	}
}

func TestSearchSQL(t *testing.T) {
	got := searchSettingPairs(SearchSettings{ExactSearch: true, HNSWEfSearch: 100, HNSWIterativeScan: IterativeRelaxedOrder, HNSWMaxScanTuples: 5000})
	want := []settingPair{
		{"enable_indexscan", "off"},
		{"enable_bitmapscan", "off"},
		{"hnsw.ef_search", "100"},
		{"hnsw.iterative_scan", "relaxed_order"},
		{"hnsw.max_scan_tuples", "5000"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("pairs = %v, want %v", got, want)
	}
}

```

Write `search_fake_test.go` (package `momo_test`, external because it imports `internal/fakedriver`):

```go
package momo_test

import (
	"slices"
	"testing"

	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestSearchSetLocalBinds(t *testing.T) {
	rec := fakedriver.NewRecorder() // statement recorder from internal/fakedriver (plan momo-01)
	tx := rec.Tx()
	if err := tx.SetSearch(t.Context(), SearchSettings{HNSWEfSearch: 100}); err != nil {
		t.Fatal(err)
	}
	stmt := rec.Last()
	if stmt.SQL != "SELECT set_config($1, $2, true)" {
		t.Fatalf("SQL = %q, want a bound set_config call", stmt.SQL)
	}
	if !slices.Equal(stmt.Args, []any{"hnsw.ef_search", "100"}) {
		t.Fatalf("args = %v, want the name and value bound as parameters", stmt.Args)
	}
}
```

Expect FAIL with `undefined: searchSettingPairs`.

- [ ] Step 2. Implement `search.go`: `Validate`, `searchSettingPairs`, `Tx.SetSearch` (validates, then calls `tx.SetLocal` per pair and returns the first error), `DB.WithSearch` (wraps `db.Tx(ctx, TxOptions{}, ...)` and calls `SetSearch` before `fn`). Run `go test . -run 'TestSearchSettingsValidation|TestSearchSQL|TestSearchSetLocalBinds' -count=1`; expect `ok`.
- [ ] Step 3. Write `TestSearchSettingsScoped` (integration, package `momo_test`) in `search_integration_test.go`. The pool has one connection, so every transaction reuses the same backend. The check runs `current_setting` inside the transaction and again after it:

```go
//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/internal/testdb"
)

// singleConnPool returns a pool with one connection, so every transaction reuses the same backend.
func singleConnPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// checkSetting raises an error unless the named setting has the expected value.
func checkSetting(ctx context.Context, q momo.Querier, name, want string) error {
	_, err := momo.Exec(ctx, q, "DO $$ BEGIN IF current_setting('"+name+"') <> '"+want+"' THEN RAISE EXCEPTION 'setting "+name+" is not "+want+"'; END IF; END $$")
	return err
}

func TestSearchSettingsScoped(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(4))")
	mustExecPG(t, db, "CREATE INDEX docs_emb ON docs USING hnsw (emb vector_l2_ops)")
	err := db.WithSearch(ctx, momo.SearchSettings{HNSWEfSearch: 200}, func(tx *momo.Tx) error {
		return checkSetting(ctx, tx, "hnsw.ef_search", "200")
	})
	if err != nil {
		t.Fatalf("inside WithSearch: %v", err)
	}
	if err := checkSetting(ctx, db, "hnsw.ef_search", "40"); err != nil {
		t.Fatalf("setting leaked after the transaction: %v", err)
	}
}
```

Expect FAIL with `undefined: WithSearch` before Task 6 Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestSearchSettingsScoped -count=1`; expect `ok`.

- [ ] Step 4. Write `TestFilteredANNIterativeScan` (integration, package `momo_test`, same file). It loads 20000 seeded rows with 1% `rare` rows, runs the filtered nearest-neighbour query with iterative scan off and on, and counts the rows through a temporary table. A counted sink table avoids reading rows through the ORM:

```go
// hitCount copies the rows of table into a temporary sink and returns how many were copied.
func hitCount(ctx context.Context, tx *momo.Tx, table string) (int64, error) {
	if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE sink_"+table+" (id bigint)"); err != nil {
		return 0, err
	}
	return momo.Exec(ctx, tx, "INSERT INTO sink_"+table+" SELECT id FROM "+table)
}

func TestFilteredANNIterativeScan(t *testing.T) {
	ctx := context.Background()
	db := momo.New(pgxdriver.New(singleConnPool(t)))
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"CREATE TABLE items (id bigint PRIMARY KEY, grp text NOT NULL, emb vector(8) NOT NULL)",
		"SELECT setseed(0.42)",
		"INSERT INTO items SELECT g, CASE WHEN g % 100 = 0 THEN 'rare' ELSE 'common' END, ARRAY(SELECT random()::real FROM generate_series(1, 8))::vector FROM generate_series(1, 20000) AS g",
		"CREATE INDEX items_emb ON items USING hnsw (emb vector_l2_ops)",
		"ANALYZE items",
	} {
		mustExecPG(t, db, stmt)
	}
	count := func(name string, s momo.SearchSettings) int64 {
		var n int64
		err := db.WithSearch(ctx, s, func(tx *momo.Tx) error {
			if _, err := momo.Exec(ctx, tx, "CREATE TEMP TABLE "+name+" AS SELECT id FROM items WHERE grp = 'rare' ORDER BY emb <-> (SELECT emb FROM items WHERE id = 1) LIMIT 10"); err != nil {
				return err
			}
			var err error
			n, err = hitCount(ctx, tx, name)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	off := count("hits_off", momo.SearchSettings{})
	on := count("hits_on", momo.SearchSettings{HNSWIterativeScan: momo.IterativeRelaxedOrder, HNSWMaxScanTuples: 20000})
	if off >= 10 {
		t.Fatalf("iterative scan off returned %d rows, want fewer than 10", off)
	}
	if on != 10 {
		t.Fatalf("iterative scan on returned %d rows, want 10", on)
	}
	if err := checkSetting(ctx, db, "hnsw.iterative_scan", "off"); err != nil {
		t.Fatalf("setting leaked after the transaction: %v", err)
	}
}
```

Expect FAIL with `undefined: WithSearch` before Task 6 Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestFilteredANNIterativeScan -count=1`; expect `ok`.

- [ ] Step 5. Commit: `feat(momo): add scoped pgvector search settings bound through set_config`.

## Task 7: EXPLAIN inspection and index-usage verification

Covers: S-15 (EXPLAIN inspection; index used), S-15 acceptance `TestVectorIndexUsed`. The general `Explain` helper is part of S-18 and is named here so the vector tests have a stable call; momo-05 or the observability owner adds `ExplainAnalyze` and must not change `Explain`'s behaviour.

Files:

- Create `explain.go`: `func (q SelectQuery[T]) Explain(ctx context.Context) (string, error)`.
- Create `explain_integration_test.go` (`//go:build integration`): `TestVectorIndexUsed`.

Interfaces (exact):

```go
// Explain runs EXPLAIN (never EXPLAIN ANALYZE) for the query and returns the plan text,
// one line per row joined by newlines. Parameters are bound, never interpolated.
func (q SelectQuery[T]) Explain(ctx context.Context) (string, error)
```

- [ ] Step 1. Write `TestVectorIndexUsed` in `explain_integration_test.go` (`//go:build integration`, package `momo_test`). It uses `vecDoc` (defined in `vector_integration_test.go`, plan line 584) and the `mustExecPG` helper (plan line 612). The test creates `docs(id bigint primary key, emb vector(8))` with the HNSW index, inserts 5000 rows from `math/rand/v2` with the fixed PCG seed `(11, 11)`, runs `ANALYZE docs`, and asserts that EXPLAIN names the index. It then swaps in an IVFFlat index and repeats the assertion in a subtest:

```go
//go:build integration

package momo_test

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

func TestVectorIndexUsed(t *testing.T) {
	ctx := context.Background()
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "DROP TABLE IF EXISTS docs")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(8))")
	mustExecPG(t, db, "CREATE INDEX docs_emb_hnsw ON docs USING hnsw (emb vector_l2_ops)")

	// Fixed PCG seed: the same 5000 vectors and the same query on every run.
	rng := rand.New(rand.NewPCG(11, 11))
	randVec := func() vector.Vector {
		v := make(vector.Vector, 8)
		for i := range v {
			v[i] = rng.Float32()
		}
		return v
	}
	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		for i := 1; i <= 5000; i++ {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i), randVec()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mustExecPG(t, db, "ANALYZE docs")

	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", 8, 0)
	query := randVec()

	// checkPlan runs EXPLAIN on the KNN query. enable_seqscan is off for this
	// transaction only (set_config(..., true)), so the planner must pick an index.
	checkPlan := func(t *testing.T, indexName string) {
		t.Helper()
		var plan string
		if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if err := tx.SetLocal(ctx, "enable_seqscan", "off"); err != nil {
				return err
			}
			var err error
			plan, err = momo.Select[vecDoc, *vecDoc](tx).
				OrderBy(momo.L2Distance(col, query).Asc()).
				Limit(10).
				Explain(ctx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(plan, "\n") {
			if strings.Contains(line, indexName) {
				return
			}
		}
		t.Fatalf("EXPLAIN does not use index %s:\n%s", indexName, plan)
	}

	checkPlan(t, "docs_emb_hnsw")

	t.Run("IVFFlat", func(t *testing.T) {
		mustExecPG(t, db, "DROP INDEX docs_emb_hnsw")
		mustExecPG(t, db, "CREATE INDEX docs_emb_ivf ON docs USING ivfflat (emb vector_l2_ops) WITH (lists = 10)")
		mustExecPG(t, db, "ANALYZE docs")
		checkPlan(t, "docs_emb_ivf")
	})
}
```

- [ ] Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration -run TestVectorIndexUsed .`; expect FAIL with `has no field or method Explain` (the compiler error for `SelectQuery[vecDoc]`, because `Explain` does not exist yet).
- [ ] Step 3. Implement `Explain`: build the statement through the existing `SQL()` path, prepend `EXPLAIN ` on PostgreSQL (and `EXPLAIN QUERY PLAN ` on SQLite), run through `q.conn()`, and join rows.
- [ ] Step 4. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration -run TestVectorIndexUsed .`; expect `ok` (with `-v`, the parent and the `IVFFlat` subtest each report `--- PASS`).
- [ ] Step 5. Commit: `feat(momo): add Explain and vector index-usage verification`.

## Task 8: pgx binary codecs for all four vector types

Covers: S-50 (pgx binary codecs for vector, halfvec, sparsevec and bit, benchmarked against text encoding). Acceptance: `TestVectorBinaryCodec`, `BenchmarkVectorEncoding` results recorded under `bench/results` by momo-12.

Files:

- Create `driver/pgxdriver/vector_codec.go`: `ConfigureVectors`, `VectorFormat`, `WithVectorFormat`, codecs.
- Create `driver/pgxdriver/vector_codec_test.go`: `TestVectorBinaryCodec`.
- Create `driver/pgxdriver/vector_bench_test.go`: `BenchmarkVectorEncoding`.
- Modify `driver/pgxdriver/pgxdriver.go` (momo-03 file): add the `WithVectorFormat` option wiring only.

Interfaces (exact):

```go
// driver/pgxdriver (package pgxdriver)
type VectorFormat int

const (
	VectorText   VectorFormat = iota // default: text encoding for every vector type
	VectorBinary                     // pgx binary codecs; requires ConfigureVectors
)

// ConfigureVectors installs an AfterConnect hook on cfg that looks up the OIDs of
// vector, halfvec, sparsevec and bit and registers pgtype codecs for them.
// It must be called before pgxpool.NewWithConfig.
func ConfigureVectors(cfg *pgxpool.Config)

func WithVectorFormat(f VectorFormat) Option
```

Binary layouts (verify each against the pgvector source before coding: `src/vector.c` `vector_send`, `src/halfvec.c` `halfvec_send`, `src/sparsevec.c` `sparsevec_send`, and PostgreSQL `src/backend/utils/adt/varbit.c` `bit_send`): vector is `uint16 dim, uint16 unused, float32[dim]` big-endian; halfvec is the same shape with float16 elements; sparsevec is `int32 dim, int32 nnz, int32 unused, int32 indices[nnz] (0-based), float32 values[nnz]`; bit is `int32 bitlen, bytes[ceil(bitlen/8)]`. The test in Step 1 is the arbiter: binary and text reads must produce equal values.

- [ ] Step 1. Write `driver/pgxdriver/vector_codec_test.go` (`//go:build integration`):

```go
func TestVectorBinaryCodec(t *testing.T) {
	url := testdb.PostgresURL(t) // from internal/testdb
	for _, format := range []VectorFormat{VectorText, VectorBinary} {
		cfg, err := pgxpool.ParseConfig(url)
		if err != nil {
			t.Fatal(err)
		}
		ConfigureVectors(cfg)
		pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil { t.Fatal(err) }
		drv := New(pool, WithVectorFormat(format))
		// insert one row per type, read back via momo Select, compare with inputs
		t.Log(drv.Dialect().Name)
		pool.Close()
	}
}
```

Complete the body with: create `codec_t(id int, v vector(3), h halfvec(3), s sparsevec(5), b bit(9))`; insert `vector.Vector{1, 2.5, -3}`, `vector.HalfVector{0.5, 1, 2}`, `vector.SparseVector{Dim: 5, Indices: []int32{0, 4}, Values: []float32{1, -2}}`, `vector.Bits{Len: 9, Data: []byte{0xAA, 0x80}}`; read back and assert each equals its input and that the value read in the two formats is identical. Expect FAIL with `undefined: ConfigureVectors`.

- [ ] Step 2. Implement `driver/pgxdriver/vector_codec.go`. `ConfigureVectors` chains the previous `AfterConnect`, then for each name in `{"vector","halfvec","sparsevec","bit"}` queries `SELECT $1::regtype::oid` and calls `c.TypeMap().RegisterType(&pgtype.Type{Name: name, OID: oid, Codec: vectorCodec{kind}})`. `vectorCodec` implements `pgtype.Codec` (`FormatSupported`, `PreferredFormat`, `PlanEncode`, `PlanScan`, `DecodeDatabaseSQLValue`, `DecodeValue`) and handles both formats; `PlanEncode` accepts `vector.Vector`, `vector.HalfVector`, `vector.SparseVector` and `vector.Bits` (and pointers). The text plan emits the same bytes as `Value()`. In `VectorBinary` mode the pgx driver passes typed values unchanged to pgx; in `VectorText` mode it passes `driver.Valuer` results (strings). Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration ./driver/pgxdriver -run TestVectorBinaryCodec -count=1`; expect `ok`.
- [ ] Step 3. Write `BenchmarkVectorEncoding` in `driver/pgxdriver/vector_bench_test.go` (package `pgxdriver`). Each sub-benchmark inserts 1000 values per iteration through the driver in one format, for the four types:

```go
package pgxdriver

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/vector"
)

type encodingCase struct {
	name  string
	ddl   string
	value func(r *rand.Rand) any
}

var encodingCases = []encodingCase{
	{"vector-1536", "vector(1536)", func(r *rand.Rand) any { return vector.Vector(randomFloats(r, 1536)) }},
	{"halfvec-3072", "halfvec(3072)", func(r *rand.Rand) any { return vector.HalfVector(randomFloats(r, 3072)) }},
	{"sparsevec-1000", "sparsevec(1000)", func(r *rand.Rand) any { return randomSparse(r, 1000, 100) }},
	{"bit-1024", "bit(1024)", func(r *rand.Rand) any { return randomBits(r, 1024) }},
}

func randomFloats(r *rand.Rand, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(r.Float64()*2 - 1)
	}
	return out
}

// randomSparse returns nnz distinct, sorted, 0-based indexes below dim.
func randomSparse(r *rand.Rand, dim, nnz int) vector.SparseVector {
	idx := r.Perm(dim)[:nnz]
	sort.Ints(idx)
	s := vector.SparseVector{Dim: dim, Indices: make([]int32, nnz), Values: make([]float32, nnz)}
	for i, j := range idx {
		s.Indices[i] = int32(j)
		s.Values[i] = float32(r.Float64()*2 - 1)
	}
	return s
}

func randomBits(r *rand.Rand, n int) vector.Bits {
	data := make([]byte, (n+7)/8)
	r.Read(data)
	return vector.Bits{Len: n, Data: data}
}

// openEncodingDriver returns a driver on a fresh pool with a table `enc` of the given column type.
func openEncodingDriver(b *testing.B, format VectorFormat, ddl string) momo.Driver {
	b.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(testdb.PostgresURL(b))
	if err != nil {
		b.Fatal(err)
	}
	if format == VectorBinary {
		ConfigureVectors(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)
	for _, stmt := range []string{
		"CREATE EXTENSION IF NOT EXISTS vector",
		"DROP TABLE IF EXISTS enc",
		"CREATE TABLE enc (id bigint, v " + ddl + ")",
	} {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			b.Fatal(err)
		}
	}
	return New(pool, WithVectorFormat(format))
}

func formatName(f VectorFormat) string {
	if f == VectorBinary {
		return "binary"
	}
	return "text"
}

func BenchmarkVectorEncoding(b *testing.B) {
	for _, format := range []VectorFormat{VectorText, VectorBinary} {
		for _, c := range encodingCases {
			b.Run(fmt.Sprintf("%s/%s", formatName(format), c.name), func(b *testing.B) {
				drv := openEncodingDriver(b, format, c.ddl)
				ctx := context.Background()
				r := rand.New(rand.NewSource(1))
				values := make([]any, 1000)
				for i := range values {
					values[i] = c.value(r)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for n, v := range values {
						if _, err := drv.Exec(ctx, "INSERT INTO enc (id, v) VALUES ($1, $2)", []any{int64(n), v}); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}
```

Expect FAIL with `undefined: ConfigureVectors` before Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test ./driver/pgxdriver -run '^$' -bench BenchmarkVectorEncoding -benchmem -benchtime=2s`; expect eight result lines, one per sub-benchmark.

- [ ] Step 4. Commit: `feat(pgxdriver): add binary codecs for pgvector types`.

## Task 9: PostgreSQL hybrid search (full-text plus vector, RRF)

Covers: S-17 (hybrid search with explicit candidate selection, rank fusion, deterministic tie-breaking). Built after Tasks 1 to 8 are verified, as the spec requires. Acceptance: `TestHybridSearch`.

Files:

- Create `hybrid.go`: `HybridSpec[T]`, `HybridResult[T]`, `HybridSearch`, `HybridSQL`.
- Create `hybrid_test.go`: `TestHybridSQL` (SQL shape, bound args, candidate and RRF parameters).
- Create `hybrid_integration_test.go` (`//go:build integration`): `TestHybridSearch`.

Interfaces (exact):

```go
type HybridSpec[T any] struct {
	Text           Column[string]     // full-text column of the model
	TextConfig     string             // regconfig name, e.g. "english"; bound as a parameter
	Query          string             // websearch_to_tsquery input
	Vector         VectorColumn[vector.Vector]
	QueryVector    vector.Vector
	TextCandidates int                // default 100; must be 1..1000
	VectorCandidates int              // default 100; must be 1..1000
	RRFK           int                // default 60 (Cormack, Clarke and Buettcher 2009); must be >= 1
	Limit          int                // final rows, 1..1000
}

type HybridResult[T any] struct {
	Row   T
	Score float64 // sum of 1/(RRFK+rank) over the two candidate lists
}

// HybridSearch builds the fused query. PostgreSQL only; other dialects return ErrUnsupported.
func HybridSearch[T any, PT Modeler[T]](q Querier, spec HybridSpec[T]) HybridQuery[T]
func (h HybridQuery[T]) All(ctx context.Context) ([]HybridResult[T], error)
func (h HybridQuery[T]) SQL() (Compiled, error)
```

Fused SQL shape (the implementer renders the table and primary key from `Meta[T]`; `$n` placeholders are bound in this order: text config, query, vector, rrf k, text candidates, vector candidates, limit):

```sql
WITH text_hits AS (
  SELECT pk, row_number() OVER (ORDER BY score DESC, pk ASC) AS rnk
  FROM (
    SELECT t.pk, ts_rank_cd(to_tsvector($1::regconfig, t.text), websearch_to_tsquery($1::regconfig, $2)) AS score
    FROM tbl t
    WHERE to_tsvector($1::regconfig, t.text) @@ websearch_to_tsquery($1::regconfig, $2)
    ORDER BY score DESC, t.pk ASC
    LIMIT $5
  ) s
),
vec_hits AS (
  SELECT pk, row_number() OVER (ORDER BY dist ASC, pk ASC) AS rnk
  FROM (
    SELECT t.pk, t.emb <=> $3 AS dist
    FROM tbl t
    ORDER BY dist ASC, t.pk ASC
    LIMIT $6
  ) s
),
fused AS (
  SELECT COALESCE(a.pk, b.pk) AS pk,
         COALESCE(1.0 / ($4 + a.rnk), 0) + COALESCE(1.0 / ($4 + b.rnk), 0) AS score
  FROM text_hits a FULL OUTER JOIN vec_hits b ON a.pk = b.pk
)
SELECT t.*, f.score FROM fused f JOIN tbl t ON t.pk = f.pk
ORDER BY f.score DESC, f.pk ASC
LIMIT $7;
```

Note: `<=>` is cosine distance; the spec's vector leg uses the metric of the `Vector` column's index. The rendering must use the operator chosen from `CosineDistance` for this plan; a different metric is a new `HybridSpec` field and not part of this task.

- [ ] Step 1. Write `hybrid_test.go` (package `momo`, internal because it calls the unexported `Builder` path) with `TestHybridSQL`. It renders the statement for a fixture model and checks the SQL text, the bound argument order and the RRF validation:

```go
package momo

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

type hybridDoc struct {
	ID   int64
	Body string
	Emb  vector.Vector
}

func (d *hybridDoc) MomoMeta() *Meta[hybridDoc] {
	return &Meta[hybridDoc]{Table: "docs", PK: []int{0}, Columns: []ColumnMeta{{Name: "id"}, {Name: "body"}, {Name: "emb"}}}
}

// dialectQuerier satisfies the sealed Querier interface for SQL-only tests.
type dialectQuerier struct{ d *dialect.Dialect }

func (q dialectQuerier) Dialect() *dialect.Dialect { return q.d }
func (q dialectQuerier) conn() *execer             { return nil }

func hybridSpec() HybridSpec[hybridDoc] {
	return HybridSpec[hybridDoc]{
		Text:        NewColumn[string]("docs", "body", 0),
		TextConfig:  "english",
		Query:       "vector databases",
		Vector:      NewVectorColumn[vector.Vector]("docs", "emb", 3, 0),
		QueryVector: vector.Vector{1, 0, 0},
		Limit:       10,
	}
}

func TestHybridSQL(t *testing.T) {
	q := dialectQuerier{dialect.For(dialect.Postgres)}
	c, err := HybridSearch[hybridDoc, *hybridDoc](q, hybridSpec()).SQL()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"websearch_to_tsquery($1::regconfig, $2)",
		"<=> $3",
		"FULL OUTER JOIN",
		"ORDER BY f.score DESC, f.pk ASC",
	} {
		if !strings.Contains(c.SQL, want) {
			t.Errorf("SQL lacks %q:\n%s", want, c.SQL)
		}
	}
	// Bind order: text config, query, query vector, RRF k, text candidates, vector candidates, limit.
	wantArgs := []any{"english", "vector databases", vector.Vector{1, 0, 0}, 60, 100, 100, 10}
	if !reflect.DeepEqual(c.Args, wantArgs) {
		t.Fatalf("args = %v, want %v", c.Args, wantArgs)
	}
	bad := hybridSpec()
	bad.RRFK = -1
	if _, err := HybridSearch[hybridDoc, *hybridDoc](q, bad).SQL(); !errors.Is(err, ErrValidation) {
		t.Fatalf("RRFK=-1: err = %v, want ErrValidation", err)
	}
}
```

Expect FAIL with `undefined: HybridSearch` before Step 2.

- [ ] Step 2. Implement `hybrid.go` with the validation, the rendering above through the `Builder`, and `All` scanning `T` columns plus the score. Run `go test . -run TestHybridSQL -count=1`; expect `ok`.
- [ ] Step 3. Write `TestHybridSearch` (integration, package `momo_test`, file `hybrid_integration_test.go`). The corpus has an exact expected order, computed from RRF k=60 (`id 1` is text rank 1 and vector rank 1, so `2/61`; `id 3`, `id 2` and `id 4` are vector-only with ranks 2, 3 and 4). A second subtest builds a tie on the fused score and checks the primary-key tie-break:

```go
//go:build integration

package momo_test

import (
	"context"
	"slices"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

type hybridRow struct {
	ID   int64
	Body string
	Emb  vector.Vector
}

func (r *hybridRow) MomoMeta() *momo.Meta[hybridRow] {
	return &momo.Meta[hybridRow]{
		Table:   "docs",
		PK:      []int{0},
		Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "body"}, {Name: "emb"}},
		Ptr: func(r *hybridRow, i int) any {
			return [...]any{&r.ID, &r.Body, &r.Emb}[i]
		},
		Value: func(r *hybridRow, i int) any {
			return [...]any{r.ID, r.Body, r.Emb}[i]
		},
	}
}

func hybridSpec() momo.HybridSpec[hybridRow] {
	return momo.HybridSpec[hybridRow]{
		Text:        momo.NewColumn[string]("docs", "body", 0),
		TextConfig:  "english",
		Query:       "vector search",
		Vector:      momo.NewVectorColumn[vector.Vector]("docs", "emb", 4, 0),
		QueryVector: vector.Vector{1, 0, 0, 0},
		Limit:       10,
	}
}

// setupHybrid creates a fresh database with the docs table and the given rows.
func setupHybrid(t *testing.T, rows ...hybridRow) momo.Querier {
	t.Helper()
	db := testopen.Postgres(t)
	mustExecPG(t, db, "CREATE EXTENSION IF NOT EXISTS vector")
	mustExecPG(t, db, "CREATE TABLE docs (id bigint PRIMARY KEY, body text NOT NULL, emb vector(4) NOT NULL)")
	for _, r := range rows {
		mustExecPG(t, db, "INSERT INTO docs (id, body, emb) VALUES (?, ?, ?)", r.ID, r.Body, r.Emb)
	}
	return db
}

func runHybrid(t *testing.T, q momo.Querier, spec momo.HybridSpec[hybridRow]) []int64 {
	t.Helper()
	res, err := momo.HybridSearch[hybridRow, *hybridRow](q, spec).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(res))
	for _, r := range res {
		ids = append(ids, r.Row.ID)
	}
	return ids
}

func TestHybridSearch(t *testing.T) {
	corpus := []hybridRow{
		{ID: 1, Body: "vector search", Emb: vector.Vector{1, 0, 0, 0}},
		{ID: 2, Body: "vector", Emb: vector.Vector{0, 1, 0, 0}},
		{ID: 3, Body: "search", Emb: vector.Vector{1, 0, 0, 0}},
		{ID: 4, Body: "unrelated words", Emb: vector.Vector{0, 0, 1, 0}},
	}
	want := []int64{1, 3, 2, 4}
	if got := runHybrid(t, setupHybrid(t, corpus...), hybridSpec()); !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	t.Run("insertion order does not change the result", func(t *testing.T) {
		rev := slices.Clone(corpus)
		slices.Reverse(rev)
		if got := runHybrid(t, setupHybrid(t, rev...), hybridSpec()); !slices.Equal(got, want) {
			t.Fatalf("order = %v, want %v", got, want)
		}
	})
	t.Run("tie on fused score breaks on primary key", func(t *testing.T) {
		// id 5 is text rank 1 and vector rank 2; id 9 is text rank 2 and vector rank 1.
		// Both fused scores are 1/61 + 1/62, so the smaller primary key comes first.
		tie := []hybridRow{
			{ID: 9, Body: "vector search", Emb: vector.Vector{1, 0, 0, 0}},
			{ID: 5, Body: "vector search", Emb: vector.Vector{0.70710678, 0.70710678, 0, 0}},
		}
		if got := runHybrid(t, setupHybrid(t, tie...), hybridSpec()); !slices.Equal(got, []int64{5, 9}) {
			t.Fatalf("tie order = %v, want [5 9]", got)
		}
	})
	t.Run("text candidate count is bound", func(t *testing.T) {
		spec := hybridSpec()
		spec.TextCandidates = 2
		c, err := momo.HybridSearch[hybridRow, *hybridRow](setupHybrid(t, corpus...), spec).SQL()
		if err != nil {
			t.Fatal(err)
		}
		if c.Args[4] != 2 {
			t.Fatalf("text candidate argument = %v, want 2", c.Args[4])
		}
	})
}
```

Expect FAIL with `undefined: HybridSearch` before Task 9 Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$URL go test -tags integration . -run TestHybridSearch -count=1`; expect `ok`.

- [ ] Step 4. Commit: `feat(momo): add PostgreSQL hybrid full-text and vector search`.

## Task 10: Pinned pgvector versions and version assertion

Covers: S-15 (record the pgvector version in the image used), S-20 (CI integration against pgvector images). Acceptance: `TestPgvectorVersionPinned` (added here; it is the stable assertion that CI runs the pinned extension).

Files:

- Modify `internal/testdb/compose.yaml` (momo-01 file): service `postgres` image `pgvector/pgvector:0.8.7-pg17`; service `postgres-pg16` image `pgvector/pgvector:0.8.7-pg16`.
- Create `internal/testdb/pgvector_version_test.go`: `TestPgvectorVersionPinned`.

- [ ] Step 1. Write the test:

```go
func TestPgvectorVersionPinned(t *testing.T) {
	conn, err := pgx.Connect(context.Background(), PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var v string
	if err := conn.QueryRow(context.Background(), `SELECT extversion FROM pg_extension WHERE extname = 'vector'`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != "0.8.7" {
		t.Fatalf("pgvector extversion = %s, want 0.8.7 (see docs/vectors.md)", v)
	}
}
```

Expect FAIL with `pgvector extversion = <none>` if the database has no vector extension yet, or with the actual older version if the compose file is not updated.

- [ ] Step 2. Update `compose.yaml` to the two images above and run `make db-up`, then `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=postgres://momo:momo@localhost:5432/momo?sslmode=disable go test -tags integration ./internal/testdb -run TestPgvectorVersionPinned -count=1`; expect `ok`. Repeat with the pg16 URL on port 5433; expect `ok`.
- [ ] Step 3. Commit: `chore(testdb): pin pgvector 0.8.7 on pg17 and pg16`.

## Task 11: sqlite-vec loading, metrics and ANN rejection

Covers: S-16 (sqlite-vec via pinned version; loading; metrics; types; ANN request returns ErrUnsupported). Acceptance: `TestSQLiteVectorParity`, and the ANN rejection test `TestSQLiteANNIndexUnsupported`. The pinned version and the metric table are documented in Task 12.

Verified facts (embedded source of github.com/asg017/sqlite-vec-go-bindings/cgo v0.1.6, read 2026-11-04): the registered distance functions are `vec_distance_l2`, `vec_distance_l1`, `vec_distance_cosine` and `vec_distance_hamming` (bit vectors only). `vec_version()` is registered separately and returns `SQLITE_VEC_VERSION`, which is `"v0.1.6"`. The bindings use `sqlite_vec.Auto()` to register the statically linked extension.

Files:

- Create `driver/sqlitedriver/vec.go`: `SQLiteVecVersion`, `Open`.
- Create `driver/sqlitedriver/vec_test.go`: `TestSQLiteVecLoaded`, `TestSQLiteVecMetrics`, `TestSQLiteANNIndexUnsupported`.
- Create `driver/sqlitedriver/parity_integration_test.go` (`//go:build integration`): `TestSQLiteVectorParity`.

Interfaces (exact):

```go
// driver/sqlitedriver
const SQLiteVecVersion = "v0.1.6"

// Open registers the statically linked sqlite-vec extension (sqlite_vec.Auto from
// github.com/asg017/sqlite-vec-go-bindings/cgo), opens the database and checks that
// vec_version() returns SQLiteVecVersion. Vector columns are BLOB columns with a length CHECK,
// for example CHECK (length(emb) = 4*1536); bit columns use CHECK (length(bits) = (N+7)/8).
func Open(dsn string, cfg Config) (*sql.DB, momo.Driver, error)

// openVecForTest opens an in-memory database through Open and closes it with t.Cleanup.
func openVecForTest(t *testing.T) *sql.DB
```

- [ ] Step 1. Write `driver/sqlitedriver/vec_test.go`:

```go
package sqlitedriver

import (
	"database/sql"
	"errors"
	"math"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/schema"
)

func openVecForTest(t *testing.T) *sql.DB {
	t.Helper()
	db, _, err := Open(":memory:", DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestSQLiteVecLoaded(t *testing.T) {
	db := openVecForTest(t)
	var v string
	if err := db.QueryRow("SELECT vec_version()").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != SQLiteVecVersion {
		t.Fatalf("vec_version() = %q, want %q (embedded by bindings v0.1.6)", v, SQLiteVecVersion)
	}
}

func TestSQLiteVecMetrics(t *testing.T) {
	db := openVecForTest(t)
	bitBlob := func(b byte) []byte { return []byte{b} }
	cases := []struct {
		name  string
		query string
		args  []any
		want  float64
	}{
		{"l2", "SELECT vec_distance_l2(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, math.Sqrt(27)},
		{"l1", "SELECT vec_distance_l1(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, 9},
		{"cosine", "SELECT vec_distance_cosine(vec_f32('[1,2,3]'), vec_f32('[4,5,6]'))", nil, 1 - 32/math.Sqrt(14*77)},
		{"hamming", "SELECT vec_distance_hamming(vec_bit(?), vec_bit(?))", []any{bitBlob(0xB0), bitBlob(0x98)}, 2},
	}
	for _, c := range cases {
		var got float64
		if err := db.QueryRow(c.query, c.args...).Scan(&got); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if math.Abs(got-c.want) > 1e-5 {
			t.Fatalf("%s = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSQLiteANNIndexUnsupported(t *testing.T) {
	desired := &schema.Schema{Dialect: "sqlite", Tables: []schema.Table{{
		Name:    "docs",
		Columns: []schema.Column{{Name: "emb", Type: "BLOB"}},
		Indexes: []schema.Index{{Name: "docs_emb", Method: "hnsw", Parts: []schema.IndexPart{{Column: "emb"}}}},
	}}}
	db := openVecForTest(t)
	_, err := sqliteEngineDiff(db, desired) // builds a migrate.Engine over db (plan momo-04) and calls Diff
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("hnsw index on sqlite: err = %v, want momo.ErrUnsupported", err)
	}
}
```

`sqliteEngineDiff` is a one-line helper in the same file that builds a `migrate.Engine` over the database and calls `Diff` for the desired schema. Expect FAIL with `undefined: Open` before Step 4.

- [ ] Step 2. Run `CGO_ENABLED=1 go test ./driver/sqlitedriver -run 'TestSQLiteVecLoaded|TestSQLiteVecMetrics' -count=1`; expect FAIL with `undefined: Open`.
- [ ] Step 3. Implement `driver/sqlitedriver/vec.go`: call `sqlite_vec.Auto()` before `sql.Open("sqlite3", dsn)`, apply the `Config` pragmas owned by plan momo-03, and check `SELECT vec_version()` equals `SQLiteVecVersion`, returning an error otherwise. Implement the ANN rejection in `schema.ValidateVectorIndex` (plan momo-07 Task 4 file) by returning `ErrInvalidVectorIndex` for `dialect.SQLite` with any method, and in `migrate/sqlite` by wrapping that error as `momo.ErrUnsupported`.
- [ ] Step 4. Run `CGO_ENABLED=1 go test ./driver/sqlitedriver -run 'TestSQLiteVecLoaded|TestSQLiteVecMetrics|TestSQLiteANNIndexUnsupported' -count=1`; expect `ok`. If the `hamming` case fails, the bit-blob argument form is wrong; fix the bit encoding (`Bits.DialectValue` on SQLite), not the expectation, because the expected value 2 is the popcount of `0xB0 XOR 0x98 = 0x28`.
- [ ] Step 5. Write `TestSQLiteVectorParity` (integration, package `sqlitedriver_test`, file `parity_integration_test.go`). It stores 500 seeded vectors of dimension 8 in SQLite (through `sqlitedriver.Open`) and in PostgreSQL (through `testopen.Postgres`), then compares the top-10 ids by L2 and by cosine:

```go
//go:build integration

package sqlitedriver_test

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"slices"
	"testing"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/sqlitedriver"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

const (
	parityN   = 500
	parityDim = 8
	parityK   = 10
)

func randomVector(r *rand.Rand, n int) vector.Vector {
	v := make(vector.Vector, n)
	for i := range v {
		v[i] = float32(r.Float64()*2 - 1)
	}
	return v
}

func sqliteIDs(t *testing.T, db *sql.DB, query string, args ...any) []int64 {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestSQLiteVectorParity(t *testing.T) {
	ctx := context.Background()
	r := rand.New(rand.NewSource(3))
	vecs := make([]vector.Vector, parityN)
	for i := range vecs {
		vecs[i] = randomVector(r, parityDim)
	}
	query := randomVector(r, parityDim)
	qBlob, err := sqlite_vec.SerializeFloat32(query)
	if err != nil {
		t.Fatal(err)
	}

	// SQLite side: float32 blobs, brute-force top-k.
	sq, _, err := sqlitedriver.Open(testdb.SQLitePath(t), sqlitedriver.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sq.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := sq.Exec("CREATE TABLE vecs (id INTEGER PRIMARY KEY, emb BLOB NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for i, v := range vecs {
		blob, err := sqlite_vec.SerializeFloat32(v)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sq.Exec("INSERT INTO vecs (id, emb) VALUES (?, ?)", i+1, blob); err != nil {
			t.Fatal(err)
		}
	}
	sqL2 := sqliteIDs(t, sq, "SELECT id FROM vecs ORDER BY vec_distance_l2(emb, ?), id LIMIT ?", qBlob, parityK)
	sqCos := sqliteIDs(t, sq, "SELECT id FROM vecs ORDER BY vec_distance_cosine(emb, ?), id LIMIT ?", qBlob, parityK)

	// PostgreSQL side: exact search through WithSearch.
	db := testopen.Postgres(t)
	pool, err := pgxpool.New(ctx, testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TABLE vecs (id bigint PRIMARY KEY, emb vector(8) NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for i, v := range vecs {
		text, err := v.Value()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO vecs (id, emb) VALUES ($1, $2)", int64(i+1), text); err != nil {
			t.Fatal(err)
		}
	}
	pgIDs := func(name, op string) []int64 {
		table := "exact_" + name
		err := db.WithSearch(ctx, momo.SearchSettings{ExactSearch: true}, func(tx *momo.Tx) error {
			_, err := momo.Exec(ctx, tx, fmt.Sprintf(
				"CREATE TABLE %s AS SELECT id, row_number() OVER (ORDER BY emb %s ?, id) AS rnk FROM vecs ORDER BY emb %s ?, id LIMIT %d",
				table, op, op, parityK), query, query)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := pool.Query(ctx, "SELECT id FROM "+table+" ORDER BY rnk")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return ids
	}
	if got := pgIDs("l2", "<->"); !slices.Equal(sqL2, got) {
		t.Fatalf("L2 top-%d: sqlite %v, pgvector exact %v", parityK, sqL2, got)
	}
	if got := pgIDs("cos", "<=>"); !slices.Equal(sqCos, got) {
		t.Fatalf("cosine top-%d: sqlite %v, pgvector exact %v", parityK, sqCos, got)
	}
}
```

Expect FAIL with `undefined: Open` before Task 11 Step 3. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL=$PG go test -tags integration ./driver/sqlitedriver -run TestSQLiteVectorParity -count=1`; expect `ok`.

- [ ] Step 6. Commit: `feat(sqlitedriver): add sqlite-vec distance search with ANN rejection`.

## Task 12: Vector documentation

Covers: S-21 (vector documentation page and the index entry), S-16 (documented platforms, loading, metrics, types, limitations and pgvector differences), S-15 (documented index and filtering limits), S-17 (hybrid candidate counts and RRF k). This plan writes its own page; the site build and navigation belong to plan momo-13.

Files:

- Create `docs/vectors.md`.
- Modify `docs/README.md`: add one row for `docs/vectors.md` to the page index.
- Create `vector/doc_test.go`: `TestVectorDocPins`.

Required content of `docs/vectors.md`, in this order:

1. Platforms: linux and darwin use cgo with the C toolchain; windows uses MinGW (S-26); PostgreSQL integration runs on Linux. No prebuilt binaries.
2. Loading: `sqlitedriver.Open` registers the statically linked sqlite-vec; Momo never loads extensions at runtime.
3. Pinned versions: sqlite-vec through `github.com/asg017/sqlite-vec-go-bindings/cgo` v0.1.6; `vec_version()` returns `v0.1.6` (`SQLiteVecVersion`). The latest stable C release (v0.1.9) is not used. PostgreSQL uses pgvector 0.8.7 (`pgvector/pgvector:0.8.7-pg17` primary, `0.8.7-pg16` older major).
4. Metric and type table per dialect, generated from `dialect.Dialect.Vector`:
   - PostgreSQL: types vector, halfvec, sparsevec, bit; metrics l2, ip, cosine, l1, hamming (bit), jaccard (bit); indexes hnsw, ivfflat.
   - SQLite: types vector (float32 blob) and bit (packed blob); metrics l2, l1, cosine, hamming (bit); no ANN index. halfvec and sparsevec return `ErrUnsupported`. Inner product and Jaccard return `ErrUnsupported`.
   - MariaDB and MySQL: see `docs/mysql.md`.
5. Index and filtering limits: HNSW dimension limits (vector 2000, halfvec 4000, bit 64000); IVFFlat limits (vector 2000, halfvec 4000, bit 64000, no sparsevec); `hnsw.ef_search` range 1..1000.
6. Iterative scans and result counts: with a selective filter and `LIMIT k`, pgvector with iterative scan off can return fewer than k rows (`TestFilteredANNIterativeScan` shows this); `relaxed_order` keeps scanning until `hnsw.max_scan_tuples` (default 20000) is reached, so the result is still approximate and can be shorter than k when the filter matches fewer rows than that bound allows.
7. Differences: pgvector exact search (`ExactSearch`) versus approximate index search; sqlite-vec is brute-force KNN with no ANN index; SQLite bit vectors keep only the packed bytes, so the declared column length is authoritative for `Len`; MariaDB VECTOR INDEX is approximate.
8. Hybrid search (PostgreSQL only): full-text and vector candidate lists, `TextCandidates` and `VectorCandidates` (default 100 each, range 1..1000), fused score `1/(k + rank_text) + 1/(k + rank_vec)` with `RRFK` default 60 (k=60), ties broken by primary key ascending.
9. Value rules: NaN and Inf are rejected; halfvec values must satisfy |x| <= 65504; dimensions above 16000 are rejected.

Pages list row in `docs/README.md`: `| [Vector search] (vectors.md) | pgvector and sqlite-vec types, metrics, limits and hybrid search |`.

- [ ] Step 1. Write `vector/doc_test.go`:

```go
package vector

import (
	"os"
	"strings"
	"testing"
)

func TestVectorDocPins(t *testing.T) {
	raw, err := os.ReadFile("../docs/vectors.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"v0.1.6", "0.8.7", "vec_version", "k=60", "TextCandidates", "fewer than k rows", "hamming"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/vectors.md lacks %q", want)
		}
	}
	readme, err := os.ReadFile("../docs/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "vectors.md") {
		t.Error("docs/README.md has no row for vectors.md")
	}
}
```

Expect FAIL with `open ../docs/vectors.md: no such file or directory`.

- [ ] Step 2. Write `docs/vectors.md` with the required content above and add the README row. Run `go test ./vector -run TestVectorDocPins -count=1`; expect `ok`.
- [ ] Step 3. Commit: `docs(vector): document pgvector and sqlite-vec support, limits and pinned versions`.

## Goal

Deliver first-class vector search for Momo: validated pgvector value types (vector, halfvec, sparsevec, bit) with binary PostgreSQL encoding, typed distance expressions, HNSW and IVFFlat index declarations with matching opclasses, scoped exact and approximate search settings, EXPLAIN-based index verification, sqlite-vec scalar distance search with documented limits, and PostgreSQL hybrid full-text plus vector search.

## Architecture

The `vector` package owns the four Go value types, their validation, and per-dialect encodings chosen by `DialectValue(dialect.Name)`; the root package owns `VectorColumn[V]` and the distance expressions, which render pgvector operators on PostgreSQL and sqlite-vec functions on SQLite, gated by `dialect.Dialect.Vector` capabilities. Index declarations are `schema.Index` values (`Method`, `Parts[].OpClass`, `With`) validated by `schema.ValidateVectorIndex` and rendered by `migrate/postgres` (plan momo-04); search settings are applied with `set_config(name, value, true)` through `Tx.SetLocal` (plan momo-04 Task 23) inside a transaction, so pooled connections never keep them. Drivers own wire format: `driver/pgxdriver` sends text by default and binary through codecs installed by `pgxdriver.ConfigureVectors`, and `driver/sqlitedriver` loads the statically linked sqlite-vec.

## Constraints

- SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked via github.com/asg017/sqlite-vec-go-bindings/cgo; SQLite builds require cgo and a C toolchain. The database/sql adapter itself stays driver-agnostic.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption.
- PostgreSQL: supported versions to be recorded (tested in CI against real PostgreSQL with pgvector, image `pgvector/pgvector:pg17` locally available; plus one older major).
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- Generated execution path performs no struct-tag inspection, no reflection over models and no name-based field resolution per query.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Test packages: unit tests that call unexported helpers stay in the package under test. A test that opens a `*momo.DB` through `testopen` or through a driver uses an external package (`momo_test`, `sqlitedriver_test`, `mysqldriver_test`, `pgxdriver_test`), because `testopen` imports the drivers and the drivers import `momo`.
- Spec edge list: Vector dimension mismatch, NaN/Inf values, sparse index bounds, halfvec range.
- Failure modes (spec): Extension missing (pgvector / sqlite-vec): clear unsupported-capability error at migration or first vector query, not a silent fallback.
- Pinned images for this plan (verified 2026-11-04 against Docker Hub): `pgvector/pgvector:0.8.7-pg17` (primary) and `pgvector/pgvector:0.8.7-pg16` (older major). pgvector 0.8.7 is the latest tagged release; 0.9.0 is unreleased and drops PG13/PG14.
- Scope limits recorded here: sqlite-vec v0.1.6 has no ANN index (brute-force KNN only); any ANN index request on SQLite returns `momo.ErrUnsupported`. SQLite supports float32 vectors (L2, L1, cosine) and bit vectors (Hamming) only; halfvec and sparsevec return `momo.ErrUnsupported` on SQLite. The SQLite metric list is the set of functions registered by sqlite-vec v0.1.6 (verified in Task 2).

## Verification of this plan

- [ ] `go test ./... -count=1` (unit tests) is green on Go 1.26.
- [ ] `MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./... -count=1` is green against both pinned pgvector images, and against SQLite with sqlite-vec.
- [ ] `go test -race ./...` is green.
- [ ] `go test ./vector -run=^$ -fuzz=FuzzVectorDecode -fuzztime=60s` runs without a crash in CI nightly.
