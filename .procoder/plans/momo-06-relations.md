# momo-06-relations — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Implement Momo's relationship layer: generated `momo.Relation[P, C]` descriptors, batched eager loading (IN-list and window-function top-N strategies, pipelined and join variants), relation-derived predicates, association helpers and explicit graph writes, all without reflection on the execution path.

## Architecture

Generated code emits one `momo.Relation[P, C]` value per relation field (a struct of key extractors and getter/setter closures plus table and column names), placed in the owning model's subpackage. The runtime in package `momo` (files `relation.go`, `keys.go`, `in_tuple.go`, `load.go`, `loader.go`, `preload.go`, `pipeline.go`, `joinload.go`, `scan.go`, `relpred.go`, `assoc.go`, `graph.go`, `relhook.go`) turns those descriptors into at most one statement per bind-limit chunk per relation, groups children by normalized key strings, and assigns results without touching the parent slice's order or length.

Contract changes this plan depends on (reported to the lead; ARCHITECTURE.md is NOT edited here):

- `SelectQuery.With` is the CTE method (`select.go:192`; ARCHITECTURE.md line 64 still describes it as relation loading, which must be corrected there). Relation eager loading is exposed as `SelectQuery.Preload(...Loader[T])`, not `With`.
- `TableRef{Name, Alias, Schema}` (ARCHITECTURE.md) is used as written; the spike's `TableRef` lacks `Schema` until momo-01 adds it.
- `selectCore` gains `preloads []any` and `joinLoads []any` (set only through `Preload`).
- Test databases (ARCHITECTURE.md "Ownership decisions (round 2)"): relation integration tests run through `relfixture.Each`, which builds each database through `testopen.Each(t, fn, opts...)` (PostgreSQL first, milestone M1). The shared `relfixture.Hook` counts statements and records relation events. The dialect loop is `testopen.Each(t, fn, opts...)` (variadic options, owned by momo-03), so SQLite and MySQL join when their milestones land. Round-trip counting (pipelining) is PostgreSQL-only and uses `relfixture.PostgresDriver`.
- `momo.WithMaxParams(n int) Option` lowers the dialect bind limit for tests and for drivers with tighter limits.
- `momo.Batcher` is implemented by `pgxdriver` (momo-03) on both the driver and its transactions; the batch path in this plan depends on it.
- Plan-05 insert builder (`Insert[T, PT]` with `.Model`, `.OnConflict(cols...).DoNothing()/DoUpdate(...)`, `.Returning`) is used by `CreateGraph`, `FindOrCreate` and `Upsert`. Those tasks cannot start before momo-05's insert task is merged.

Test rule for this plan: tests that touch a database live in `package momo_test` (they import `internal/relfixture`, which imports `momo`, so internal tests would form an import cycle). Pure internal helpers are tested in `package momo` files named `*_internal_test.go`.

## Constraints

- [S-6] "Relationships: belongs-to, has-one, has-many, many-to-many, composite keys; explicit batched eager loading with bind-limit chunking, correct parent pagination, stable association, defined empty/NULL behavior, no parent duplication."
- [S-33] "Window functions (ROW_NUMBER, RANK, DENSE_RANK, aggregates OVER with PARTITION BY/ORDER BY) and an explicit second relation-loading strategy that loads the top N children per parent with a window function."
- [S-51] "Pipelined relation loading on PostgreSQL: multiple relation loads sent in one round trip via pgx batching, with measured gain."
- [S-76] "Relation-derived predicates (HasX, HasXWith, HasNoX as EXISTS), traversal from parents to a typed child query, and ordering by relation count or related field."
- [S-81] "Nested and filtered/ordered/limited eager loading, and single-query join loading for to-one relations with NULL-to-nil semantics."
- [S-82] "Polymorphic has-one/has-many, many-to-many through an explicit join model with extra columns and custom naming, self-referencing relations, and optional back-references."
- [S-83] "Explicit association helpers Link, Unlink, Replace and Count, each issuing exactly one documented statement (Replace: one transaction)."
- [S-84] "Explicit opt-in graph and convenience writes: CreateGraph with listed relations in one transaction, FindOrCreate, and Upsert by primary key."
- Out of scope: "Implicit lazy loading and automatic saving of object graphs."
- Generated execution path performs no struct-tag inspection, no reflection over models and no name-based field resolution per query.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool. Verified version for this plan: `github.com/jackc/pgx/v5 v5.12.0` (pgxpool `SendBatch` uses a pipeline; `pgx.Batch.Queue`/`BatchResults` as in v5.12.0 `batch.go`).
- SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked; SQLite: >= 3.35 (RETURNING).

## Interfaces

### Generator contract (what momo-02 must emit for each relation)

For every relation field of a model the generator emits, in the subpackage of the model that declares the field (the struct that holds the field), one package-level variable named exactly after the Go field (`document.Author`, `document.Comments`). Its type is `momo.Relation[models.P, models.C]`. Nothing else is emitted for a relation except the models' own column descriptors. The subpackage imports only the model package (never another subpackage), so relation descriptors cannot create import cycles among subpackages. Cross-model-package relations whose descriptors would close an import cycle are a generator diagnostic (momo-02).

Required literal shape (fixture: `internal/relfixture/document/momo_gen.go`, the canonical example):

```go
var Author = momo.Relation[models.Document, models.Author]{
	Name: "Author", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "authors"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Author)(nil).MomoMeta(),
	ParentCols: []string{"author_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey:    func(p *models.Document) ([]any, bool) { /* nil component -> false */ },
	ChildKey:     func(c *models.Author) ([]any, bool) { return []any{c.ID}, true },
	SetParentKey: func(p *models.Document, k []any) { /* k[i].(T) assertion per column */ },
	GetOne:       func(p *models.Document) *models.Author { return p.Author },
	SetOne:       func(p *models.Document, c *models.Author) { p.Author = c },
}
```

Rules the emitter must follow:

1. Key closures return base Go values only (`int64`, `string`, `bool`, `float64`, `time.Time`, `[]byte`). Named types are converted (`string(p.Status)`); pointer FK fields return `ok=false` when nil.
2. `ParentKey` / `ChildKey` return values in the order of `ParentCols` / `ChildCols`. `SetParentKey` / `SetChildKey` assert `k[i].(T)` with the field's base type.
3. Setters required per kind: BelongsTo: `ParentKey, ChildKey, SetParentKey, GetOne, SetOne`. HasOne: `ParentKey, ChildKey, SetChildKey, GetOne, SetOne`. HasMany: `ParentKey, ChildKey, SetChildKey, GetMany, SetMany`. ManyToMany: `ParentKey, ChildKey, GetMany, SetMany` plus `Through`. `SetChildKey` for HasX sets the child FK fields and, for a polymorphic relation, the discriminator field (`c.OwnerType = "document"`).
4. Value slices only: to-many fields are `[]C` (not `[]*C`). To-one fields are `*C`.
5. `Discriminator` is set for polymorphic relations: `&momo.Discriminator{Column: "owner_type", Value: "document"}`.
6. `Through` for ManyToMany: `&momo.Through{Table: momo.TableRef{Name: "taggings"}, ParentCols: []string{"document_id"}, ChildCols: []string{"tag_id"}}`. Explicit join models (with extra columns) are modelled as a HasMany to the join model plus a BelongsTo from the join model (see fixture `tagging.Tag` and `document.Taggings`), not as `Through`.
7. Self-referencing relations use the same literal; back-references are separate descriptors on the other model.
8. Column descriptors (`momo.NewColumn[T]`, `momo.NewNullColumn[T]`) for the same subpackage are emitted alongside relations.

### Runtime API (package `momo`, files listed in Architecture)

```go
type RelationKind uint8
const (BelongsTo RelationKind = iota + 1; HasOne; HasMany; ManyToMany)

type Through struct { Table TableRef; ParentCols, ChildCols []string }
type Discriminator struct { Column string; Value any }

type Relation[P, C any] struct {
	Name          string
	Kind          RelationKind
	Parent        TableRef
	Child         TableRef
	ParentMeta    *Meta[P]
	ChildMeta     *Meta[C]
	ParentCols    []string
	ChildCols     []string
	ChildPK       []string
	Through       *Through
	Discriminator *Discriminator
	ParentKey     func(p *P) ([]any, bool)
	ChildKey      func(c *C) ([]any, bool)
	SetParentKey  func(p *P, key []any)
	SetChildKey   func(c *C, key []any)
	GetOne        func(p *P) *C
	SetOne        func(p *P, c *C)
	GetMany       func(p *P) []C
	SetMany       func(p *P, cs []C)
}

type LoadOption[C any] func(*loadConfig)
func LoadWhere[C any](ps ...Predicate) LoadOption[C]
func LoadOrderBy[C any](items ...Orderable) LoadOption[C]
func LoadPerParentLimit[C any](n int) LoadOption[C]

func Load[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) error
func LoadAll[P any](ctx context.Context, q Querier, parents []P, loaders ...Loader[P]) error

type Loader[P any] interface { relationName() string; prepare(q Querier, parents []P) (pendingLoad, error) }
func (r Relation[P, C]) Scope(opts ...LoadOption[C]) Loader[P]
func (r Relation[P, C]) Then(nested ...Loader[C]) Loader[P]
func (r Relation[P, C]) Joined() Loader[P]                       // BelongsTo only
func (r Relation[P, C]) Has(scope ...Predicate) Predicate
func (r Relation[P, C]) HasFrom(parentAlias string, scope ...Predicate) Predicate
func (r Relation[P, C]) HasWith(scope ...Predicate) Predicate   // alias of Has
func (r Relation[P, C]) HasNo(scope ...Predicate) Predicate
func (r Relation[P, C]) From(parents SelectQuery[P]) SelectQuery[C]
func (r Relation[P, C]) CountOrder(dir Direction) Orderable     // dir: DirAsc or DirDesc
func (r Relation[P, C]) Link(ctx context.Context, q Querier, p *P, cs ...*C) error
func (r Relation[P, C]) Unlink(ctx context.Context, q Querier, p *P, cs ...*C) error
func (r Relation[P, C]) Replace(ctx context.Context, q Querier, p *P, cs ...*C) error
func (r Relation[P, C]) Count(ctx context.Context, q Querier, p *P) (int64, error)

type Association[P any] interface{ before(ctx context.Context, tx *Tx, p *P) error; after(ctx context.Context, tx *Tx, p *P) error }
func (r Relation[P, C]) Include() Association[P]
func CreateGraph[P any, PP Modeler[P]](ctx context.Context, db *DB, p *P, includes ...Association[P]) error
func FindOrCreate[T any, PT Modeler[T]](ctx context.Context, q Querier, key []ColumnRef, m *T) (created bool, err error)
func Upsert[T any, PT Modeler[T]](ctx context.Context, q Querier, m *T) error

func (sq SelectQuery[T]) Preload(loaders ...Loader[T]) SelectQuery[T]
func ScanValue(dst any, src any) error

type RelationEvent struct { Parent, Name string; Kind RelationKind; Parents int; Children int64; Queries int; Duration time.Duration; Err error }
type RelationHook interface { BeforeRelation(ctx context.Context, e *RelationEvent) context.Context; AfterRelation(ctx context.Context, e *RelationEvent) }

func WithMaxParams(n int) Option
```

Error sentinels used (from `errors.go`): `ErrTooManyRows` (HasOne with two children), `ErrValidation` (invalid descriptor, NULL scanned into non-nullable field), `ErrUnsupported` (TopN on BelongsTo, Unlink on BelongsTo, joined loading in Each/Iter, `ON CONFLICT` on a dialect without it).

Query-event convention: a relation load's child statements carry `QueryEvent.Relation == "<parent table>.<relation name>"` (for example `documents.Author`), and `QueryEvent.Op == "select"`.

---

## Task 1: Normalized key encoding

Files:

- Create `keys.go`
- Create `keys_internal_test.go`

Interfaces:

- `func keyString(vals []any) string` (package `momo`, internal). Produces a comparable map key; a NULL component is never passed here (callers check `ok` first).

Covers: S-6 ("no parent duplication" depends on key grouping). Acceptance test named here: `TestKeyStringNormalization` (internal).

- [ ] **Step 1: Write the failing test** — create `keys_internal_test.go`:

```go
package momo

import (
	"database/sql/driver"
	"testing"
	"time"
)

type testUUIDValuer string

func (u testUUIDValuer) Value() (driver.Value, error) { return string(u), nil }

func TestKeyStringNormalization(t *testing.T) {
	same := func(a, b []any) {
		t.Helper()
		if keyString(a) != keyString(b) {
			t.Fatalf("keys differ: %q vs %q", keyString(a), keyString(b))
		}
	}
	diff := func(a, b []any) {
		t.Helper()
		if keyString(a) == keyString(b) {
			t.Fatalf("keys collide: %q", keyString(a))
		}
	}
	same([]any{int64(7)}, []any{7})                    // int vs int64 from driver
	same([]any{int32(7)}, []any{int64(7)})             // int32 scanned from a smaller column
	same([]any{[]byte("ab")}, []any{"ab"})             // bytea/text from driver
	same([]any{testUUIDValuer("u-1")}, []any{"u-1"})   // driver.Valuer is normalized through Value()
	diff([]any{"a", "b"}, []any{"ab", ""})             // composite components are delimited
	diff([]any{int64(1)}, []any{"1"})                  // type tag keeps 1 and "1" apart
	diff([]any{nil}, []any{""})                        // NULL is distinct from empty string
	same([]any{time.Date(2020, 1, 1, 1, 0, 0, 0, time.FixedZone("x", 3600))}, []any{time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
}
```

- [ ] **Step 2: Run it and expect FAIL** — `go test ./ -run '^TestKeyStringNormalization$' -count=1` must fail with `undefined: keyString`.

- [ ] **Step 3: Implement** — create `keys.go`:

```go
package momo

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// keyString encodes a relation key tuple as a comparable string. Each
// component is type-tagged so 1 and "1" differ, and components are separated
// by 0x1f so ("a","b") and ("ab","") differ. Values are normalized so that a
// key built from a struct field (int, named string) matches the same key read
// back from the driver (int64, []byte, driver.Valuer output).
func keyString(vals []any) string {
	var sb strings.Builder
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(0x1f)
		}
		writeKeyPart(&sb, v)
	}
	return sb.String()
}

func writeKeyPart(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("n:")
	case string:
		sb.WriteString("s:")
		sb.WriteString(x)
	case []byte:
		sb.WriteString("s:")
		sb.Write(x)
	case int:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int8:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int16:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int32:
		sb.WriteString("i:" + strconv.FormatInt(int64(x), 10))
	case int64:
		sb.WriteString("i:" + strconv.FormatInt(x, 10))
	case uint:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint8:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint16:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint32:
		sb.WriteString("u:" + strconv.FormatUint(uint64(x), 10))
	case uint64:
		sb.WriteString("u:" + strconv.FormatUint(x, 10))
	case bool:
		sb.WriteString("b:" + strconv.FormatBool(x))
	case float32:
		sb.WriteString("f:" + strconv.FormatFloat(float64(x), 'g', -1, 64))
	case float64:
		sb.WriteString("f:" + strconv.FormatFloat(x, 'g', -1, 64))
	case time.Time:
		sb.WriteString("t:" + x.UTC().Format(time.RFC3339Nano))
	case driver.Valuer:
		vv, err := x.Value()
		if err != nil {
			sb.WriteString("e:" + err.Error())
			return
		}
		writeKeyPart(sb, vv)
	default:
		fmt.Fprintf(sb, "x:%T:%v", x, x)
	}
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestKeyStringNormalization$' -count=1` prints `ok  	github.com/azrtydxb/momo`.

- [ ] **Step 5: Commit** — `git add keys.go keys_internal_test.go && git commit -m "feat(relations): normalize relation key tuples"`.

---

Reconciled 2026-11-05 (story 138 review): the shipped `keyString` also reduces named types to their kind (via reflect), encodes an unsigned value that fits int64 with the signed tag, and length-prefixes strings and bytes (`s<len>:`), so named key types match driver values and a key containing 0x1f cannot collide with a composite key. `TestKeyStringNormalization` carries the extra cases.

## Task 2: Relation descriptor type and validation

Files:

- Create `relation.go` (descriptor types, `Through`, `Discriminator`, `RelationKind`, `validate`)
- Create `relation_internal_test.go`

Interfaces:

- `type RelationKind uint8` with constants `BelongsTo`, `HasOne`, `HasMany`, `ManyToMany`; `func (k RelationKind) String() string`.
- `type Through struct{ Table TableRef; ParentCols, ChildCols []string }`; `type Discriminator struct{ Column string; Value any }`.
- `type Relation[P, C any] struct{...}` exactly as in the Interfaces section.
- `func (r *Relation[P, C]) validate() error` returns `*Error{Kind: ErrValidation, Op: "relation", Table: r.Child.Name, Err: ...}`.

Covers: S-6 (descriptor shape; composite arity), S-82 (Through rule, polymorphic rule). Acceptance test named here: `TestRelationValidate` (internal, table-driven).

- [ ] **Step 1: Write the failing test** — create `relation_internal_test.go`:

```go
package momo

import (
	"errors"
	"testing"
)

func validRel() Relation[int, int] {
	return Relation[int, int]{
		Name: "Child", Kind: HasMany,
		Parent: TableRef{Name: "p"}, Child: TableRef{Name: "c"},
		ParentCols: []string{"id"}, ChildCols: []string{"pid"}, ChildPK: []string{"id"},
		ParentKey:  func(p *int) ([]any, bool) { return []any{*p}, true },
		ChildKey:   func(c *int) ([]any, bool) { return []any{*c}, true },
		SetChildKey: func(c *int, k []any) {},
		GetMany:    func(p *int) []int { return nil },
		SetMany:    func(p *int, cs []int) {},
	}
}

func TestRelationValidate(t *testing.T) {
	cases := map[string]struct {
		mut  func(r *Relation[int, int])
		want bool // true: valid
	}{
		"valid has-many":          {func(r *Relation[int, int]) {}, true},
		"no name":                 {func(r *Relation[int, int]) { r.Name = "" }, false},
		"arity mismatch":          {func(r *Relation[int, int]) { r.ChildCols = []string{"a", "b"} }, false},
		"missing ParentKey":       {func(r *Relation[int, int]) { r.ParentKey = nil }, false},
		"has-many without GetMany": {func(r *Relation[int, int]) { r.GetMany = nil }, false},
		"to-one needs SetOne":     {func(r *Relation[int, int]) { r.Kind = BelongsTo; r.GetOne = func(*int) *int { return nil } }, false},
		"through on has-many":     {func(r *Relation[int, int]) { r.Through = &Through{ParentCols: []string{"x"}, ChildCols: []string{"y"}} }, false},
		"many-to-many needs through": {func(r *Relation[int, int]) { r.Kind = ManyToMany }, false},
		"discriminator on to-one": {func(r *Relation[int, int]) { r.Kind = BelongsTo; r.GetOne = func(*int) *int { return nil }; r.SetOne = func(*int, *int) {}; r.Discriminator = &Discriminator{Column: "t", Value: "x"} }, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := validRel()
			tc.mut(&r)
			err := r.validate()
			if tc.want && err != nil {
				t.Fatalf("want valid, got %v", err)
			}
			if !tc.want && !errors.Is(err, ErrValidation) {
				t.Fatalf("want ErrValidation, got %v", err)
			}
		})
	}
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestRelationValidate$' -count=1` fails with `undefined: Relation`.

- [ ] **Step 3: Implement** — create `relation.go`:

```go
package momo

import (
	"errors"
	"fmt"
)

// RelationKind is the shape of a relation.
type RelationKind uint8

const (
	BelongsTo  RelationKind = iota + 1 // foreign key on the parent; child matched by its key
	HasOne                             // foreign key on the child; at most one child per parent
	HasMany                            // foreign key on the child; any number per parent
	ManyToMany                         // join table between parent and child
)

func (k RelationKind) String() string {
	switch k {
	case BelongsTo:
		return "belongs-to"
	case HasOne:
		return "has-one"
	case HasMany:
		return "has-many"
	case ManyToMany:
		return "many-to-many"
	}
	return fmt.Sprintf("RelationKind(%d)", uint8(k))
}

// Through names the join table of a ManyToMany relation.
type Through struct {
	Table      TableRef
	ParentCols []string // join columns referencing Relation.ParentCols, same order
	ChildCols  []string // join columns referencing Relation.ChildCols, same order
}

// Discriminator restricts a polymorphic has-one/has-many to rows whose type
// column equals Value.
type Discriminator struct {
	Column string
	Value  any
}

// Relation describes relation Name from parent P to child C. Generated code
// emits one value per relation field; see the Generator contract in the plan.
// Relation values are immutable and safe for concurrent use.
type Relation[P, C any] struct {
	Name          string
	Kind          RelationKind
	Parent        TableRef
	Child         TableRef
	ParentMeta    *Meta[P]
	ChildMeta     *Meta[C]
	ParentCols    []string
	ChildCols     []string
	ChildPK       []string
	Through       *Through
	Discriminator *Discriminator
	ParentKey     func(p *P) ([]any, bool)
	ChildKey      func(c *C) ([]any, bool)
	SetParentKey  func(p *P, key []any)
	SetChildKey   func(c *C, key []any)
	GetOne        func(p *P) *C
	SetOne        func(p *P, c *C)
	GetMany       func(p *P) []C
	SetMany       func(p *P, cs []C)
}

func (r *Relation[P, C]) invalid(msg string) error {
	return &Error{Kind: ErrValidation, Op: "relation", Table: r.Child.Name, Err: errors.New(r.Name + ": " + msg)}
}

// validate checks the descriptor's shape. Every loader calls it before SQL is built.
func (r *Relation[P, C]) validate() error {
	switch {
	case r.Name == "":
		return r.invalid("relation has no name")
	case len(r.ParentCols) == 0:
		return r.invalid("no parent columns")
	case r.ParentKey == nil || r.ChildKey == nil:
		return r.invalid("ParentKey and ChildKey are required")
	case r.Kind == ManyToMany && r.Through == nil:
		return r.invalid("many-to-many requires Through")
	case r.Kind != ManyToMany && r.Through != nil:
		return r.invalid("Through is only valid for many-to-many")
	case r.Discriminator != nil && (r.Kind == BelongsTo || r.Kind == ManyToMany):
		return r.invalid("Discriminator is only valid for has-one and has-many")
	case r.Through != nil && (len(r.Through.ParentCols) != len(r.ParentCols) || len(r.Through.ChildCols) != len(r.ChildCols)):
		return r.invalid("Through column arity must match the relation key arity")
	case r.Through == nil && len(r.ChildCols) != len(r.ParentCols):
		return r.invalid("parent and child key arity differ")
	}
	switch r.Kind {
	case BelongsTo, HasOne:
		if r.GetOne == nil || r.SetOne == nil {
			return r.invalid("to-one relations require GetOne and SetOne")
		}
	case HasMany, ManyToMany:
		if r.GetMany == nil || r.SetMany == nil {
			return r.invalid("to-many relations require GetMany and SetMany")
		}
	default:
		return r.invalid("unknown relation kind")
	}
	return nil
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestRelationValidate$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add relation.go relation_internal_test.go && git commit -m "feat(relations): add Relation descriptor and validation"`.

---

## Task 3: Relation fixture models, schema and seed data

Files:

- Create `internal/relfixture/models/model.go`
- Create `internal/relfixture/models/momo_gen.go`
- Create `internal/relfixture/author/momo_gen.go`
- Create `internal/relfixture/document/momo_gen.go`
- Create `internal/relfixture/comment/momo_gen.go`
- Create `internal/relfixture/tagging/momo_gen.go`
- Create `internal/relfixture/employee/momo_gen.go`
- Create `internal/relfixture/region/momo_gen.go`
- Create `internal/relfixture/store/momo_gen.go`
- Create `internal/relfixture/schema.go` (DDL, seed, `Hook`, `Each`, `PostgresDriver`)
- Create `internal/relfixture/relfixture_test.go` (package `relfixture_test`)

Interfaces:

- `models.Author{ID int64; Name string; Images []Image}`, `models.Document{ID int64; AuthorID *int64; Title string; Author *Author; Comments []Comment; Tags []Tag; Taggings []Tagging; Images []Image}`, `models.Comment{ID, DocumentID int64; Body string; Approved bool}`, `models.Tag{ID int64; Name string}`, `models.Tagging{DocumentID, TagID, Position int64; Tag *Tag}`, `models.Image{ID int64; OwnerType string; OwnerID int64; URL string}`, `models.Employee{ID int64; ManagerID *int64; Name string; Manager *Employee; Reports []Employee}`, `models.Region{TenantID int64; Code, Name string; Stores []Store}`, `models.Store{ID, TenantID int64; RegionCode *string; Name string; Region *Region}`.
- Every model's `MomoMeta()` is `func (*T) MomoMeta() *momo.Meta[T]`, returning a package-level pointer.
- Relation variables (exact names): `author.Images`; `document.Author`, `document.Comments`, `document.Tags`, `document.Taggings`, `document.Images`; `tagging.Tag`; `employee.Manager`, `employee.Reports`; `region.Stores`; `store.Region`.
- Column variables: `author.ID`, `author.Name`; `document.ID`, `document.AuthorID` (NullColumn), `document.Title`; `comment.ID`, `comment.DocumentID`, `comment.Body`, `comment.Approved`; `tagging.DocumentID`, `tagging.TagID`, `tagging.Position`; `employee.ID`, `employee.ManagerID`, `employee.Name`; `region.TenantID`, `region.Code`, `region.Name`; `store.ID`, `store.TenantID`, `store.RegionCode`, `store.Name`.
- `type Hook struct{ After []string; Events []momo.RelationEvent }` (counting and relation-event hook); `func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, h *Hook))` (PostgreSQL subtest via `testopen.Postgres`, schema applied); `func PostgresDriver(t testing.TB) momo.Driver`; `func CreateSchema(t testing.TB, db *momo.DB)`; `func SeedBasic(t testing.TB, db *momo.DB)`.

Covers: S-6, S-76, S-82 (fixture for all kinds); the descriptors are the canonical generator output (Interfaces). Acceptance test named here: `TestRelfixtureSchemaApplies` (external, `relfixture_test.go`).

- [ ] **Step 1: Write the failing test** — create `internal/relfixture/relfixture_test.go`:

```go
package relfixture_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/store"
)

func TestRelfixtureSchemaApplies(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		n, err := momo.Select[models.Document, *models.Document](db).Count(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 4 {
			t.Fatalf("documents: got %d want 4", n)
		}
		if document.Author.Kind != momo.BelongsTo || document.Comments.Kind != momo.HasMany || document.Tags.Kind != momo.ManyToMany {
			t.Fatal("fixture relation kinds drifted from the generator contract")
		}
		if store.Region.ChildMeta == nil || len(store.Region.ParentCols) != 2 {
			t.Fatal("composite fixture relation is not composite")
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./internal/relfixture/ -run '^TestRelfixtureSchemaApplies$' -count=1` fails with `no required module provides package github.com/azrtydxb/momo/internal/relfixture` or `package ... is not in std`.

- [ ] **Step 3: Implement the models** — create `internal/relfixture/models/model.go`:

```go
// Package models holds the relation fixture's model structs. In a real
// project these are the user-written types; momo-02's generator emits the
// MomoMeta methods in momo_gen.go and the subpackages.
package models

import "github.com/azrtydxb/momo"

type Author struct {
	momo.Model `momo:"table:authors"`
	ID         int64 `momo:"pk"`
	Name       string
	Images     []Image
	FirstDocument *Document
}

type Document struct {
	momo.Model `momo:"table:documents"`
	ID         int64 `momo:"pk"`
	AuthorID   *int64
	Title      string
	Author     *Author
	Comments   []Comment
	Tags       []Tag
	Taggings   []Tagging
	Images     []Image
}

type Comment struct {
	momo.Model `momo:"table:comments"`
	ID         int64 `momo:"pk"`
	DocumentID int64
	Body       string
	Approved   bool
}

type Tag struct {
	momo.Model `momo:"table:tags"`
	ID         int64 `momo:"pk"`
	Name       string
}

type Tagging struct {
	momo.Model `momo:"table:taggings"`
	DocumentID int64 `momo:"pk"`
	TagID      int64 `momo:"pk"`
	Position   int64
	Tag        *Tag
}

type Image struct {
	momo.Model `momo:"table:images"`
	ID         int64 `momo:"pk"`
	OwnerType  string
	OwnerID    int64
	URL        string
}

type Employee struct {
	momo.Model `momo:"table:employees"`
	ID         int64 `momo:"pk"`
	ManagerID  *int64
	Name       string
	Manager    *Employee
	Reports    []Employee
}

type Region struct {
	momo.Model `momo:"table:regions"`
	TenantID   int64  `momo:"pk"`
	Code       string `momo:"pk"`
	Name       string
	Stores     []Store
}

type Store struct {
	momo.Model `momo:"table:stores"`
	ID         int64 `momo:"pk"`
	TenantID   int64
	RegionCode *string
	Name       string
	Region     *Region
}
```

- [ ] **Step 4: Implement the MomoMeta methods** — create `internal/relfixture/models/momo_gen.go`. Each `Ptr` returns the field address for column index `i` in `Columns` order; `Value` returns the field value; relation fields are never columns:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package models

import "github.com/azrtydxb/momo"

var authorMeta = &momo.Meta[Author]{
	Table:   "authors",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr: func(m *Author, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.Name
		}
		panic("models: column index out of range")
	},
	Value: func(m *Author, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.Name
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Author) MomoMeta() *momo.Meta[Author] { return authorMeta }

var documentMeta = &momo.Meta[Document]{
	Table:   "documents",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "author_id", Flags: momo.FlagNullable}, {Name: "title"}},
	Ptr: func(m *Document, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.AuthorID
		case 2:
			return &m.Title
		}
		panic("models: column index out of range")
	},
	Value: func(m *Document, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.AuthorID
		case 2:
			return m.Title
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Document) MomoMeta() *momo.Meta[Document] { return documentMeta }

var commentMeta = &momo.Meta[Comment]{
	Table:   "comments",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "document_id"}, {Name: "body"}, {Name: "approved"}},
	Ptr: func(m *Comment, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.DocumentID
		case 2:
			return &m.Body
		case 3:
			return &m.Approved
		}
		panic("models: column index out of range")
	},
	Value: func(m *Comment, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.DocumentID
		case 2:
			return m.Body
		case 3:
			return m.Approved
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Comment) MomoMeta() *momo.Meta[Comment] { return commentMeta }

var tagMeta = &momo.Meta[Tag]{
	Table:   "tags",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr: func(m *Tag, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.Name
		}
		panic("models: column index out of range")
	},
	Value: func(m *Tag, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.Name
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Tag) MomoMeta() *momo.Meta[Tag] { return tagMeta }

var taggingMeta = &momo.Meta[Tagging]{
	Table:   "taggings",
	Columns: []momo.ColumnMeta{{Name: "document_id", Flags: momo.FlagPK}, {Name: "tag_id", Flags: momo.FlagPK}, {Name: "position"}},
	Ptr: func(m *Tagging, i int) any {
		switch i {
		case 0:
			return &m.DocumentID
		case 1:
			return &m.TagID
		case 2:
			return &m.Position
		}
		panic("models: column index out of range")
	},
	Value: func(m *Tagging, i int) any {
		switch i {
		case 0:
			return m.DocumentID
		case 1:
			return m.TagID
		case 2:
			return m.Position
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0, 1},
}

func (*Tagging) MomoMeta() *momo.Meta[Tagging] { return taggingMeta }

var imageMeta = &momo.Meta[Image]{
	Table:   "images",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "owner_type"}, {Name: "owner_id"}, {Name: "url"}},
	Ptr: func(m *Image, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.OwnerType
		case 2:
			return &m.OwnerID
		case 3:
			return &m.URL
		}
		panic("models: column index out of range")
	},
	Value: func(m *Image, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.OwnerType
		case 2:
			return m.OwnerID
		case 3:
			return m.URL
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Image) MomoMeta() *momo.Meta[Image] { return imageMeta }

var employeeMeta = &momo.Meta[Employee]{
	Table:   "employees",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "manager_id", Flags: momo.FlagNullable}, {Name: "name"}},
	Ptr: func(m *Employee, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.ManagerID
		case 2:
			return &m.Name
		}
		panic("models: column index out of range")
	},
	Value: func(m *Employee, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.ManagerID
		case 2:
			return m.Name
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Employee) MomoMeta() *momo.Meta[Employee] { return employeeMeta }

var regionMeta = &momo.Meta[Region]{
	Table:   "regions",
	Columns: []momo.ColumnMeta{{Name: "tenant_id", Flags: momo.FlagPK}, {Name: "code", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr: func(m *Region, i int) any {
		switch i {
		case 0:
			return &m.TenantID
		case 1:
			return &m.Code
		case 2:
			return &m.Name
		}
		panic("models: column index out of range")
	},
	Value: func(m *Region, i int) any {
		switch i {
		case 0:
			return m.TenantID
		case 1:
			return m.Code
		case 2:
			return m.Name
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0, 1},
}

func (*Region) MomoMeta() *momo.Meta[Region] { return regionMeta }

var storeMeta = &momo.Meta[Store]{
	Table:   "stores",
	Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK}, {Name: "tenant_id"}, {Name: "region_code", Flags: momo.FlagNullable}, {Name: "name"}},
	Ptr: func(m *Store, i int) any {
		switch i {
		case 0:
			return &m.ID
		case 1:
			return &m.TenantID
		case 2:
			return &m.RegionCode
		case 3:
			return &m.Name
		}
		panic("models: column index out of range")
	},
	Value: func(m *Store, i int) any {
		switch i {
		case 0:
			return m.ID
		case 1:
			return m.TenantID
		case 2:
			return m.RegionCode
		case 3:
			return m.Name
		}
		panic("models: column index out of range")
	},
	Identity: -1, Version: -1, SoftDelete: -1, PK: []int{0},
}

func (*Store) MomoMeta() *momo.Meta[Store] { return storeMeta }
```

- [ ] **Step 5: Implement the subpackages with the canonical relation descriptors** — create `internal/relfixture/document/momo_gen.go`. This file is the canonical generator output that momo-02 must reproduce:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package document

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	ID       = momo.NewColumn[int64]("documents", "id", momo.FlagPK)
	AuthorID = momo.NewNullColumn[int64]("documents", "author_id", momo.FlagNullable)
	Title    = momo.NewColumn[string]("documents", "title", 0)
)

var Author = momo.Relation[models.Document, models.Author]{
	Name: "Author", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "authors"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Author)(nil).MomoMeta(),
	ParentCols: []string{"author_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Document) ([]any, bool) {
		if p.AuthorID == nil {
			return nil, false
		}
		return []any{*p.AuthorID}, true
	},
	ChildKey: func(c *models.Author) ([]any, bool) { return []any{c.ID}, true },
	SetParentKey: func(p *models.Document, k []any) {
		v := k[0].(int64)
		p.AuthorID = &v
	},
	GetOne: func(p *models.Document) *models.Author { return p.Author },
	SetOne: func(p *models.Document, c *models.Author) { p.Author = c },
}

var Comments = momo.Relation[models.Document, models.Comment]{
	Name: "Comments", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "comments"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Comment)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"document_id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Document) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *models.Comment) ([]any, bool) { return []any{c.DocumentID}, true },
	SetChildKey: func(c *models.Comment, k []any) {
		c.DocumentID = k[0].(int64)
	},
	GetMany: func(p *models.Document) []models.Comment { return p.Comments },
	SetMany: func(p *models.Document, cs []models.Comment) { p.Comments = cs },
}

var Tags = momo.Relation[models.Document, models.Tag]{
	Name: "Tags", Kind: momo.ManyToMany,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "tags"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Tag)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	Through: &momo.Through{
		Table:      momo.TableRef{Name: "taggings"},
		ParentCols: []string{"document_id"},
		ChildCols:  []string{"tag_id"},
	},
	ParentKey: func(p *models.Document) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *models.Tag) ([]any, bool) { return []any{c.ID}, true },
	GetMany:   func(p *models.Document) []models.Tag { return p.Tags },
	SetMany:   func(p *models.Document, cs []models.Tag) { p.Tags = cs },
}

// Taggings is the explicit join model: a has-many to the join table so that
// join-row attributes (Position) are reachable, then tagging.Tag for the tag.
var Taggings = momo.Relation[models.Document, models.Tagging]{
	Name: "Taggings", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "taggings"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Tagging)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"document_id"}, ChildPK: []string{"document_id", "tag_id"},
	ParentKey: func(p *models.Document) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *models.Tagging) ([]any, bool) { return []any{c.DocumentID}, true },
	SetChildKey: func(c *models.Tagging, k []any) {
		c.DocumentID = k[0].(int64)
	},
	GetMany: func(p *models.Document) []models.Tagging { return p.Taggings },
	SetMany: func(p *models.Document, cs []models.Tagging) { p.Taggings = cs },
}

// Images is polymorphic: images.owner_type = 'document' AND owner_id = documents.id.
var Images = momo.Relation[models.Document, models.Image]{
	Name: "Images", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "documents"}, Child: momo.TableRef{Name: "images"},
	ParentMeta: (*models.Document)(nil).MomoMeta(), ChildMeta: (*models.Image)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"owner_id"}, ChildPK: []string{"id"},
	Discriminator: &momo.Discriminator{Column: "owner_type", Value: "document"},
	ParentKey:     func(p *models.Document) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:      func(c *models.Image) ([]any, bool) { return []any{c.OwnerID}, true },
	SetChildKey: func(c *models.Image, k []any) {
		c.OwnerID = k[0].(int64)
		c.OwnerType = "document"
	},
	GetMany: func(p *models.Document) []models.Image { return p.Images },
	SetMany: func(p *models.Document, cs []models.Image) { p.Images = cs },
}
```

Create `internal/relfixture/tagging/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package tagging

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	DocumentID = momo.NewColumn[int64]("taggings", "document_id", momo.FlagPK)
	TagID      = momo.NewColumn[int64]("taggings", "tag_id", momo.FlagPK)
	Position   = momo.NewColumn[int64]("taggings", "position", 0)
)

var Tag = momo.Relation[models.Tagging, models.Tag]{
	Name: "Tag", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "taggings"}, Child: momo.TableRef{Name: "tags"},
	ParentMeta: (*models.Tagging)(nil).MomoMeta(), ChildMeta: (*models.Tag)(nil).MomoMeta(),
	ParentCols: []string{"tag_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Tagging) ([]any, bool) { return []any{p.TagID}, true },
	ChildKey:  func(c *models.Tag) ([]any, bool) { return []any{c.ID}, true },
	SetParentKey: func(p *models.Tagging, k []any) {
		p.TagID = k[0].(int64)
	},
	GetOne: func(p *models.Tagging) *models.Tag { return p.Tag },
	SetOne: func(p *models.Tagging, c *models.Tag) { p.Tag = c },
}
```

Create `internal/relfixture/author/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package author

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	ID   = momo.NewColumn[int64]("authors", "id", momo.FlagPK)
	Name = momo.NewColumn[string]("authors", "name", 0)
)

var Images = momo.Relation[models.Author, models.Image]{
	Name: "Images", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "authors"}, Child: momo.TableRef{Name: "images"},
	ParentMeta: (*models.Author)(nil).MomoMeta(), ChildMeta: (*models.Image)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"owner_id"}, ChildPK: []string{"id"},
	Discriminator: &momo.Discriminator{Column: "owner_type", Value: "author"},
	ParentKey:     func(p *models.Author) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:      func(c *models.Image) ([]any, bool) { return []any{c.OwnerID}, true },
	SetChildKey: func(c *models.Image, k []any) {
		c.OwnerID = k[0].(int64)
		c.OwnerType = "author"
	},
	GetMany: func(p *models.Author) []models.Image { return p.Images },
	SetMany: func(p *models.Author, cs []models.Image) { p.Images = cs },
}
```

Create `internal/relfixture/comment/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package comment

import "github.com/azrtydxb/momo"

var (
	ID         = momo.NewColumn[int64]("comments", "id", momo.FlagPK)
	DocumentID = momo.NewColumn[int64]("comments", "document_id", 0)
	Body       = momo.NewColumn[string]("comments", "body", 0)
	Approved   = momo.NewColumn[bool]("comments", "approved", 0)
)
```

Create `internal/relfixture/employee/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package employee

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	ID        = momo.NewColumn[int64]("employees", "id", momo.FlagPK)
	ManagerID = momo.NewNullColumn[int64]("employees", "manager_id", momo.FlagNullable)
	Name      = momo.NewColumn[string]("employees", "name", 0)
)

// Manager is a self-referencing belongs-to.
var Manager = momo.Relation[models.Employee, models.Employee]{
	Name: "Manager", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "employees"}, Child: momo.TableRef{Name: "employees"},
	ParentMeta: (*models.Employee)(nil).MomoMeta(), ChildMeta: (*models.Employee)(nil).MomoMeta(),
	ParentCols: []string{"manager_id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Employee) ([]any, bool) {
		if p.ManagerID == nil {
			return nil, false
		}
		return []any{*p.ManagerID}, true
	},
	ChildKey: func(c *models.Employee) ([]any, bool) { return []any{c.ID}, true },
	SetParentKey: func(p *models.Employee, k []any) {
		v := k[0].(int64)
		p.ManagerID = &v
	},
	GetOne: func(p *models.Employee) *models.Employee { return p.Manager },
	SetOne: func(p *models.Employee, c *models.Employee) { p.Manager = c },
}

// Reports is the back-reference of Manager (has-many over the same columns).
var Reports = momo.Relation[models.Employee, models.Employee]{
	Name: "Reports", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "employees"}, Child: momo.TableRef{Name: "employees"},
	ParentMeta: (*models.Employee)(nil).MomoMeta(), ChildMeta: (*models.Employee)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"manager_id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Employee) ([]any, bool) { return []any{p.ID}, true },
	ChildKey: func(c *models.Employee) ([]any, bool) {
		if c.ManagerID == nil {
			return nil, false
		}
		return []any{*c.ManagerID}, true
	},
	SetChildKey: func(c *models.Employee, k []any) {
		v := k[0].(int64)
		c.ManagerID = &v
	},
	GetMany: func(p *models.Employee) []models.Employee { return p.Reports },
	SetMany: func(p *models.Employee, cs []models.Employee) { p.Reports = cs },
}
```

Create `internal/relfixture/region/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package region

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	TenantID = momo.NewColumn[int64]("regions", "tenant_id", momo.FlagPK)
	Code     = momo.NewColumn[string]("regions", "code", momo.FlagPK)
	Name     = momo.NewColumn[string]("regions", "name", 0)
)

var Stores = momo.Relation[models.Region, models.Store]{
	Name: "Stores", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "regions"}, Child: momo.TableRef{Name: "stores"},
	ParentMeta: (*models.Region)(nil).MomoMeta(), ChildMeta: (*models.Store)(nil).MomoMeta(),
	ParentCols: []string{"tenant_id", "code"}, ChildCols: []string{"tenant_id", "region_code"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Region) ([]any, bool) { return []any{p.TenantID, p.Code}, true },
	ChildKey: func(c *models.Store) ([]any, bool) {
		if c.RegionCode == nil {
			return nil, false
		}
		return []any{c.TenantID, *c.RegionCode}, true
	},
	SetChildKey: func(c *models.Store, k []any) {
		c.TenantID = k[0].(int64)
		s := k[1].(string)
		c.RegionCode = &s
	},
	GetMany: func(p *models.Region) []models.Store { return p.Stores },
	SetMany: func(p *models.Region, cs []models.Store) { p.Stores = cs },
}
```

Create `internal/relfixture/store/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package store

import (
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

var (
	ID         = momo.NewColumn[int64]("stores", "id", momo.FlagPK)
	TenantID   = momo.NewColumn[int64]("stores", "tenant_id", 0)
	RegionCode = momo.NewNullColumn[string]("stores", "region_code", momo.FlagNullable)
	Name       = momo.NewColumn[string]("stores", "name", 0)
)

// Region is a composite belongs-to.
var Region = momo.Relation[models.Store, models.Region]{
	Name: "Region", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "stores"}, Child: momo.TableRef{Name: "regions"},
	ParentMeta: (*models.Store)(nil).MomoMeta(), ChildMeta: (*models.Region)(nil).MomoMeta(),
	ParentCols: []string{"tenant_id", "region_code"}, ChildCols: []string{"tenant_id", "code"}, ChildPK: []string{"tenant_id", "code"},
	ParentKey: func(p *models.Store) ([]any, bool) {
		if p.RegionCode == nil {
			return nil, false
		}
		return []any{p.TenantID, *p.RegionCode}, true
	},
	ChildKey: func(c *models.Region) ([]any, bool) { return []any{c.TenantID, c.Code}, true },
	SetParentKey: func(p *models.Store, k []any) {
		p.TenantID = k[0].(int64)
		s := k[1].(string)
		p.RegionCode = &s
	},
	GetOne: func(p *models.Store) *models.Region { return p.Region },
	SetOne: func(p *models.Store, c *models.Region) { p.Region = c },
}
```

- [ ] **Step 6: Implement the schema, seed and open helpers** — create `internal/relfixture/schema.go`:

```go
// Package relfixture is the shared relation test fixture: models, generated
// descriptors (see the plan's generator contract) and seeded SQLite/PostgreSQL databases.
package relfixture

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// ddl is dialect-neutral: BIGINT, TEXT and BOOLEAN exist on SQLite and PostgreSQL.
var ddl = []string{
	`CREATE TABLE authors (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
	`CREATE TABLE documents (id BIGINT PRIMARY KEY, author_id BIGINT REFERENCES authors(id), title TEXT NOT NULL)`,
	`CREATE TABLE comments (id BIGINT PRIMARY KEY, document_id BIGINT NOT NULL REFERENCES documents(id), body TEXT NOT NULL, approved BOOLEAN NOT NULL)`,
	`CREATE TABLE tags (id BIGINT PRIMARY KEY, name TEXT NOT NULL)`,
	`CREATE TABLE taggings (document_id BIGINT NOT NULL, tag_id BIGINT NOT NULL, position BIGINT NOT NULL DEFAULT 0, PRIMARY KEY (document_id, tag_id))`,
	`CREATE TABLE images (id BIGINT PRIMARY KEY, owner_type TEXT NOT NULL, owner_id BIGINT NOT NULL, url TEXT NOT NULL)`,
	`CREATE TABLE employees (id BIGINT PRIMARY KEY, manager_id BIGINT REFERENCES employees(id), name TEXT NOT NULL)`,
	`CREATE TABLE regions (tenant_id BIGINT NOT NULL, code TEXT NOT NULL, name TEXT NOT NULL, PRIMARY KEY (tenant_id, code))`,
	`CREATE TABLE stores (id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, region_code TEXT, name TEXT NOT NULL, FOREIGN KEY (tenant_id, region_code) REFERENCES regions(tenant_id, code))`,
}

// seed rows, in dependency order.
var seed = []struct {
	sql  string
	args []any
}{
	{`INSERT INTO authors (id, name) VALUES (?, ?)`, []any{int64(1), "Ada"}},
	{`INSERT INTO authors (id, name) VALUES (?, ?)`, []any{int64(2), "Grace"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(1), int64(1), "A1"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(2), int64(1), "A2"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, ?, ?)`, []any{int64(3), int64(2), "G1"}},
	{`INSERT INTO documents (id, author_id, title) VALUES (?, NULL, ?)`, []any{int64(4), "Orphan"}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(1), int64(1), "c1", true}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(2), int64(1), "c2", false}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(3), int64(1), "c3", true}},
	{`INSERT INTO comments (id, document_id, body, approved) VALUES (?, ?, ?, ?)`, []any{int64(4), int64(3), "c4", true}},
	{`INSERT INTO tags (id, name) VALUES (?, ?)`, []any{int64(1), "go"}},
	{`INSERT INTO tags (id, name) VALUES (?, ?)`, []any{int64(2), "db"}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(1), int64(1), int64(1)}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(1), int64(2), int64(2)}},
	{`INSERT INTO taggings (document_id, tag_id, position) VALUES (?, ?, ?)`, []any{int64(3), int64(2), int64(1)}},
	{`INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, []any{int64(1), "document", int64(1), "u1"}},
	{`INSERT INTO images (id, owner_type, owner_id, url) VALUES (?, ?, ?, ?)`, []any{int64(2), "author", int64(1), "u2"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, NULL, ?)`, []any{int64(1), "boss"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, ?, ?)`, []any{int64(2), int64(1), "dev1"}},
	{`INSERT INTO employees (id, manager_id, name) VALUES (?, ?, ?)`, []any{int64(3), int64(1), "dev2"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(1), "EU", "Europe"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(1), "US", "America"}},
	{`INSERT INTO regions (tenant_id, code, name) VALUES (?, ?, ?)`, []any{int64(2), "EU", "Europe2"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(1), int64(1), "EU", "Paris"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(2), int64(1), "US", "NYC"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, NULL, ?)`, []any{int64(3), int64(1), "Nowhere"}},
	{`INSERT INTO stores (id, tenant_id, region_code, name) VALUES (?, ?, ?, ?)`, []any{int64(4), int64(2), "EU", "Other"}},
}

// CreateSchema applies the fixture DDL. The database must be empty.
func CreateSchema(t testing.TB, db *momo.DB) {
	t.Helper()
	for _, s := range ddl {
		if _, err := momo.Exec(context.Background(), db, s); err != nil {
			t.Fatalf("ddl %q: %v", s, err)
		}
	}
}

// SeedBasic inserts the deterministic rows that the relation tests assert on:
// authors 1 (Ada) and 2 (Grace); documents 1,2 by Ada, 3 by Grace, 4 with a
// NULL author; comments 1-3 on document 1 and comment 4 on document 3;
// tags go and db; taggings (1,1,pos1), (1,2,pos2), (3,2,pos1); images owned by
// document 1 and author 1; employees boss(1), dev1(2) and dev2(3) reporting to
// boss; regions (1,EU),(1,US),(2,EU); stores 1 (1,EU), 2 (1,US), 3 (1,NULL), 4 (2,EU).
func SeedBasic(t testing.TB, db *momo.DB) {
	t.Helper()
	for _, s := range seed {
		if _, err := momo.Exec(context.Background(), db, s.sql, s.args...); err != nil {
			t.Fatalf("seed %q: %v", s.sql, err)
		}
	}
}

// Hook counts statements and records relation events. Each subtest gets its own.
type Hook struct {
	After  []string // relation name of each statement ("" when not a relation load)
	Events []momo.RelationEvent
}

func (h *Hook) BeforeQuery(ctx context.Context, e *momo.QueryEvent) context.Context { return ctx }
func (h *Hook) AfterQuery(ctx context.Context, e *momo.QueryEvent)                 { h.After = append(h.After, e.Relation) }
func (h *Hook) BeforeRelation(ctx context.Context, e *momo.RelationEvent) context.Context {
	return ctx
}
func (h *Hook) AfterRelation(ctx context.Context, e *momo.RelationEvent) { h.Events = append(h.Events, *e) }

// Each runs fn once per dialect available in this build (testopen.Each), on a
// fresh database with the fixture schema applied. The hook is attached to every
// database; its counters are reset for each run.
func Each(t *testing.T, fn func(t *testing.T, db *momo.DB, h *Hook)) {
	t.Helper()
	h := &Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		h.After, h.Events = nil, nil
		CreateSchema(t, db)
		fn(t, db, h)
	}, momo.WithHook(h))
}

// PostgresDriver returns a pgx-backed driver on a fresh database from
// testdb.PostgresURL, for tests that need the driver itself (round-trip counting).
func PostgresDriver(t testing.TB) momo.Driver {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testdb.PostgresURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	return pgxdriver.New(pool)
}
```

- [ ] **Step 7: Run and expect PASS** — `go test ./internal/relfixture/... -run '^TestRelfixtureSchemaApplies$' -count=1` prints `ok`. (`internal/testdb` is delivered by momo-01; this fixture uses its `SQLitePath` and `PostgresURL` helpers.)

- [ ] **Step 8: Commit** — `git add internal/relfixture && git commit -m "test(relations): add relation fixture models, descriptors and seed"`.

---

## Task 4: Tuple IN predicate and bind-limit budget

Files:

- Create `in_tuple.go`
- Create `in_tuple_internal_test.go`

Interfaces:

- `type inTuple struct{ table string; cols []string; keys [][]any }` implementing `Predicate` (`AppendSQL(*Builder)`, `resultType() bool`).
- `func keysPerStatement(d *dialect.Dialect, ncols, reserved int) int`.

Covers: S-6 (bind-limit chunking; empty IN renders always-false), S-77 (tuple IN for composite keys). Acceptance test named here: `TestTupleInRendering` (internal).

- [ ] **Step 1: Write the failing test** — create `in_tuple_internal_test.go`:

```go
package momo

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func renderPred(t *testing.T, d dialect.Name, p Predicate) Compiled {
	t.Helper()
	c, err := compile(dialect.For(d), func(b *Builder) { p.AppendSQL(b) })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTupleInRendering(t *testing.T) {
	single := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a"}, keys: [][]any{{int64(1)}, {int64(2)}}})
	if single.SQL != `"c"."a" IN ($1, $2)` || len(single.Args) != 2 {
		t.Fatalf("single: got %q args=%d", single.SQL, len(single.Args))
	}
	pgComposite := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}}})
	if pgComposite.SQL != `("c"."a", "c"."b") IN (($1, $2))` {
		t.Fatalf("pg composite: got %q", pgComposite.SQL)
	}
	liteComposite := renderPred(t, dialect.SQLite, inTuple{table: "c", cols: []string{"a", "b"}, keys: [][]any{{int64(1), "x"}, {int64(2), "y"}}})
	if strings.Contains(liteComposite.SQL, ") IN (") || strings.Count(liteComposite.SQL, " OR ") != 1 {
		t.Fatalf("sqlite composite must use OR of ANDs: got %q", liteComposite.SQL)
	}
	empty := renderPred(t, dialect.Postgres, inTuple{table: "c", cols: []string{"a"}})
	if empty.SQL != "1 = 0" || len(empty.Args) != 0 {
		t.Fatalf("empty IN must render always-false: got %q", empty.SQL)
	}
	if n := keysPerStatement(dialect.For(dialect.SQLite), 2, 16); n != (32766-16)/2 {
		t.Fatalf("sqlite budget: got %d", n)
	}
	if n := keysPerStatement(dialect.For(dialect.Postgres), 1, 16); n != 65535-16 {
		t.Fatalf("postgres budget: got %d", n)
	}
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestTupleInRendering$' -count=1` fails with `undefined: inTuple`.

- [ ] **Step 3: Implement** — create `in_tuple.go`:

```go
package momo

import "github.com/azrtydxb/momo/dialect"

// inTuple renders "key IN (...)" for one or more columns. An empty key list
// renders the always-false predicate. Composite keys use row values on
// PostgreSQL and MySQL; SQLite uses an OR of ANDs (valid on every SQLite
// version Momo supports). Values are bound, never interpolated.
type inTuple struct {
	table string
	cols  []string
	keys  [][]any
}

func (x inTuple) AppendSQL(b *Builder) {
	if len(x.keys) == 0 {
		b.WriteString("1 = 0")
		return
	}
	if len(x.cols) == 1 {
		b.QualifiedIdent(x.table, x.cols[0])
		b.WriteString(" IN (")
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Arg(k[0])
		}
		b.WriteByte(')')
		return
	}
	if b.Dialect().RowValues && b.Dialect().Name != dialect.SQLite {
		b.WriteByte('(')
		for i, c := range x.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(x.table, c)
		}
		b.WriteString(") IN (")
		for i, k := range x.keys {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			for j, v := range k {
				if j > 0 {
					b.WriteString(", ")
				}
				b.Arg(v)
			}
			b.WriteByte(')')
		}
		b.WriteByte(')')
		return
	}
	b.WriteByte('(')
	for i, k := range x.keys {
		if i > 0 {
			b.WriteString(" OR ")
		}
		b.WriteByte('(')
		for j, c := range x.cols {
			if j > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(x.table, c)
			b.WriteString(" = ")
			b.Arg(k[j])
		}
		b.WriteByte(')')
	}
	b.WriteByte(')')
}

func (inTuple) resultType() bool { return false }

// keysPerStatement returns how many key tuples fit in one statement under the
// dialect's bind limit, leaving reserved parameters for other predicates. A
// dialect without a limit (MaxParams <= 0) returns a large bound.
func keysPerStatement(d *dialect.Dialect, ncols, reserved int) int {
	if d.MaxParams <= 0 {
		return 1 << 20
	}
	n := (d.MaxParams - reserved) / ncols
	if n < 1 {
		return 1
	}
	return n
}
```

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestTupleInRendering$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add in_tuple.go in_tuple_internal_test.go && git commit -m "feat(relations): tuple IN predicate and bind-limit budget"`.

Reconciliation note (2026-11-05): the `keysPerStatement` body above is superseded. Composite keys (ncols > 1) are capped per statement, because the raw bind budget is unusable for them: PostgreSQL rejects a long row-value IN list with SQLSTATE 54001 (stack depth limit exceeded) at roughly 10000 two-column tuples, and SQLite rejects the OR-of-ANDs form beyond expression depth 1000. The caps are the named constants `maxCompositeTuples = 2000` (PostgreSQL, MySQL, others) and `maxCompositeTuplesSQLite = 400`; single-column keys keep the raw budget. `inTuple` also records a build error (`Builder.SetErr`) for keys without columns or a key tuple whose length differs from the column count, and its doc comment states that NULL key elements never match.

---

## Task 5: Eager loading core for belongs-to and has-one, relation hook events

Files:

- Create `loader.go` (`Loader[P]`, `pendingLoad`, `LoadOption`, `loadConfig`, `Load`, `LoadAll` sequential, `Relation.Scope`, `Relation.Then`, scoped loader)
- Create `load.go` (`Relation.prepare`, `pending[P,C]` with `run`, `consume`, `finish`, child query builder for BelongsTo/HasOne/HasMany)
- Create `relhook.go` (`RelationEvent`, `RelationHook`, `beginRelation`)
- Modify `db.go` (add `relHooks []RelationHook` set in `WithHook`; add `db *DB` reference in execer if absent: check `db.go:108`)
- Create `load_test.go` (package `momo_test`)

Interfaces:

```go
func Load[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) error
func LoadAll[P any](ctx context.Context, q Querier, parents []P, loaders ...Loader[P]) error
type loadConfig struct { where []Predicate; order []Orderable; perParentLimit int }
type pendingLoad interface { run(ctx context.Context, q Querier) error } // extended in Task 14
func (r Relation[P, C]) Scope(opts ...LoadOption[C]) Loader[P]
func (r Relation[P, C]) Then(nested ...Loader[C]) Loader[P]
func LoadWhere[C any](ps ...Predicate) LoadOption[C]
func LoadOrderBy[C any](items ...Orderable) LoadOption[C]
func LoadPerParentLimit[C any](n int) LoadOption[C]
// RelationEvent and RelationHook are defined in relhook.go (Step 6).
```

Covers: S-6 (batched IN-list loading, stable association, NULL-FK empty, no parent duplication for to-one), S-18 (relation loading reported to hooks). Acceptance test names here: `TestEagerLoadBelongsTo`, `TestEagerLoadHasOne`, `TestRelationHookEvents`.

- [ ] **Step 1: Write the failing tests** — create `load_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/author"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)


func allDocs(t *testing.T, db *momo.DB) []models.Document {
	t.Helper()
	docs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return docs
}

func TestEagerLoadBelongsTo(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		h.After = nil

		if err := momo.Load(context.Background(), db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 || h.After[0] != "documents.Author" {
			t.Fatalf("want exactly one child statement tagged documents.Author, got %v", h.After)
		}
		wantNames := map[int64]string{1: "Ada", 2: "Ada", 3: "Grace"}
		for _, d := range docs {
			want, ok := wantNames[d.ID]
			if !ok {
				if d.Author != nil {
					t.Fatalf("doc %d with NULL FK must keep Author nil", d.ID)
				}
				continue
			}
			if d.Author == nil || d.Author.Name != want {
				t.Fatalf("doc %d: author %+v, want %s", d.ID, d.Author, want)
			}
		}
		if docs[0].Author == docs[1].Author {
			t.Fatal("parents sharing a key must get distinct copies, not one shared pointer")
		}
	})
}

func TestEagerLoadHasOne(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		// has-one fixture: a belongs-to from the author side is exercised through
		// author.Images semantics; here the to-one is document.Author inverted:
		// each author has at most one document whose id equals authors.id.
		// Load it with a one-off descriptor that matches the generator contract.
		relfixture.SeedBasic(t, db)
		authors, err := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(context.Background(), db, authors, author.FirstDocument); err != nil {
			t.Fatal(err)
		}
		if authors[0].FirstDocument == nil || authors[0].FirstDocument.ID != 1 || authors[1].FirstDocument == nil || authors[1].FirstDocument.ID != 2 {
			t.Fatalf("has-one loaded wrong documents: %+v %+v", authors[0].FirstDocument, authors[1].FirstDocument)
		}
	})
}

func TestRelationHookEvents(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		if err := momo.Load(context.Background(), db, docs, document.Author); err != nil {
			t.Fatal(err)
		}
		if len(h.Events) != 1 {
			t.Fatalf("want one RelationEvent, got %d", len(h.Events))
		}
		e := h.Events[0]
		if e.Parent != "documents" || e.Name != "Author" || e.Kind != momo.BelongsTo || e.Parents != 4 || e.Children != 2 || e.Queries != 1 {
			t.Fatalf("unexpected event %+v", e)
		}
	})
}

```

Note: `TestEagerLoadHasOne` uses `author.FirstDocument`. Add that descriptor to `internal/relfixture/author/momo_gen.go` in step 3 below, as a has-one from authors to documents where `documents.id = authors.id` (a deliberately simple fixture that gives the to-one-from-parent path a test: HasOne with `ParentCols: []string{"id"}`, `ChildCols: []string{"id"}`).

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run 'TestEagerLoadBelongsTo|TestEagerLoadHasOne|TestRelationHookEvents' -count=1` fails with `undefined: momo.Load` and `author.FirstDocument undefined`.

- [ ] **Step 3: Add the has-one fixture descriptor** — append to `internal/relfixture/author/momo_gen.go`:

```go
// FirstDocument is a has-one used for the has-one path: the document whose id
// equals the author's id (fixture only; documents.id = authors.id).
var FirstDocument = momo.Relation[models.Author, models.Document]{
	Name: "FirstDocument", Kind: momo.HasOne,
	Parent: momo.TableRef{Name: "authors"}, Child: momo.TableRef{Name: "documents"},
	ParentMeta: (*models.Author)(nil).MomoMeta(), ChildMeta: (*models.Document)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"id"}, ChildPK: []string{"id"},
	ParentKey: func(p *models.Author) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:  func(c *models.Document) ([]any, bool) { return []any{c.ID}, true },
	SetChildKey: func(c *models.Document, k []any) {
		c.ID = k[0].(int64)
	},
	GetOne: func(p *models.Author) *models.Document { return p.FirstDocument },
	SetOne: func(p *models.Author, c *models.Document) { p.FirstDocument = c },
}
```

The `FirstDocument` descriptor above is final; `models.Author.FirstDocument` is declared in the Task 3 model file.

- [ ] **Step 4: Implement the loader core** — create `loader.go`:

```go
package momo

import (
	"context"
)

// Loader is one relation load request, optionally scoped and nested.
// It is sealed: only Relation.Scope, Relation.Then and Relation itself implement it.
type Loader[P any] interface {
	relationName() string
	prepare(q Querier, parents []P) (pendingLoad, error)
}

// pendingLoad is a relation load whose statements are compiled but not yet run.
type pendingLoad interface {
	run(ctx context.Context, q Querier) error
}

// loadConfig holds per-load options. Options never mutate a shared slice:
// each option appends to a clipped copy.
type loadConfig struct {
	where          []Predicate
	order          []Orderable
	perParentLimit int
}

// LoadOption configures one relation load.
type LoadOption[C any] func(*loadConfig)

// LoadWhere adds predicates on the child table to the child statement.
func LoadWhere[C any](ps ...Predicate) LoadOption[C] {
	return func(c *loadConfig) { c.where = append(c.where[:len(c.where):len(c.where)], ps...) }
}

// LoadOrderBy sets the child order. Stable tie-breakers (relation key columns,
// then child primary key) are always appended after it.
func LoadOrderBy[C any](items ...Orderable) LoadOption[C] {
	return func(c *loadConfig) { c.order = append(c.order[:len(c.order):len(c.order)], items...) }
}

// LoadPerParentLimit keeps at most n children per parent (implemented with a
// window function; see the top-N task).
func LoadPerParentLimit[C any](n int) LoadOption[C] {
	return func(c *loadConfig) { c.perParentLimit = n }
}

// Load loads rel for parents and assigns the result onto each parent. Parent
// order and length are unchanged. Parents with a NULL key get an empty
// association (nil pointer or nil slice).
func Load[P, C any](ctx context.Context, q Querier, parents []P, rel Relation[P, C], opts ...LoadOption[C]) error {
	return rel.scoped(opts, nil).load(ctx, q, parents)
}

// LoadAll loads several relations for the same parents, in order. The top
// level of each relation is one statement per bind-limit chunk; nested loads
// run once over all children of the level above.
func LoadAll[P any](ctx context.Context, q Querier, parents []P, loaders ...Loader[P]) error {
	if len(parents) == 0 {
		return nil
	}
	pending := make([]pendingLoad, 0, len(loaders))
	for _, l := range loaders {
		p, err := l.prepare(q, parents)
		if err != nil {
			return err
		}
		pending = append(pending, p)
	}
	for _, p := range pending {
		if err := p.run(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// Scope returns a loader for r with per-load options.
func (r Relation[P, C]) Scope(opts ...LoadOption[C]) Loader[P] {
	return r.scoped(opts, nil)
}

// Then returns a loader for r that also loads nested relations on the children.
func (r Relation[P, C]) Then(nested ...Loader[C]) Loader[P] {
	return r.scoped(nil, nested)
}

// scopedLoader is the sealed Loader implementation for a relation with options and nested loads.
type scopedLoader[P, C any] struct {
	rel    Relation[P, C]
	cfg    loadConfig
	nested []Loader[C]
}

func (r Relation[P, C]) scoped(opts []LoadOption[C], nested []Loader[C]) scopedLoader[P, C] {
	s := scopedLoader[P, C]{rel: r, nested: nested}
	for _, o := range opts {
		o(&s.cfg)
	}
	return s
}

func (s scopedLoader[P, C]) relationName() string { return s.rel.Name }

func (s scopedLoader[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return s.rel.prepareWith(q, parents, &s.cfg, s.nested)
}

func (s scopedLoader[P, C]) load(ctx context.Context, q Querier, parents []P) error {
	return LoadAll(ctx, q, parents, Loader[P](s))
}

// Relation implements Loader for the plain relation (no options, no nesting).
func (r Relation[P, C]) relationName() string { return r.Name }

func (r Relation[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return r.prepareWith(q, parents, &loadConfig{}, nil)
}
```

- [ ] **Step 5: Implement prepare/run/consume/finish** — create `load.go`:

```go
package momo

import (
	"context"
	"errors"
)

// pending is one relation's load in progress: its child statements, the parent
// key groups, and the child rows collected so far.
type pending[P, C any] struct {
	r       *Relation[P, C]
	q       Querier
	parents []P
	cfg     *loadConfig
	nested  []Loader[C]
	groups  map[string][]int // parent key -> parent indexes
	keys    [][]any          // distinct parent keys, first-seen order
	queries []Compiled
	pool    []C              // child rows, in query order
	owners  [][]int          // owners[j]: parent indexes that receive pool[j]
	ev      *RelationEvent   // event reported to relation hooks
}

// prepareWith validates the descriptor, clears existing associations and
// compiles the child statements. It is the single entry point used by all
// loaders.
func (r *Relation[P, C]) prepareWith(q Querier, parents []P, cfg *loadConfig, nested []Loader[C]) (pendingLoad, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	pd := &pending[P, C]{r: r, q: q, parents: parents, cfg: cfg, nested: nested, groups: map[string][]int{}}
	for i := range parents {
		r.clear(&parents[i])
		vals, ok := r.ParentKey(&parents[i])
		if !ok {
			continue
		}
		k := keyString(vals)
		if _, seen := pd.groups[k]; !seen {
			pd.keys = append(pd.keys, vals)
		}
		pd.groups[k] = append(pd.groups[k], i)
	}
	qs, err := r.childQueries(q, pd.keys, cfg)
	if err != nil {
		return nil, err
	}
	pd.queries = qs
	return pd, nil
}

// clear empties the association before loading, so a parent that matches
// nothing (or has a NULL key) ends up empty rather than stale.
func (r *Relation[P, C]) clear(p *P) {
	if r.Kind == BelongsTo || r.Kind == HasOne {
		r.SetOne(p, nil)
		return
	}
	r.SetMany(p, nil)
}

// run executes the child statements one at a time, consuming each result set
// before the next statement starts (a single pooled connection must never be
// held by two open result sets). Nested loads run in finish.
func (pd *pending[P, C]) run(ctx context.Context, q Querier) error {
	ctx, end := pd.begin(ctx, q)
	err := pd.runQueries(ctx, q)
	if err == nil {
		err = pd.finish(ctx)
	}
	end(err)
	return err
}

// begin reports the start of this relation load to relation hooks.
func (pd *pending[P, C]) begin(ctx context.Context, q Querier) (context.Context, func(error)) {
	return beginRelation(ctx, q.conn().db, pd.event())
}

// event returns this load's RelationEvent, created on first use.
func (pd *pending[P, C]) event() *RelationEvent {
	if pd.ev == nil {
		pd.ev = &RelationEvent{Parent: pd.r.Parent.Name, Name: pd.r.Name, Kind: pd.r.Kind, Parents: len(pd.parents), Queries: len(pd.queries)}
	}
	return pd.ev
}

func (pd *pending[P, C]) runQueries(ctx context.Context, q Querier) error {
	rel := pd.r.Parent.Name + "." + pd.r.Name
	for _, c := range pd.queries {
		rows, err := q.conn().query(ctx, "select", pd.r.Child.Name, rel, c)
		if err != nil {
			return err
		}
		if err := pd.consume(rows); err != nil {
			return err
		}
	}
	return nil
}

// consume reads one result set into the pool and closes it. T14 adds the
// batched path, which calls consume once per result set in order.
func (pd *pending[P, C]) consume(rows Rows) error {
	defer rows.Close()
	for rows.Next() {
		var c C
		if err := scanChild(pd.r, rows, &c); err != nil {
			return err
		}
		key, ok := pd.r.ChildKey(&c)
		if !ok {
			continue
		}
		owners := pd.groups[keyString(key)]
		if len(owners) == 0 {
			continue
		}
		pd.pool = append(pd.pool, c)
		pd.owners = append(pd.owners, owners)
	}
	return rows.Err()
}

// finish runs nested loads once over the whole pool, then assigns children to
// their parents. To-one children are copied per parent; to-many children are
// collected per parent in pool order, so association order is stable.
func (pd *pending[P, C]) finish(ctx context.Context) error {
	if pd.ev != nil {
		pd.ev.Children = int64(len(pd.pool))
	}
	if len(pd.nested) > 0 && len(pd.pool) > 0 {
		if err := LoadAll(ctx, pd.q, pd.pool, pd.nested...); err != nil {
			return err
		}
	}
	toOne := pd.r.Kind == BelongsTo || pd.r.Kind == HasOne
	counts := make([]int, len(pd.parents))
	lists := make([][]C, len(pd.parents))
	for j := range pd.pool {
		for _, i := range pd.owners[j] {
			counts[i]++
			if toOne {
				if counts[i] > 1 && pd.r.Kind == HasOne {
					return &Error{Kind: ErrTooManyRows, Op: "load", Table: pd.r.Child.Name,
						Err: errors.New(pd.r.Name + ": has-one matched more than one child")}
				}
				cp := pd.pool[j]
				pd.r.SetOne(&pd.parents[i], &cp)
				continue
			}
			lists[i] = append(lists[i], pd.pool[j])
		}
	}
	if !toOne {
		for i, l := range lists {
			if len(l) > 0 {
				pd.r.SetMany(&pd.parents[i], l)
			}
		}
	}
	return nil
}
```

Also add the child query builder and scan helper in the same file (append to `load.go`):

```go
// childQueries builds one statement per bind-limit chunk for the BelongsTo,
// HasOne and HasMany shapes. T6 and T9 extend it: T9 adds the ManyToMany
// branch, T12 adds the top-N branch.
func (r *Relation[P, C]) childQueries(q Querier, keys [][]any, cfg *loadConfig) ([]Compiled, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	d := q.Dialect()
	scopeArgs, err := countPredicateArgs(d, cfg.where)
	if err != nil {
		return nil, err
	}
	reserved := 16 + scopeArgs
	budget := keysPerStatement(d, len(r.ChildCols), reserved)
	var out []Compiled
	for start := 0; start < len(keys); start += budget {
		end := min(start+budget, len(keys))
		chunk := keys[start:end]
		c, err := compile(d, func(b *Builder) {
			b.WriteString("SELECT ")
			r.writeChildColumns(b)
			b.WriteString(" FROM ")
			r.Child.appendFrom(b)
			b.WriteString(" WHERE ")
			inTuple{table: r.Child.Qualifier(), cols: r.ChildCols, keys: chunk}.AppendSQL(b)
			r.writeScope(b, cfg)
			b.WriteString(" ORDER BY ")
			r.writeOrder(b, cfg)
		})
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *Relation[P, C]) writeChildColumns(b *Builder) {
	for i, col := range r.ChildMeta.Columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.QualifiedIdent(r.Child.Qualifier(), col.Name)
	}
}

// writeScope appends the discriminator and user predicates with AND.
func (r *Relation[P, C]) writeScope(b *Builder, cfg *loadConfig) {
	var ps []Predicate
	if r.Discriminator != nil {
		ps = append(ps, binOp{qcol{r.Child.Qualifier(), r.Discriminator.Column}, " = ", argExpr{r.Discriminator.Value}})
	}
	ps = append(ps, cfg.where...)
	for _, p := range ps {
		b.WriteString(" AND ")
		p.AppendSQL(b)
	}
}

// writeOrder writes user order items, then the relation key columns, then the child primary key.
func (r *Relation[P, C]) writeOrder(b *Builder, cfg *loadConfig) {
	first := true
	sep := func() {
		if !first {
			b.WriteString(", ")
		}
		first = false
	}
	for _, o := range cfg.order {
		sep()
		o.order().appendSQL(b)
	}
	for _, c := range r.ChildCols {
		sep()
		b.QualifiedIdent(r.Child.Qualifier(), c)
	}
	for _, c := range r.ChildPK {
		sep()
		b.QualifiedIdent(r.Child.Qualifier(), c)
	}
}

// countPredicateArgs returns how many bind parameters the predicates render.
func countPredicateArgs(d *dialect.Dialect, ps []Predicate) (int, error) {
	if len(ps) == 0 {
		return 0, nil
	}
	c, err := compile(d, func(b *Builder) {
		for _, p := range ps {
			p.AppendSQL(b)
		}
	})
	if err != nil {
		return 0, err
	}
	return len(c.Args), nil
}

// scanChild scans the child's mapped columns in ChildMeta order.
func scanChild[P, C any](r *Relation[P, C], rows Rows, c *C) error {
	dest := make([]any, len(r.ChildMeta.Columns))
	for i := range dest {
		dest[i] = r.ChildMeta.Ptr(c, i)
	}
	return rows.Scan(dest...)
}
```

Note: the `childQueries` builder must use the child's alias-free table name `r.Child.Qualifier()` (TableRef.Qualifier exists in meta.go). `binOp`, `qcol`, `argExpr` exist in the spike (`expr.go`, `select.go`). Add the `dialect` import to `load.go` in step 5.

- [ ] **Step 6: Implement the hook plumbing** — create `relhook.go`:

```go
package momo

import (
	"context"
	"time"
)

// RelationEvent describes one relation load (S-18). Children and Duration are
// filled before AfterRelation; Queries counts child statements issued.
type RelationEvent struct {
	Parent   string
	Name     string
	Kind     RelationKind
	Parents  int
	Children int64
	Queries  int
	Duration time.Duration
	Err      error
}

// RelationHook is an optional extension of Hook. Hooks registered with
// WithHook that implement it receive relation events.
type RelationHook interface {
	BeforeRelation(ctx context.Context, e *RelationEvent) context.Context
	AfterRelation(ctx context.Context, e *RelationEvent)
}

var noopRelationEnd = func(error) {}

// beginRelation calls BeforeRelation on relation hooks and returns the end
// function. Without relation hooks it allocates nothing.
func beginRelation(ctx context.Context, db *DB, e *RelationEvent) (context.Context, func(error)) {
	if db == nil || len(db.relHooks) == 0 {
		return ctx, noopRelationEnd
	}
	e.Duration = 0
	start := time.Now()
	for _, h := range db.relHooks {
		ctx = h.BeforeRelation(ctx, e)
	}
	return ctx, func(err error) {
		e.Err = err
		e.Duration = time.Since(start)
		for _, h := range db.relHooks {
			h.AfterRelation(ctx, e)
		}
	}
}
```

Also: in `pending.run`, the `RelationEvent` has to carry the `Children` count: `consume` increments `pd.children` (add field `children int64` to `pending` and set `e.Children` in `end` by passing a pointer). Implement by building the `*RelationEvent` as `pd.event` stored on the pending struct; `run` passes `pd.event` and `consume` increments `pd.event.Children`. Keep `end` closure reading the same pointer.

Modify `db.go`: in `WithHook`, after appending, also append to `db.relHooks` when `h` implements `RelationHook`; add field `relHooks []RelationHook` to `DB`. If `execer` lacks a `db` back-pointer, add `db *DB` to `execer` and set it in `New` and `Begin`.

- [ ] **Step 7: Run and expect PASS** — `go test ./ ./internal/relfixture/... -run 'TestEagerLoadBelongsTo|TestEagerLoadHasOne|TestRelationHookEvents' -count=1` prints `ok`.

- [ ] **Step 8: Commit** — `git add loader.go load.go relhook.go db.go load_test.go internal/relfixture/author && git commit -m "feat(relations): eager loading for belongs-to and has-one with relation events"`.

---

## Task 6: Has-many loading, stable order, parent pagination and NULL-key emptiness

Files:

- Modify `load_test.go` (add tests)
- Modify `load.go` (no logic change expected; `writeOrder` already adds key then PK tie-breakers)

Interfaces:

- No new exported names. Uses `Load`, `document.Comments`, `employee.Reports`.

Covers: S-6 (parent pagination preserved, stable association, defined NULL behavior, no parent duplication). Acceptance test names: `TestEagerLoadHasMany`, `TestEagerLoadParentOrderAndPagination`, `TestEagerLoadNullFKEmpty`.

- [ ] **Step 1: Write the failing tests** — append to `load_test.go`:

```go
func TestEagerLoadHasMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		docs := allDocs(t, db)
		if err := momo.Load(context.Background(), db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		got := map[int64][]int64{}
		for _, d := range docs {
			for _, c := range d.Comments {
				got[d.ID] = append(got[d.ID], c.ID)
			}
		}
		if fmt.Sprint(got[1]) != "[1 2 3]" || fmt.Sprint(got[3]) != "[4]" || len(got[2]) != 0 || len(got[4]) != 0 {
			t.Fatalf("comments by doc: %v", got)
		}
	})
}

func TestEagerLoadParentOrderAndPagination(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		// A page of parents, reversed and with a duplicate key on purpose.
		page, err := momo.Select[models.Document, *models.Document](db).
			Where(document.ID.Le(3)).OrderBy(document.ID.Desc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		page = append(page, page[0]) // duplicate parent row
		if err := momo.Load(context.Background(), db, page, document.Comments); err != nil {
			t.Fatal(err)
		}
		ids := []int64{}
		for _, d := range page {
			ids = append(ids, d.ID)
		}
		if fmt.Sprint(ids) != "[3 2 1 3]" {
			t.Fatalf("parent order changed: %v", ids)
		}
		if len(page[0].Comments) != 1 || len(page[3].Comments) != 1 || page[3].Comments[0].ID != 4 {
			t.Fatalf("duplicate parent must receive its own copy of the children: %+v", page[3].Comments)
		}
		if len(page[2].Comments) != 3 {
			t.Fatalf("parent 1 must have 3 comments, got %d", len(page[2].Comments))
		}
	})
}

func TestEagerLoadNullFKEmpty(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		// Employee 1 has a NULL manager_id: Manager must stay nil, and Reports of boss must be 2 employees.
		if err := momo.Load(context.Background(), db, emps, employee.Manager); err != nil {
			t.Fatal(err)
		}
		if emps[0].Manager != nil || emps[1].Manager == nil || emps[1].Manager.Name != "boss" {
			t.Fatalf("manager loading wrong: %+v", emps)
		}
		if err := momo.Load(context.Background(), db, emps, employee.Reports); err != nil {
			t.Fatal(err)
		}
		if len(emps[0].Reports) != 2 || emps[0].Reports[0].Name != "dev1" || emps[0].Reports[1].Name != "dev2" {
			t.Fatalf("reports of boss: %+v", emps[0].Reports)
		}
		if emps[1].Reports != nil {
			t.Fatal("a leaf must have a nil slice, not an empty one")
		}
	})
}
```

Append this test to `load_test.go` and add the imports `fmt`, `comment` and `employee` to its import block:

```go
func TestEagerLoadReloadClearsStaleChildren(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		// Reload with a scope that matches nothing: every association must become empty.
		if err := momo.LoadAll(ctx, db, docs, document.Comments.Scope(momo.LoadWhere[models.Comment](comment.ID.Eq(999)))); err != nil {
			t.Fatal(err)
		}
		for _, d := range docs {
			if d.Comments != nil {
				t.Fatalf("doc %d kept stale comments after reload: %d", d.ID, len(d.Comments))
			}
		}
	})
}
```

- [ ] **Step 2: Mutation check: the reload test must fail when the association reset is removed.** In `load.go`, delete the line `r.clear(&parents[i])` from the loop in `prepareWith`, then run `go test ./ -run '^TestEagerLoadReloadClearsStaleChildren$' -count=1`. Expect FAIL with `doc 1 kept stale comments after reload: 3`.
- [ ] **Step 3: Restore the line.** Put `r.clear(&parents[i])` back as the first statement of the `for i := range parents` loop in `prepareWith`. No other change.
- [ ] **Step 4: Run and expect PASS** — `go test ./ -run 'TestEagerLoadHasMany|TestEagerLoadParentOrderAndPagination|TestEagerLoadNullFKEmpty' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add load_test.go load.go && git commit -m "test(relations): has-many order, pagination and NULL-key emptiness"`.

---

## Task 7: Composite keys

Files:

- Modify `load_test.go`

Interfaces:

- Uses `region.Stores` (composite has-many) and `store.Region` (composite belongs-to).

Covers: S-6 composite keys. Acceptance test: `TestEagerLoadComposite`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

```go
func TestEagerLoadComposite(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		stores, err := momo.Select[models.Store, *models.Store](db).OrderBy(store.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, stores, store.Region); err != nil {
			t.Fatal(err)
		}
		// store 1 -> (1,EU) Europe; store 2 -> (1,US) America; store 3 NULL region_code -> nil; store 4 -> (2,EU) Europe2.
		want := map[int64]string{1: "Europe", 2: "America", 4: "Europe2"}
		for _, s := range stores {
			if name, ok := want[s.ID]; ok {
				if s.Region == nil || s.Region.Name != name {
					t.Fatalf("store %d: region %+v want %s", s.ID, s.Region, name)
				}
			} else if s.Region != nil {
				t.Fatalf("store %d with NULL composite component must have nil Region", s.ID)
			}
		}

		regions, err := momo.Select[models.Region, *models.Region](db).OrderBy(region.TenantID.Asc(), region.Code.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, regions, region.Stores); err != nil {
			t.Fatal(err)
		}
		if len(regions[0].Stores) != 1 || regions[0].Stores[0].Name != "Paris" {
			t.Fatalf("region (1,EU) stores: %+v", regions[0].Stores)
		}
		if len(regions[1].Stores) != 1 || regions[1].Stores[0].Name != "NYC" {
			t.Fatalf("region (1,US) stores: %+v", regions[1].Stores)
		}
		if len(regions[2].Stores) != 1 || regions[2].Stores[0].Name != "Other" {
			t.Fatalf("region (2,EU) stores: %+v", regions[2].Stores)
		}
	})
}
```

Add imports for `store` and `region` subpackages.

- [ ] **Step 2: Run and expect FAIL** — before the `store` and `region` imports are added, `go test ./ -run '^TestEagerLoadComposite$' -count=1` fails with `undefined: store`. After the imports are added, the test runs against the Task 4 and Task 5 code.
- [ ] **Step 3: Check the composite rendering.** Run `go test ./ -run '^TestTupleInRendering$' -count=1`; expect `ok`. Task 4 already pins the composite SQL shape, so a failure here means the qualifier rules in `inTuple` diverged from the FROM clause.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestEagerLoadComposite$' -count=1` prints `ok`. Also run `MOMO_TEST_POSTGRES_URL=$MOMO_TEST_POSTGRES_URL go test ./ -run '^TestEagerLoadComposite$' -count=1` where the URL is set by CI; locally this step is skipped if the variable is empty.

- [ ] **Step 5: Commit** — `git add load_test.go && git commit -m "test(relations): composite-key belongs-to and has-many"`.

Reconciliation note (2026-11-05): Task 7 needed one small logic change, not only tests. `prepareWith` now treats a parent key with a NULL (nil) component as no key even when the extractor reports `ok = true` (`hasNilPart` in `load.go`), so such a parent issues no statement and gets no association; generated extractors already return `ok = false` for a nullable component, so this guards hand-written descriptors. `TestEagerLoadComposite` passes on the Task 4 and Task 5 code (its red state is the compile failure from the missing `store` and `region` imports). The added tests `TestEagerLoadCompositeSemantics`, `TestEagerLoadCompositeEmptyAndStale`, `TestEagerLoadCompositeNullComponent` and the internal `TestLoadCompositeKeysDedupeNullAndChunk` pin one statement per load, parent-key dedupe, primary-key child order, tuple (not per-column) matching, stale clearing and composite chunking. The composite chunk test lowers `MaxParams` on a dialect copy because `WithMaxParams` arrives in Task 8.

---

## Task 8: Bind-limit chunking

Files:

- Create `options_maxparams.go` (`WithMaxParams`)
- Modify `load_test.go` (add imports `github.com/azrtydxb/momo/dialect` and `github.com/azrtydxb/momo/internal/testdb/testopen`)

Interfaces:

- `func WithMaxParams(n int) Option` — copies the DB's dialect and lowers `MaxParams` to `n`. Requires that `New` sets `db.ex.d` before applying options; if `New` applies options first, move the dialect assignment above the option loop in `db.go`.

Covers: S-6 (bind-limit chunking), S-5 (dialect limit). Acceptance test: `TestEagerLoadChunking`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

```go
func TestEagerLoadChunking(t *testing.T) {
	h := &relfixture.Hook{}
	testopen.Each(t, func(t *testing.T, db *momo.DB, _ dialect.Name) {
		h.After, h.Events = nil, nil
		relfixture.CreateSchema(t, db)
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db) // 4 documents; keys 1,2,3,4 -> with budget 4-16 < 1, chunk size is 1
		h.After = nil
		if err := momo.Load(ctx, db, docs, document.Comments); err != nil {
			t.Fatal(err)
		}
		// keysPerStatement reserves 16 parameters, so with MaxParams 4 each chunk holds one key.
		// Four distinct keys -> four child statements, all tagged with the relation.
		if len(h.After) != 4 {
			t.Fatalf("want 4 chunked child statements, got %d: %v", len(h.After), h.After)
		}
		total := 0
		for _, d := range docs {
			total += len(d.Comments)
		}
		if total != 4 {
			t.Fatalf("chunked load lost or duplicated comments: total %d", total)
		}
	}, momo.WithHook(h), momo.WithMaxParams(4))
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestEagerLoadChunking$' -count=1` fails with `undefined: momo.WithMaxParams`.

- [ ] **Step 3: Implement** — create `options_maxparams.go`:

```go
package momo

// WithMaxParams lowers the dialect bind-parameter limit for this DB. It is
// for tests and for drivers or proxies with tighter limits than the dialect
// default. The dialect value is copied; the shared dialect is never mutated.
func WithMaxParams(n int) Option {
	return func(db *DB) {
		d := *db.ex.d
		d.MaxParams = n
		db.ex.d = &d
	}
}

```

Ensure `New` assigns `db.ex.d` from the driver before the option loop (`db.go:70`); if it applies options first, move the assignment above the option loop.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestEagerLoadChunking$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add options_maxparams.go db.go load_test.go && git commit -m "feat(relations): bind-limit chunking with WithMaxParams"`.

Reconciliation note (2026-11-05): `New` applied options before building `db.ex`, so `db.go` now builds the execer first and runs the option loop after it (the plan's contingency). Transactions copy `db.ex.d`, so they inherit the lowered limit, and the bulk-insert budget (`InsertQuery.plan`) already reads the DB dialect, so `WithMaxParams` applies to inserts too without further change. Added `options_maxparams_test.go` (`TestWithMaxParamsCopiesDialect`, `TestWithMaxParamsAppliesToTransactionsAndInserts`) for the copy, transaction and insert behaviour. The Task 4-7 internal tests (`load_internal_test.go`, `load_composite_internal_test.go`, `in_tuple_edges_internal_test.go`) call `prepareWith` on a dialect copy with no `DB`, so they stay as they are; the plan does not ask to migrate them.

---

## Task 9: Many-to-many through a join table

Files:

- Modify `load.go` (add ManyToMany branch to `childQueries`; M2M key holders in `consume`)
- Modify `load_test.go`

Interfaces:

- Extends `childQueries` and `consume`; no new exported names.

Covers: S-6 (many-to-many), S-82 (through a join table). Acceptance test: `TestEagerLoadManyToMany`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

```go
func TestEagerLoadManyToMany(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		h.After = nil
		if err := momo.Load(ctx, db, docs, document.Tags); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("many-to-many must be one statement, got %d", len(h.After))
		}
		names := map[int64][]string{}
		for _, d := range docs {
			for _, tg := range d.Tags {
				names[d.ID] = append(names[d.ID], tg.Name)
			}
		}
		if fmt.Sprint(names[1]) != "[go db]" || len(names[2]) != 0 || len(names[3]) != 1 || names[3][0] != "db" {
			t.Fatalf("tags by doc: %v", names)
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestEagerLoadManyToMany$' -count=1` fails with `want M2M`-style output: `tags by doc: map[1:[] 3:[]]` (the Task 5 code treats ManyToMany as a plain has-many with no join, so the query returns no rows).

- [ ] **Step 3: Implement** — in `load.go`, in `childQueries`, branch on `r.Through != nil` before the plain branch. The M2M statement joins the join table and projects the parent key as extra holder columns:

```go
// m2mQuery replaces the plain statement for ManyToMany relations:
//   SELECT child.cols..., jt.parent_col AS momo_key_0, ...
//   FROM child INNER JOIN jt ON jt.child_col = child.child_col ...
//   WHERE (jt.parent_cols) IN (keys) AND scope ORDER BY ...
func (r *Relation[P, C]) m2mQuery(d *dialect.Dialect, chunk [][]any, cfg *loadConfig) (Compiled, error) {
	return compile(d, func(b *Builder) {
		b.WriteString("SELECT ")
		r.writeChildColumns(b)
		for i, c := range r.Through.ParentCols {
			b.WriteString(", ")
			b.QualifiedIdent(r.Through.Table.Qualifier(), c)
			b.WriteString(" AS ")
			b.Ident("momo_key_" + strconv.Itoa(i))
		}
		b.WriteString(" FROM ")
		r.Child.appendFrom(b)
		b.WriteString(" INNER JOIN ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" ON ")
		for i, c := range r.Through.ChildCols {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(r.Through.Table.Qualifier(), c)
			b.WriteString(" = ")
			b.QualifiedIdent(r.Child.Qualifier(), r.ChildCols[i])
		}
		b.WriteString(" WHERE ")
		inTuple{table: r.Through.Table.Qualifier(), cols: r.Through.ParentCols, keys: chunk}.AppendSQL(b)
		r.writeScope(b, cfg)
		b.WriteString(" ORDER BY ")
		r.writeOrder(b, cfg)
	})
}
```

In `childQueries`, replace the body of the chunk loop with `if r.Through != nil { c, err = r.m2mQuery(d, chunk, cfg) } else { ...existing compile... }`. Add imports `strconv` and `dialect`.

In `consume` for ManyToMany: the scan destinations are the child columns followed by the key holders; the key comes from the holders, not from `ChildKey`. Replace `consume`'s row loop with:

```go
	for rows.Next() {
		var c C
		var key []any
		if r.Through != nil {
			holders := make([]any, len(r.Through.ParentCols))
			dest := make([]any, 0, len(r.ChildMeta.Columns)+len(holders))
			for i := range r.ChildMeta.Columns {
				dest = append(dest, r.ChildMeta.Ptr(&c, i))
			}
			for i := range holders {
				dest = append(dest, &holders[i])
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			key = holders
		} else {
			if err := scanChild(pd.r, rows, &c); err != nil {
				return err
			}
			k, ok := pd.r.ChildKey(&c)
			if !ok {
				continue
			}
			key = k
		}
		owners := pd.groups[keyString(key)]
		if len(owners) == 0 {
			continue
		}
		pd.pool = append(pd.pool, c)
		pd.owners = append(pd.owners, owners)
	}
```

Important: `keyString` normalization (Task 1) is what makes the driver-returned holder values (int64, []byte, string) equal the struct-derived keys.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestEagerLoadManyToMany$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add load.go load_test.go && git commit -m "feat(relations): many-to-many eager loading through a join table"`.

Reconciliation note (2026-11-06): Task 9 differs from the sketch in four ways. (1) The per-chunk statement is built by `chunkQuery` (plain and join shapes) instead of a separate `m2mQuery` method, so `childQueries` keeps one loop. (2) The bind budget and the "scope leaves no room" check use the join table's parent key width (`len(Through.ParentCols)`), because that is the tuple the join statement binds, not `len(ChildCols)`. (3) `consume` skips rows with a NULL holder and collapses duplicated join-table rows by (holder parent key, child primary key) whenever the child meta has a primary key and a `Value` getter (review fix 2026-11-06: this replaced the narrower `ChildCols == ChildPK` condition, so relations targeting a non-PK unique column also dedupe), so a join table without a primary key never duplicates a child; a child without a primary key is not collapsed. (4) `Through` carries no `Meta`, so soft-delete is applied to the far table only; join-table rows are never filtered by a soft-delete column. Children shared by several parents are copied per parent (value slices), never aliased. The chunking test lowers `MaxParams` on a dialect copy (`WithMaxParams` is added separately).

---

## Task 10: Polymorphic, self-referencing, back-references and explicit join model

Files:

- Modify `load_test.go`

Interfaces:

- Uses `document.Images` (polymorphic), `author.Images` (polymorphic, other type), `employee.Manager` and `employee.Reports` (self-ref and back-reference), `document.Taggings` followed by `tagging.Tag` (join model with extra columns).

Covers: S-82 (polymorphic has-many, self-reference, back-reference, join model with extra columns). Acceptance test: `TestPolymorphicAndThrough`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

```go
func TestPolymorphicAndThrough(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		docs := allDocs(t, db)
		if err := momo.Load(ctx, db, docs, document.Images); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Images) != 1 || docs[0].Images[0].URL != "u1" {
			t.Fatalf("document images: %+v", docs[0].Images)
		}

		authors, err := momo.Select[models.Author, *models.Author](db).OrderBy(author.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, authors, author.Images); err != nil {
			t.Fatal(err)
		}
		if len(authors[0].Images) != 1 || authors[0].Images[0].URL != "u2" {
			t.Fatalf("author images must not mix with document images: %+v", authors[0].Images)
		}

		// Join model: document -> taggings (with Position) -> tag.
		if err := momo.Load(ctx, db, docs, document.Taggings.Then(tagging.Tag)); err != nil {
			t.Fatal(err)
		}
		if len(docs[0].Taggings) != 2 || docs[0].Taggings[0].Position != 1 || docs[0].Taggings[0].Tag == nil || docs[0].Taggings[0].Tag.Name != "go" {
			t.Fatalf("join rows with tags: %+v", docs[0].Taggings)
		}
		if len(docs[1].Taggings) != 0 {
			t.Fatal("document 2 has no taggings")
		}

		emps, err := momo.Select[models.Employee, *models.Employee](db).OrderBy(employee.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := momo.Load(ctx, db, emps, employee.Reports); err != nil {
			t.Fatal(err)
		}
		if len(emps[0].Reports) != 2 {
			t.Fatalf("self-reference back-reference: %+v", emps[0].Reports)
		}
	})
}
```

Add import `"github.com/azrtydxb/momo/internal/relfixture/tagging"`.

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestPolymorphicAndThrough$' -count=1` fails at `document images` if `Discriminator` is not applied, with `document images: []` or with extra images mixed in (`len == 2`). Failure text names the assertion; the red state is the mixed result when the discriminator predicate is absent.

- [ ] **Step 3: Check the discriminator predicate.** Run `go test ./ -run '^TestPolymorphicAndThrough$' -count=1 -v`; expect `ok`. The discriminator comes from `writeScope` in Task 5, which appends `"images"."owner_type" = ?` before user predicates. Self-reference and join-model paths use the Task 5 HasMany and BelongsTo branches unchanged.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestPolymorphicAndThrough$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add load_test.go && git commit -m "test(relations): polymorphic, self-referencing and join-model relations"`.

Reconciliation note (2026-11-06): Task 10 needed no production change; the discriminator, back-reference, self-reference and join-model paths were already served by the Task 5, 6 and 9 code. Two deviations from the sketch: the nested join-model load is `momo.LoadAll(ctx, db, docs, document.Taggings.Then(tagging.Tag))`, because `Then` returns a `Loader` (the plan's `momo.Load(..., rel.Then(...))` does not compile), and the test also asserts the second join row and the self-referencing belongs-to (`employee.Manager`). The added `TestPolymorphicChunkingKeepsDiscriminator` lowers the limit with `WithMaxParams` and pins that the discriminator bind is budgeted per statement. Red state: with the discriminator predicate removed, `document images` contains the author's image.

---

## Task 11: Nested, filtered, ordered eager loading

Files:

- Modify `load_test.go`

Interfaces:

- Uses `document.Comments.Scope(...)`, `document.Taggings.Then(...)`, `LoadWhere`, `LoadOrderBy`, `Then`.

Covers: S-81 (nested, filtered, ordered, limited eager loading). Acceptance test: `TestNestedFilteredEagerLoad`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

```go
func TestNestedFilteredEagerLoad(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		docs := allDocs(t, db)
		h.After = nil

		// Approved comments only, newest id first, nested author of the document's comments' ... :
		// comments is the level; document.Comments scoped; the nested level loads the tags of
		// the documents through Taggings and tagging.Tag in a single pass.
		loaders := []momo.Loader[models.Document]{
			document.Comments.Scope(
				momo.LoadWhere[models.Comment](comment.Approved.Eq(true)),
				momo.LoadOrderBy[models.Comment](comment.ID.Desc()),
			),
			document.Taggings.Then(tagging.Tag),
		}
		if err := momo.LoadAll(ctx, db, docs, loaders...); err != nil {
			t.Fatal(err)
		}
		// Two top-level statements (comments, taggings) and one nested statement (tags).
		if len(h.After) != 3 {
			t.Fatalf("want 3 statements (comments, taggings, nested tags), got %d: %v", len(h.After), h.After)
		}
		var ids []int64
		for _, c := range docs[0].Comments {
			ids = append(ids, c.ID)
		}
		if fmt.Sprint(ids) != "[3 1]" {
			t.Fatalf("approved comments newest-first: %v", ids)
		}
		if docs[0].Taggings[1].Tag == nil || docs[0].Taggings[1].Tag.Name != "db" {
			t.Fatal("nested tag not loaded")
		}
	})
}
```

Add import `"github.com/azrtydxb/momo/internal/relfixture/comment"`.

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestNestedFilteredEagerLoad$' -count=1` fails with `undefined: momo.Loader` only if Task 5 is not present; with Task 5 present it fails on the assertion `approved comments newest-first` if `LoadOrderBy` or `LoadWhere` is ignored, or on the statement count if nested loads run per parent.

- [ ] **Step 3: Check the nested statement count.** Run `go test ./ -run '^TestNestedFilteredEagerLoad$' -count=1 -v`; expect `ok` with three statements. The nested tag load runs once in `pending.finish` (Task 5 code) through `LoadAll(ctx, pd.q, pd.pool, pd.nested...)`, so the count does not grow with parents.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestNestedFilteredEagerLoad$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add load_test.go && git commit -m "test(relations): nested, filtered and ordered eager loading"`.

Reconciliation note (2026-11-06): Task 11 needed no production change; `finish` already runs nested loads once over the pooled children, and `LoadWhere`/`LoadOrderBy` were already applied in `childQueries`. The red state is mutation-based: skipping the nested load, dropping the where, dropping the order, or loading nested relations for only the first pooled child each fail `TestNestedFilteredEagerLoad`. Beyond the plan's test, `load_nested_test.go` adds `TestNestedLoadDepthAndSharedChildren` (three levels, one statement per level, a child shared by two parents) and `TestNestedLoadFilterAtLeaf` (a nested scope does not leak to the level above). Its helper is named `commentIDs`, not `ids`, so Task 12's `ids` helper in `load_test.go` does not collide.

---

## Task 12: Top-N per parent with a window function

Files:

- Modify `load.go` (top-N branch in `childQueries`)
- Modify `load_test.go`

Interfaces:

- Uses the option `LoadPerParentLimit` with a positive count. It is valid for HasMany and ManyToMany only; BelongsTo and HasOne return an `ErrUnsupported` error.

Covers: S-33 (top-N-per-parent via window functions; explicit second strategy). Acceptance test: `TestEagerLoadTopNPerParent`.

- [ ] **Step 1: Write the failing test** — append to `load_test.go`:

Append the `ids` helper and this test to `load_test.go`. Add the imports `fmt` and `comment` to its import block.

```go
func ids(cs []models.Comment) []int64 {
	out := []int64{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func TestEagerLoadTopNPerParent(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		docs := allDocs(t, db)
		h.After = nil
		err := momo.LoadAll(context.Background(), db, docs, document.Comments.Scope(
			momo.LoadPerParentLimit[models.Comment](2),
			momo.LoadOrderBy[models.Comment](comment.ID.Asc()),
		))
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("top-N must be one statement per chunk, got %d", len(h.After))
		}
		if fmt.Sprint(ids(docs[0].Comments)) != "[1 2]" {
			t.Fatalf("doc 1 top-2: got %v want [1 2]", ids(docs[0].Comments))
		}
		if len(docs[2].Comments) != 1 || docs[2].Comments[0].ID != 4 {
			t.Fatalf("doc 3 must keep its only comment: %+v", docs[2].Comments)
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestEagerLoadTopNPerParent$' -count=1` fails with `top-2 per parent: doc1=[1 2 3] doc3=1` because `LoadPerParentLimit` is currently ignored by `childQueries`.

- [ ] **Step 3: Implement** — in `childQueries`, when `cfg.perParentLimit > 0`, build the window statement. Reject BelongsTo and HasOne with `ErrUnsupported`. Use the statement shape below; the outer statement keeps only the child columns and key holders, filtered by the row number:

```go
// topNQuery:
//   SELECT momo_t.cols..., momo_t.momo_key_i... FROM (
//     SELECT child.cols..., key_cols AS momo_key_i,
//            ROW_NUMBER() OVER (PARTITION BY partition_cols ORDER BY user_order, child.childcols, child.pk) AS momo_rn
//     FROM child [INNER JOIN jt ...] WHERE keys AND scope
//   ) AS momo_t WHERE momo_t.momo_rn <= ? ORDER BY momo_t.momo_rn
// partition_cols: r.ChildCols for HasMany; r.Through.ParentCols for ManyToMany.
// The row-number expression is written with TypedSQL-style raw SQL and quoted identifiers via Builder.Ident.
func (r *Relation[P, C]) topNQuery(d *dialect.Dialect, chunk [][]any, cfg *loadConfig) (Compiled, error) {
	partition := r.ChildCols
	qual := r.Child.Qualifier()
	if r.Through != nil {
		partition = r.Through.ParentCols
		qual = r.Through.Table.Qualifier()
	}
	return compile(d, func(b *Builder) {
		b.WriteString("SELECT ")
		for i, col := range r.ChildMeta.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent("momo_t", col.Name)
		}
		if r.Through != nil {
			for i := range r.Through.ParentCols {
				b.WriteString(", ")
				b.QualifiedIdent("momo_t", "momo_key_"+strconv.Itoa(i))
			}
		}
		b.WriteString(" FROM (SELECT ")
		r.writeChildColumns(b)
		if r.Through != nil {
			for i, c := range r.Through.ParentCols {
				b.WriteString(", ")
				b.QualifiedIdent(qual, c)
				b.WriteString(" AS ")
				b.Ident("momo_key_" + strconv.Itoa(i))
			}
		}
		b.WriteString(", ROW_NUMBER() OVER (PARTITION BY ")
		for i, c := range partition {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(qual, c)
		}
		b.WriteString(" ORDER BY ")
		r.writeOrder(b, cfg)
		b.WriteString(") AS ")
		b.Ident("momo_rn")
		b.WriteString(" FROM ")
		r.Child.appendFrom(b)
		if r.Through != nil {
			b.WriteString(" INNER JOIN ")
			r.Through.Table.appendFrom(b)
			b.WriteString(" ON ")
			for i, c := range r.Through.ChildCols {
				if i > 0 {
					b.WriteString(" AND ")
				}
				b.QualifiedIdent(r.Through.Table.Qualifier(), c)
				b.WriteString(" = ")
				b.QualifiedIdent(r.Child.Qualifier(), r.ChildCols[i])
			}
		}
		b.WriteString(" WHERE ")
		inTuple{table: qual, cols: partition, keys: chunk}.AppendSQL(b)
		r.writeScope(b, cfg)
		b.WriteString(") AS momo_t WHERE ")
		b.QualifiedIdent("momo_t", "momo_rn")
		b.WriteString(" <= ")
		b.Arg(int64(cfg.perParentLimit))
		b.WriteString(" ORDER BY ")
		b.QualifiedIdent("momo_t", "momo_rn")
	})
}
```

Add the check at the top of `childQueries`: `if cfg.perParentLimit > 0 && (r.Kind == BelongsTo || r.Kind == HasOne) { return nil, &Error{Kind: ErrUnsupported, Op: "load", Table: r.Child.Name, Err: errors.New(r.Name + ": per-parent limit applies to has-many and many-to-many")} }`. Then, when the limit is set, call `topNQuery` instead of the plain/M2M builder. The `consume` method for the M2M holder path already handles the projected `momo_key_i` columns; the plain HasMany path projects only child columns, which `consume` scans through `scanChild`.

Note: the outer `ORDER BY momo_rn` alone yields per-parent order because the consume step groups by parent in pool order; the window `ORDER BY` supplies the per-parent sequence.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestEagerLoadTopNPerParent$' -count=1` prints `ok`. Also run `go test ./ -run 'TestEagerLoadTopNPerParent' -count=1 -v` and confirm one child statement.

- [ ] **Step 5: Commit** — `git add load.go load_test.go && git commit -m "feat(relations): top-N per parent via ROW_NUMBER window strategy"`.

Reconciliation note (2026-11-06): Task 12 differs from the sketch in six ways. (1) The guard in `prepareWith` now rejects only BelongsTo/HasOne (`ErrUnsupported`) and a negative limit (`ErrValidation`); zero means unlimited. (2) The sketch's `qual`/`partition` are kept, but many-to-many with a child primary key ranks with `DENSE_RANK()` instead of `ROW_NUMBER()`, so duplicated join rows share a rank and cannot use up the limit before `consume` collapses them. (3) The outer `ORDER BY` is `momo_rn` then the partition columns, so the pool order is deterministic across parents. (4) The limit bind is counted in the scope-argument reserve. (5) The plan's `ids` helper and test live in `load_topn_test.go` (not `load_test.go`, which story 148 edits in parallel); `export_test.go` adds `ScopeThen`, because `Scope` and `Then` cannot be combined through the public API, to test nested loads over kept children. (6) `TestLoadPerParentLimitReturnsUnsupported` (load_review_test.go) and the internal "not supported yet" test are replaced by to-one/negative rejection tests.

---

## Task 13: Preload on SelectQuery

Files:

- Create `preload.go` (`Preload`, `runPreloads`)
- Modify `select.go`: add `preloads []any` to `selectCore`; call `sq.runPreloads(ctx, rows)` at the end of `All` and `One` (after rows are collected, before return).
- Create `preload_test.go` (package `momo_test`)

Interfaces:

- `func (sq SelectQuery[T]) Preload(loaders ...Loader[T]) SelectQuery[T]`
- `func (sq SelectQuery[T]) runPreloads(ctx context.Context, rows []T) error`

Covers: S-81 (preloads attached to the query). Acceptance test: `TestPreloadOnQuery` (own name; S-81 query integration).

- [ ] **Step 1: Write the failing test** — create `preload_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func TestPreloadOnQuery(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		base := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc())
		derived := base.Preload(document.Author, document.Comments)
		// The base query is immutable: it must not gain preloads.
		h.After = nil
		docs, err := base.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if docs[0].Author != nil || len(h.After) != 1 {
			t.Fatalf("base query was mutated or preloaded: author=%v statements=%d", docs[0].Author, len(h.After))
		}
		h.After = nil
		docs, err = derived.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" || len(docs[0].Comments) != 3 {
			t.Fatalf("preloads not applied: %+v", docs[0])
		}
		if len(h.After) != 3 {
			t.Fatalf("want parent + 2 relation statements, got %d: %v", len(h.After), h.After)
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestPreloadOnQuery$' -count=1` fails with `base.Preload undefined`.

- [ ] **Step 3: Implement** — create `preload.go`:

```go
package momo

import (
	"context"
	"slices"
)

// Preload returns a copy of the query that loads the given relations for every
// row it returns. The receiver is unchanged.
func (sq SelectQuery[T]) Preload(loaders ...Loader[T]) SelectQuery[T] {
	add := make([]any, len(loaders))
	for i, l := range loaders {
		add[i] = l
	}
	sq.s.preloads = append(slices.Clip(sq.s.preloads), add...)
	return sq
}

// runPreloads loads all preloaded relations for rows in one LoadAll call, so
// that the batched path (T14) applies.
func (sq SelectQuery[T]) runPreloads(ctx context.Context, rows []T) error {
	if len(sq.s.preloads) == 0 || len(rows) == 0 {
		return nil
	}
	loaders := make([]Loader[T], len(sq.s.preloads))
	for i, p := range sq.s.preloads {
		loaders[i] = p.(Loader[T])
	}
	return LoadAll(ctx, sq.q, rows, loaders...)
}
```

In `select.go`, add the field `preloads []any` to `selectCore` (the struct at `select.go:34`), and in `All` (around `select.go:404`) and `One` (around `select.go:429`) call `if err := sq.runPreloads(ctx, out); err != nil { return nil, err }` before the final return, where `out` is the collected slice. `slices.Clip` makes the append copy-on-write.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestPreloadOnQuery$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add preload.go preload_test.go select.go && git commit -m "feat(relations): Preload on SelectQuery"`.

Reconciliation note (2026-11-06): Task 13 shipped as sketched, with three additions. (1) `All` and `One` close the cursor before running the preloads, so the relation statements never run while the parent cursor still holds the connection (single-connection and transaction safety); `One` preloads only after the single-row and too-many-rows checks pass. (2) `Each` and `Iter` return an `ErrUnsupported` error (`errStreamPreload`, before any statement runs) when the query has preloads, because streaming cannot batch the relation load; Task 19 keeps this behaviour for the non-joined preloads too. (3) The guard on an empty result in `runPreloads` is redundant with `LoadAll` (which returns early for no parents) but kept for clarity. Extra tests: `TestPreloadDerivedQueriesAreIndependent` (copy-on-write with spare capacity), `TestPreloadFirstOneAndEmpty`, `TestPreloadInTransactionAndEvents`, `TestPreloadErrorPropagates`, `TestPreloadStreamingUnsupported`. README section 6 now describes `Preload` in prose (its Go snippets must stay verbatim in `examples/quickstart`).

---

## Task 14: Pipelined relation loading over pgx batching

Files:

- Modify `pending` in `load.go` (`runBatch` path) and `loader.go` (`LoadAll` chooses the batch path)
- Modify `exec.go` (create): `execer.batch(ctx, rel string, table string, cs []Compiled) ([]Rows, error)` firing one before/after hook pair per statement
- Create `internal/relfixture/count.go` (`Counting` driver wrapper)
- Create `pipeline_test.go` (package `momo_test`, PostgreSQL only)

Interfaces:

- `type Batcher interface { SendBatch(ctx context.Context, qs []Compiled) ([]Rows, error) }` (contract, implemented by `pgxdriver`, momo-03).
- `func Counting(inner momo.Driver) (momo.Driver, *Counter)` in `internal/relfixture`; `type Counter struct{ Queries, Execs, Batches atomic.Int64 }`. The returned driver implements `Batcher` only when `inner` does.

Covers: S-51 (pipelined loading via a single round trip). Acceptance test: `TestPipelinedEagerLoad`.

- [ ] **Step 1: Write the failing test** — create `pipeline_test.go`:

```go
package momo_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func TestPipelinedEagerLoad(t *testing.T) {
	ctx := context.Background()
	drv, ctr := relfixture.Counting(relfixture.PostgresDriver(t))
	db := momo.New(drv)
	relfixture.CreateSchema(t, db)
	relfixture.SeedBasic(t, db)

	seqDocs, err := momo.Select[models.Document, *models.Document](db).OrderBy(document.ID.Asc()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Sequential reference: two relations, one statement each.
	ctr.Queries.Store(0)
	ctr.Batches.Store(0)
	if err := momo.LoadAll(ctx, db, seqDocs, document.Author, document.Comments); err != nil {
		t.Fatal(err)
	}
	if ctr.Batches.Load() != 1 || ctr.Queries.Load() != 0 {
		t.Fatalf("two relations must be one batch and no single statements: batches=%d queries=%d",
			ctr.Batches.Load(), ctr.Queries.Load())
	}
	got := fmt.Sprintf("%s %d", seqDocs[0].Author.Name, len(seqDocs[0].Comments))
	if got != "Ada 3" {
		t.Fatalf("batched result differs: %s", got)
	}
}
```

This test is PostgreSQL-only: `relfixture.PostgresDriver(t)` skips when `MOMO_TEST_POSTGRES_URL` is unset. Run it in CI with the variable set.

- [ ] **Step 2: Run and expect FAIL** — with `MOMO_TEST_POSTGRES_URL` set: `go test ./ -run '^TestPipelinedEagerLoad$' -count=1` fails with `two relations must be one batch and no single statements: batches=0 queries=2`. Without the variable the test is skipped and the step does not count as red: run it in CI, or locally against `make db-up`.

- [ ] **Step 3: Implement** — create `internal/relfixture/count.go`:

```go
package relfixture

import (
	"context"
	"sync/atomic"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
)

// Counter counts driver-level round trips.
type Counter struct {
	Queries atomic.Int64
	Execs   atomic.Int64
	Batches atomic.Int64
}

type countDriver struct {
	inner momo.Driver
	c     *Counter
}

func (d *countDriver) Dialect() *dialect.Dialect { return d.inner.Dialect() }

func (d *countDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	d.c.Queries.Add(1)
	return d.inner.Query(ctx, sql, args)
}

func (d *countDriver) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	d.c.Execs.Add(1)
	return d.inner.Exec(ctx, sql, args)
}

func (d *countDriver) Begin(ctx context.Context, opts momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.inner.Begin(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &countTx{inner: tx, c: d.c}, nil
}

type countTx struct {
	inner momo.DriverTx
	c     *Counter
}

func (t *countTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	t.c.Queries.Add(1)
	return t.inner.Query(ctx, sql, args)
}

func (t *countTx) Exec(ctx context.Context, sql string, args []any) (int64, error) {
	t.c.Execs.Add(1)
	return t.inner.Exec(ctx, sql, args)
}

func (t *countTx) Commit(ctx context.Context) error   { return t.inner.Commit(ctx) }
func (t *countTx) Rollback(ctx context.Context) error { return t.inner.Rollback(ctx) }

type countBatchDriver struct {
	*countDriver
}

func (d countBatchDriver) SendBatch(ctx context.Context, qs []momo.Compiled) ([]momo.Rows, error) {
	d.c.Batches.Add(1)
	return d.inner.(momo.Batcher).SendBatch(ctx, qs)
}

// Counting wraps inner and counts statements. The returned driver implements
// momo.Batcher only when inner does, so batched and sequential paths stay selectable.
func Counting(inner momo.Driver) (momo.Driver, *Counter) {
	c := &Counter{}
	cd := &countDriver{inner: inner, c: c}
	if _, ok := inner.(momo.Batcher); ok {
		return countBatchDriver{cd}, c
	}
	return cd, c
}
```

Create `exec.go` with the batched statement runner. It fires one before/after hook pair per statement, as `execer.query` does (`db.go:175`):

```go
package momo

import (
	"context"
	"errors"
)

// stmt is one compiled statement tagged with its relation for hook events.
type stmt struct {
	rel, table string
	c          Compiled
}

// batch sends every statement in one driver round trip (Batcher) and returns
// one Rows per statement, in order. Each statement gets its own hook pair.
func (e *execer) batch(ctx context.Context, stmts []stmt) ([]Rows, error) {
	b, ok := e.q.(Batcher)
	if !ok {
		return nil, &Error{Kind: ErrUnsupported, Op: "batch", Err: errors.New("driver does not implement Batcher")}
	}
	cs := make([]Compiled, len(stmts))
	ctxs := make([]context.Context, len(stmts))
	evs := make([]*QueryEvent, len(stmts))
	for i, s := range stmts {
		cs[i] = s.c
		ctxs[i], evs[i] = e.before(ctx, "select", s.table, s.rel, s.c.SQL, s.c.Args)
	}
	rows, err := b.SendBatch(ctx, cs)
	if err != nil {
		err = e.wrapErr("select", "", err)
		for i := range stmts {
			e.after(ctxs[i], evs[i], -1, err)
		}
		return nil, err
	}
	out := make([]Rows, len(rows))
	for i := range rows {
		out[i] = &hookedRows{Rows: rows[i], e: e, ctx: ctxs[i], ev: evs[i], op: "select", table: stmts[i].table}
	}
	return out, nil
}

func closeRows(rs []Rows) {
	for _, r := range rs {
		_ = r.Close()
	}
}
```

In `loader.go`, replace the `pendingLoad` interface with the full form. Batched loads need the statements, the names for hook events, and a way to consume rows before any nested query runs (a pgx pipeline must be drained in order before another statement is sent):

```go
type pendingLoad interface {
	run(ctx context.Context, q Querier) error
	statements() []Compiled
	target() (rel, table string)
	begin(ctx context.Context, q Querier) (context.Context, func(error))
	consumeRows(rows []Rows) error
	finish(ctx context.Context) error
}
```

In `load.go`, add these methods to `pending[P, C]`:

```go
func (pd *pending[P, C]) statements() []Compiled { return pd.queries }

func (pd *pending[P, C]) target() (string, string) {
	return pd.r.Parent.Name + "." + pd.r.Name, pd.r.Child.Name
}

// consumeRows reads this relation's result sets in statement order. Nested
// loads and assignment happen later, in finish.
func (pd *pending[P, C]) consumeRows(rows []Rows) error {
	for _, rs := range rows {
		if err := pd.consume(rs); err != nil {
			return err
		}
	}
	return nil
}
```

In `loader.go`, replace the `LoadAll` function with this version. Sequential loads stay the default; when the connection's driver implements `Batcher` and there is more than one load, all statements go in one batch:

```go
func LoadAll[P any](ctx context.Context, q Querier, parents []P, loaders ...Loader[P]) error {
	if len(parents) == 0 {
		return nil
	}
	pend := make([]pendingLoad, 0, len(loaders))
	for _, l := range loaders {
		p, err := l.prepare(q, parents)
		if err != nil {
			return err
		}
		pend = append(pend, p)
	}
	if _, ok := q.conn().q.(Batcher); ok && len(pend) > 1 {
		return runBatched(ctx, q, pend)
	}
	for _, p := range pend {
		if err := p.run(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

// runBatched sends the statements of every pending load in one batch, reads
// all result sets in order, and only then runs the nested loads and assignments.
// Relation events for batched loads start after their rows are read.
func runBatched(ctx context.Context, q Querier, pend []pendingLoad) error {
	var stmts []stmt
	counts := make([]int, len(pend))
	for i, p := range pend {
		rel, table := p.target()
		for _, c := range p.statements() {
			stmts = append(stmts, stmt{rel: rel, table: table, c: c})
		}
		counts[i] = len(p.statements())
	}
	rows, err := q.conn().batch(ctx, stmts)
	if err != nil {
		return err
	}
	off := 0
	for i, p := range pend {
		n := counts[i]
		if err := p.consumeRows(rows[off : off+n]); err != nil {
			closeRows(rows[off+n:])
			return err
		}
		off += n
	}
	for _, p := range pend {
		_, end := p.begin(ctx, q)
		err := p.finish(ctx)
		end(err)
		if err != nil {
			return err
		}
	}
	return nil
}
```

Run the batched path only on PostgreSQL: SQLite and the sequential fallback use `run`.

- [ ] **Step 4: Run and expect PASS** — `MOMO_TEST_POSTGRES_URL=... go test ./ -run '^TestPipelinedEagerLoad$' -count=1` prints `ok`. Also run the sequential suite on SQLite (`go test ./ -run 'TestEagerLoad' -count=1`) to confirm the non-batch path is unchanged.

- [ ] **Step 5: Commit** — `git add exec.go load.go loader.go internal/relfixture/count.go pipeline_test.go && git commit -m "feat(relations): pipelined relation loading via driver batches"`.

---

## Task 15: Single-query join loading for to-one relations with NULL-to-nil

Files:

- Create `joinload.go` (`Relation.Joined`, join rewrite, joined scan)
- Create `scan.go` (`ScanValue`)
- Create `scan_internal_test.go` (package `momo`) for `ScanValue`
- Modify `preload.go` (`Preload` splits joinable loaders into `sq.s.joinLoads`; `All` and `One` use the joined scan when present; `Each` and `Iter` return `ErrUnsupported` with joined loaders)
- Modify `select.go` only for the `joinLoads []any` field
- Modify `preload_test.go`

Interfaces:

- `func (r Relation[P, C]) Joined() Loader[P]` (BelongsTo only, because HasOne can match several children per parent and would duplicate parent rows; `ErrUnsupported` otherwise at `Preload` time).
- `func ScanValue(dst any, src any) error` — assigns a driver value to a field pointer: `*string`, `*int64`, `*int`, `*int32`, `*bool`, `*float64`, `*[]byte`, `*time.Time`, the same types behind `**T` (nil `src` sets the pointer to nil), and any `sql.Scanner`. A nil `src` into a non-pointer destination returns `&Error{Kind: ErrValidation, Op: "scan"}`.

Covers: S-81 (single-query join loading, NULL-to-nil). Acceptance test: `TestJoinLoadToOne`.

- [ ] **Step 1: Write the failing tests** — create `scan_internal_test.go`:

```go
package momo

import (
	"errors"
	"testing"
)

func TestScanValueNullHandling(t *testing.T) {
	var s *string
	if err := ScanValue(&s, nil); err != nil || s != nil {
		t.Fatalf("NULL into **string must set nil: err=%v s=%v", err, s)
	}
	var n int64
	if err := ScanValue(&n, nil); !errors.Is(err, ErrValidation) {
		t.Fatalf("NULL into non-nullable int64 must fail with ErrValidation, got %v", err)
	}
	var v int64
	if err := ScanValue(&v, int64(9)); err != nil || v != 9 {
		t.Fatalf("int64 round trip: %v %d", err, v)
	}
}
```

Append to `preload_test.go`:

```go
func TestJoinLoadToOne(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		h.After = nil
		docs, err := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.ID.Asc()).Preload(document.Author.Joined()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("join loading must be one statement, got %d", len(h.After))
		}
		if len(docs) != 4 {
			t.Fatalf("join must not duplicate or drop parents: %d", len(docs))
		}
		if docs[0].Author == nil || docs[0].Author.Name != "Ada" {
			t.Fatalf("doc 1 author: %+v", docs[0].Author)
		}
		if docs[3].Author != nil {
			t.Fatal("NULL foreign key through a LEFT JOIN must leave Author nil, not a zero struct")
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestScanValueNullHandling$' -count=1` fails with `undefined: ScanValue`; `go test ./ -run '^TestJoinLoadToOne$' -count=1` fails with `undefined: Joined` (or `Joined` returns a loader that is not joined: `len(h.After) == 2`).

- [ ] **Step 3: Implement** — create `scan.go` with a type switch over the listed destination types and the `sql.Scanner` fallback:

```go
package momo

import (
	"database/sql"
	"errors"
	"time"
)

// ScanValue assigns src to the field behind dst. The type switch is over
// concrete field types so the join path needs no reflection. Unknown types
// fall back to sql.Scanner; anything else is ErrValidation.
func ScanValue(dst any, src any) error {
	if src == nil {
		switch d := dst.(type) {
		case **string:
			*d = nil
			return nil
		case **int64:
			*d = nil
			return nil
		case **int:
			*d = nil
			return nil
		case **int32:
			*d = nil
			return nil
		case **bool:
			*d = nil
			return nil
		case **float64:
			*d = nil
			return nil
		case **time.Time:
			*d = nil
			return nil
		case *[]byte:
			*d = nil
			return nil
		}
		if s, ok := dst.(sql.Scanner); ok {
			return s.Scan(nil)
		}
		return &Error{Kind: ErrValidation, Op: "scan", Err: errors.New("NULL into a non-nullable field")}
	}
	switch d := dst.(type) {
	case *string:
		return convertTo(d, src, func(v any) (string, error) { return asString(v) })
	case **string:
		s, err := asString(src)
		*d = &s
		return err
	case *int64:
		v, err := asInt64(src)
		*d = v
		return err
	case **int64:
		v, err := asInt64(src)
		*d = &v
		return err
	case *int:
		v, err := asInt64(src)
		*d = int(v)
		return err
	case **int:
		v, err := asInt64(src)
		n := int(v)
		*d = &n
		return err
	case *int32:
		v, err := asInt64(src)
		*d = int32(v)
		return err
	case **int32:
		v, err := asInt64(src)
		n := int32(v)
		*d = &n
		return err
	case *bool:
		v, err := asBool(src)
		*d = v
		return err
	case **bool:
		v, err := asBool(src)
		*d = &v
		return err
	case *float64:
		v, err := asFloat64(src)
		*d = v
		return err
	case **float64:
		v, err := asFloat64(src)
		*d = &v
		return err
	case *[]byte:
		v, err := asBytes(src)
		*d = v
		return err
	case *time.Time:
		v, ok := src.(time.Time)
		if !ok {
			return &Error{Kind: ErrValidation, Op: "scan", Err: errors.New("expected time.Time")}
		}
		*d = v
		return nil
	case **time.Time:
		v, ok := src.(time.Time)
		if !ok {
			return &Error{Kind: ErrValidation, Op: "scan", Err: errors.New("expected time.Time")}
		}
		*d = &v
		return nil
	}
	if s, ok := dst.(sql.Scanner); ok {
		return s.Scan(src)
	}
	return &Error{Kind: ErrValidation, Op: "scan", Err: errors.New("unsupported destination type")}
}
```

Implement the helpers `asString`, `asInt64`, `asBool`, `asFloat64`, `asBytes` in the same file: each accepts the driver value types (`string`, `[]byte`, `int64`, `float64`, `bool`, `time.Time` where meaningful) and returns `&Error{Kind: ErrValidation, Op: "scan"}` for anything else. Remove the unused `convertTo` helper in the `*string` case and use `asString` directly.

Create `joinload.go`:

```go
package momo

import (
	"context"
	"errors"
)

// joinedLoader marks a to-one relation for single-query LEFT JOIN loading.
type joinedLoader[P, C any] struct{ rel Relation[P, C] }

func (r Relation[P, C]) Joined() Loader[P] { return joinedLoader[P, C]{r} }

func (j joinedLoader[P, C]) relationName() string { return j.rel.Name }

func (j joinedLoader[P, C]) prepare(q Querier, parents []P) (pendingLoad, error) {
	return nil, &Error{Kind: ErrUnsupported, Op: "load", Table: j.rel.Child.Name, Err: errors.New("joined loaders are applied by SelectQuery.Preload, not Load")}
}

func (j joinedLoader[P, C]) joinedRelation() *Relation[P, C] { return &j.rel }
```

Add the following to `joinload.go`. A joined loader adds a LEFT JOIN with alias `momo_j<idx>` and the child's columns to the select list, so one row carries the parent and its child. `Preload` replaces the Task 13 version with this one: joined loaders are routed to the join, the rest to `preloads`.

```go
// joinApplier is implemented by joined loaders. Preload uses it to add the join.
type joinApplier[T any] interface {
	applyJoin(sq SelectQuery[T], idx int) SelectQuery[T]
	width() int
	assign(p *T, vals []any) error
}

// applyJoin adds the LEFT JOIN and the child columns (aliased momo_j<idx>_<col>).
// Without an explicit select list the parent columns are selected first, so the
// scan order is parent columns, then one block per joined loader.
func (j joinedLoader[P, C]) applyJoin(sq SelectQuery[P], idx int) SelectQuery[P] {
	alias := "momo_j" + strconv.Itoa(idx)
	parent := sq.qualifier()
	on := make([]Predicate, 0, len(j.rel.ParentCols)+1)
	for i, pc := range j.rel.ParentCols {
		on = append(on, binOp{qcol{parent, pc}, " = ", qcol{alias, j.rel.ChildCols[i]}})
	}
	if d := j.rel.Discriminator; d != nil {
		on = append(on, binOp{qcol{alias, d.Column}, " = ", argExpr{d.Value}})
	}
	cols := append([]Expr(nil), sq.s.cols...)
	if len(cols) == 0 {
		for _, c := range sq.meta.Columns {
			cols = append(cols, qcol{parent, c.Name})
		}
	}
	for _, c := range j.rel.ChildMeta.Columns {
		cols = append(cols, As(qcol{alias, c.Name}, alias+"_"+c.Name))
	}
	return sq.LeftJoin(TableRef{Name: j.rel.Child.Name, Schema: j.rel.Child.Schema, Alias: alias}, on...).Columns(cols...)
}

// width is the number of child columns this join adds to each row.
func (j joinedLoader[P, C]) width() int { return len(j.rel.ChildMeta.Columns) }

// assign sets the parent's to-one field from one joined block. A NULL primary
// key means no child: the field becomes nil, never a zero struct.
func (j joinedLoader[P, C]) assign(p *P, vals []any) error {
	if vals[columnIndex(j.rel.ChildMeta, j.rel.ChildPK[0])] == nil {
		j.rel.SetOne(p, nil)
		return nil
	}
	var c C
	for i, v := range vals {
		if err := ScanValue(j.rel.ChildMeta.Ptr(&c, i), v); err != nil {
			return err
		}
	}
	j.rel.SetOne(p, &c)
	return nil
}

// columnIndex returns the index of a mapped column by database name, or -1.
func columnIndex[C any](m *Meta[C], name string) int {
	for i, col := range m.Columns {
		if col.Name == name {
			return i
		}
	}
	return -1
}
```

In `preload.go`, replace the Task 13 `Preload` with:

```go
func (sq SelectQuery[T]) Preload(loaders ...Loader[T]) SelectQuery[T] {
	for _, l := range loaders {
		if ja, ok := l.(joinApplier[T]); ok {
			sq = ja.applyJoin(sq, len(sq.s.joinLoads))
			sq.s.joinLoads = append(slices.Clip(sq.s.joinLoads), ja)
			continue
		}
		sq.s.preloads = append(slices.Clip(sq.s.preloads), l)
	}
	return sq
}

// allJoined runs the query when joined loaders are present: each row holds the
// parent columns, then one block of child columns per joined loader.
func (sq SelectQuery[T]) allJoined(ctx context.Context) ([]T, error) {
	rows, err := sq.rows(ctx)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		var p T
		dest := make([]any, 0, len(sq.meta.Columns))
		for i := range sq.meta.Columns {
			dest = append(dest, sq.meta.Ptr(&p, i))
		}
		blocks := make([][]any, len(sq.s.joinLoads))
		for k, l := range sq.s.joinLoads {
			ja := l.(joinApplier[T])
			blocks[k] = make([]any, ja.width())
			for i := range blocks[k] {
				dest = append(dest, &blocks[k][i])
			}
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		for k, l := range sq.s.joinLoads {
			if err := l.(joinApplier[T]).assign(&p, blocks[k]); err != nil {
				return nil, err
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

At the top of `All` in `select.go`, add the joined path (the remaining `preloads` run afterwards through `runPreloads`):

```go
if len(sq.s.joinLoads) > 0 {
	out, err := sq.allJoined(ctx)
	if err != nil {
		return nil, err
	}
	if err := sq.runPreloads(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}
```

`One` with joined loaders uses `Limit(2)` on the same path and applies the One rules (zero rows: ErrNotFound; two or more: ErrTooManyRows). `Each` and `Iter` return `ErrUnsupported` when `joinLoads` is non-empty.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run 'TestScanValueNullHandling|TestJoinLoadToOne' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add scan.go scan_internal_test.go joinload.go preload.go preload_test.go select.go && git commit -m "feat(relations): single-query join loading for to-one relations"`.

Reconciliation note (2026-11-06): Task 15 differs from the sketch in seven ways. (1) The join is not added by `Preload`: `Preload` only records joined loaders in `joinLoads`, and `expandJoins` (in `joinload.go`) builds the wider select list and the LEFT JOINs when the query is compiled (`SQL`) or run, so `Columns`, `Join` and `Where` added after `Preload` compose and `Preload` never touches its receiver. (2) There is no `allJoined`: `scanner` gained a `joinScan` that scans the child block into scratch values after the model's columns, so `All` and `One` keep their own row loops (`One` still reads at most two rows and has no `Limit(2)`). (3) Child columns are labelled `momo_j<n>_<column index>` (not `momo_j<n>_<name>`), so a long column name never hits the identifier length limit; the table alias is `momo_j<n>`. (4) A row has a child when a `ChildCols` column is non-NULL (not `ChildPK[0]`), which is correct for composite and unique-column keys. (5) Joined loading is refused with `ErrUnsupported` (Op `select`, at run time) for has-one, has-many and many-to-many, for a belongs-to whose `ChildCols` are neither the primary key nor a single unique column (the join could otherwise multiply parents), and for `GroupBy`, `Having`, row locking, `Each` and `Iter`; a select list that is not one expression per mapped field is `ErrValidation`. Passing a joined loader to `Load` or `LoadAll` is `ErrUnsupported`. (6) The discriminator condition in the sketch is dropped: `validate` already rejects a `Discriminator` on belongs-to, and has-one is not joinable. The soft-delete filter of the child is in ON, never WHERE. (7) A joined load runs no statement of its own, so it emits no `RelationEvent`; the parent statement reaches `Hook` as one query. `ScanValue` also accepts `*int16` and `*float32`. The tests live in `joinload_test.go` (not `preload_test.go`, which the Preload story edits in parallel); `TestNestedFilteredEagerLoad` is Task 11's and is unchanged.

Reconciliation note (2026-11-06, review fix): point (2) above changed. The child block is scanned into a scratch child through its own `Meta.Ptr` destinations (`joinBinding`), not into `*any` through `ScanValue`, so child columns have the same semantics as a plain Load of the child. Non-nullable destinations are scanned through `**T` and a NULL there is interpreted only once the key columns show a match (`scanNull`: `Scan(nil)` for a `sql.Scanner`, nil for slices/maps/interfaces, an error otherwise); an unmatched row leaves the relation nil and drops the scratch child. `ScanValue` stays as a public helper; `scanNull` now also accepts named slice, map and interface kinds. A raw unqualified column such as `id` in the parent's Where is ambiguous with a Joined relation (SQLSTATE 42702), as the `Joined` doc says.

---

## Task 16: Relation predicates, traversal and count ordering

Files:

- Create `relpred.go`
- Create `relpred_test.go` (package `momo_test`)

Interfaces:

- `func (r Relation[P, C]) Has(scope ...Predicate) Predicate` — `EXISTS (SELECT 1 FROM child AS "momo_r" WHERE correlation AND scope)`.
- `func (r Relation[P, C]) HasFrom(parentAlias string, scope ...Predicate) Predicate` — correlates on the given parent alias (required for self-referencing relations).
- `func (r Relation[P, C]) HasWith(scope ...Predicate) Predicate` — same as `Has`.
- `func (r Relation[P, C]) HasNo(scope ...Predicate) Predicate` — `NOT EXISTS (...)`.
- `func (r Relation[P, C]) From(parents SelectQuery[P]) SelectQuery[C]` — child rows whose key is in the parents' key set.
- `type Direction uint8` with constants `DirAsc`, `DirDesc` (not `Asc`/`Desc`: those names are taken by `Asc(e Expr)` and `Desc(e Expr)` in `expr.go`). `CountOrder(dir Direction) Orderable` renders `(SELECT COUNT(*) FROM child AS "momo_r" WHERE correlation) DESC`.

Scope predicates in `Has*` must reference the child alias `momo_r` (build them with `.As("momo_r")` on the generated column). Documented in the godoc of `Has`.

Covers: S-76 (EXISTS predicates, traversal to typed child query, ordering by relation count). Acceptance test names: `TestRelationPredicates`, `TestRelationTraverseAndOrder`.

- [ ] **Step 1: Write the failing tests** — create `relpred_test.go`:

```go
package momo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
)

func TestRelationPredicates(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		withApproved := momo.Select[models.Document, *models.Document](db).
			Where(document.Comments.Has(comment.Approved.As("momo_r").Eq(true))).OrderBy(document.ID.Asc())
		c, err := withApproved.SQL()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(c.SQL, "EXISTS (SELECT 1 FROM") || strings.Contains(c.SQL, "JOIN") {
			t.Fatalf("HasX must render as EXISTS without JOIN: %s", c.SQL)
		}
		docs, err := withApproved.All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(docs)) != "[1 3]" {
			t.Fatalf("documents with an approved comment: %v", ids(docs))
		}
		noComments, err := momo.Select[models.Document, *models.Document](db).
			Where(document.Comments.HasNo()).OrderBy(document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(noComments)) != "[2 4]" {
			t.Fatalf("documents without comments: %v", ids(noComments))
		}
	})
}

func docIDs(ds []models.Document) []int64 {
	out := []int64{}
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return out
}

func TestRelationTraverseAndOrder(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		parents := momo.Select[models.Document, *models.Document](db).Where(document.ID.Eq(1))
		comments, err := document.Comments.From(parents).OrderBy(comment.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(commentIDs(comments)) != "[1 2 3]" {
			t.Fatalf("traversal to typed child query: %v", commentIDs(comments))
		}
		ordered, err := momo.Select[models.Document, *models.Document](db).
			OrderBy(document.Comments.CountOrder(momo.DirDesc), document.ID.Asc()).All(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprint(docIDs(ordered)) != "[1 3 2 4]" {
			t.Fatalf("order by relation count desc: %v", docIDs(ordered))
		}
	})
}

func commentIDs(cs []models.Comment) []int64 {
	out := []int64{}
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run 'TestRelationPredicates|TestRelationTraverseAndOrder' -count=1` fails with `document.Comments.Has undefined`.

- [ ] **Step 3: Implement** — create `relpred.go`:

```go
package momo

import (
	"context"
	"errors"

	"github.com/azrtydxb/momo/dialect"
)

// Direction orders by relation count. DirAsc and DirDesc avoid the names
// Asc(e Expr) and Desc(e Expr) already used in expr.go.
type Direction uint8

const (
	DirAsc Direction = iota
	DirDesc
)

// relAlias is the child alias used inside EXISTS and COUNT subqueries.
const relAlias = "momo_r"

// correlated renders EXISTS / NOT EXISTS / COUNT over the child table,
// correlated with the parent through the relation's key columns.
type correlated struct {
	r      *relCorr
	scope  []Predicate
	count  bool
	negate bool
}

// relCorr is the non-generic part of a relation used by correlated predicates.
type relCorr struct {
	parentQual string
	child      TableRef
	parentCols []string
	childCols  []string
	through    *Through
	disc       *Discriminator
}

func (x correlated) AppendSQL(b *Builder) {
	if x.count {
		b.WriteString("(SELECT COUNT(*) FROM ")
	} else {
		if x.negate {
			b.WriteString("NOT ")
		}
		b.WriteString("EXISTS (SELECT 1 FROM ")
	}
	child := TableRef{Name: x.r.child.Name, Schema: x.r.child.Schema, Alias: relAlias}
	if x.r.through != nil {
		b.WriteString(" ")
		x.r.through.Table.appendFrom(b)
		b.WriteString(" AS ")
		b.Ident("momo_j")
		b.WriteString(" INNER JOIN ")
		child.appendFrom(b)
		b.WriteString(" ON ")
		for i, c := range x.r.through.ChildCols {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(relAlias, x.r.childCols[i])
			b.WriteString(" = ")
			b.QualifiedIdent("momo_j", c)
		}
		b.WriteString(" WHERE ")
		for i, c := range x.r.through.ParentCols {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent("momo_j", c)
			b.WriteString(" = ")
			b.QualifiedIdent(x.r.parentQual, x.r.parentCols[i])
		}
	} else {
		child.appendFrom(b)
		b.WriteString(" WHERE ")
		for i, c := range x.r.childCols {
			if i > 0 {
				b.WriteString(" AND ")
			}
			b.QualifiedIdent(relAlias, c)
			b.WriteString(" = ")
			b.QualifiedIdent(x.r.parentQual, x.r.parentCols[i])
		}
	}
	if x.r.disc != nil {
		b.WriteString(" AND ")
		binOp{qcol{relAlias, x.r.disc.Column}, " = ", argExpr{x.r.disc.Value}}.AppendSQL(b)
	}
	for _, p := range x.scope {
		b.WriteString(" AND ")
		p.AppendSQL(b)
	}
	if x.count {
		b.WriteString(")")
		return
	}
	b.WriteString(")")
}

func (correlated) resultType() bool { return false }

// Has returns EXISTS over the child table. Scope predicates must reference the
// child through the alias "momo_r" (for example comment.Approved.As("momo_r")).
func (r Relation[P, C]) Has(scope ...Predicate) Predicate {
	return correlated{r: r.corr(r.Parent.Qualifier()), scope: scope}
}

// HasWith is an alias of Has.
func (r Relation[P, C]) HasWith(scope ...Predicate) Predicate { return r.Has(scope...) }

// HasFrom is Has with an explicit parent alias. Self-referencing relations need
// it: the parent table is aliased in the outer query and passed here.
func (r Relation[P, C]) HasFrom(parentAlias string, scope ...Predicate) Predicate {
	return correlated{r: r.corr(parentAlias), scope: scope}
}

// HasNo returns NOT EXISTS over the child table.
func (r Relation[P, C]) HasNo(scope ...Predicate) Predicate {
	return correlated{r: r.corr(r.Parent.Qualifier()), scope: scope, negate: true}
}

// CountOrder orders parents by the number of related children.
func (r Relation[P, C]) CountOrder(dir Direction) Orderable {
	o := Order{Expr: correlated{r: r.corr(r.Parent.Qualifier()), count: true}}
	if dir == DirDesc {
		o.Desc = true
	}
	return o
}

func (r Relation[P, C]) corr(parentQual string) *relCorr {
	return &relCorr{parentQual: parentQual, child: r.Child, parentCols: r.ParentCols, childCols: r.ChildCols, through: r.Through, disc: r.Discriminator}
}

// From returns the children whose key is in the parents' key set. The parents
// query is a subquery, so no parent row is fetched.
func (r Relation[P, C]) From(parents SelectQuery[P]) SelectQuery[C] {
	keyCols := make([]Expr, len(r.ParentCols))
	for i, c := range r.ParentCols {
		keyCols[i] = qcol{r.Parent.Qualifier(), c}
	}
	sub := parents.Columns(keyCols...)
	q := selectFrom(parents.q, r.ChildMeta)
	return q.Where(tupleInSubquery{table: r.Child.Qualifier(), cols: r.ChildCols, sub: sub})
}

// tupleInSubquery renders (cols) IN (subquery).
type tupleInSubquery struct {
	table string
	cols  []string
	sub   Subquery
}

func (x tupleInSubquery) AppendSQL(b *Builder) {
	if len(x.cols) > 1 && b.Dialect().Name == dialect.SQLite {
		b.WriteString("(")
		for i, c := range x.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(x.table, c)
		}
		b.WriteString(") IN (")
		x.sub.AppendSQL(b)
		b.WriteString(")")
		return
	}
	if len(x.cols) == 1 {
		b.QualifiedIdent(x.table, x.cols[0])
	} else {
		b.WriteString("(")
		for i, c := range x.cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(x.table, c)
		}
		b.WriteString(")")
	}
	b.WriteString(" IN (")
	x.sub.AppendSQL(b)
	b.WriteString(")")
}

func (tupleInSubquery) resultType() bool { return false }

```

Remove the unused `errors` and `context` imports from `relpred.go` (the file imports only `github.com/azrtydxb/momo/dialect`). Add `selectFrom` in a new file `select_meta.go`:

```go
// selectFrom builds an unfiltered SelectQuery over a generic meta value.
func selectFrom[T any](q Querier, meta *Meta[T]) SelectQuery[T] {
	return SelectQuery[T]{q: q, meta: meta, s: selectCore{limit: -1}}
}
```

Remove the two placeholder `var _` lines in step 3 before running the build; they exist only to keep the import list valid while the file is drafted. The `errors` and `context` imports must be removed if unused.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run 'TestRelationPredicates|TestRelationTraverseAndOrder' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add relpred.go relpred_test.go select_meta.go && git commit -m "feat(relations): EXISTS predicates, traversal and relation-count ordering"`.

Reconciliation note (2026-11-06): the shipped `relpred.go` differs from the draft above. `selectFrom` takes the child `TableRef` as a third argument and sets it as FROM (the draft left FROM empty). The correlated subquery renders inside `Builder.withScope([momo_r, momo_j])`, so a scope predicate that names another table fails the build instead of silently correlating. Scope predicates are wrapped in parentheses, so an `Or` scope cannot escape the correlation. The child's soft-delete marker and a polymorphic discriminator are applied inside `Has`, `HasNo` and `CountOrder`, and `From` also applies the discriminator. `From` builds the parent key subquery with an internal `selectCore.keysOnly` flag that skips the "select list matches model fields" arity check, because the key list is not the model's column list; `From` on a many-to-many relation returns a query that fails with `ErrValidation` (the traversal needs a join table, which `tupleInSubquery` does not model). `tupleInSubquery` writes the same `(cols) IN (subquery)` on every dialect, since `Subquery.AppendSQL` already adds the parentheses. The plan's test used an undefined `ids(...)` helper; it is `docIDs(...)`. Extra tests: `TestRelationPredicateShapes` (self-reference with `HasFrom`, composite key, polymorphic discriminator, composite traversal), count ordering ascending, `HasNo` and many-to-many SQL shape.

---

## Task 17: Association helpers Link, Unlink, Replace and Count

Files:

- Create `assoc.go`
- Create `assoc_test.go` (package `momo_test`)

Interfaces:

- `func (r Relation[P, C]) Link(ctx context.Context, q Querier, p *P, cs ...*C) error`
- `func (r Relation[P, C]) Unlink(ctx context.Context, q Querier, p *P, cs ...*C) error`
- `func (r Relation[P, C]) Replace(ctx context.Context, q Querier, p *P, cs ...*C) error`
- `func (r Relation[P, C]) Count(ctx context.Context, q Querier, p *P) (int64, error)`

Statement contract (documented in godoc):

- HasOne/HasMany `Link`: one `UPDATE child SET fk = ? WHERE pk IN (...)`.
- HasOne/HasMany `Unlink`: one `UPDATE child SET fk = NULL WHERE fk = ? AND pk IN (...)`.
- ManyToMany `Link`: one `INSERT INTO jt (parent cols, child cols) VALUES (...), ... ON CONFLICT DO NOTHING` (MySQL: `ON DUPLICATE KEY UPDATE` with a no-op assignment of the first column to itself).
- ManyToMany `Unlink`: one `DELETE FROM jt WHERE parent cols = ? AND (child cols) IN (...)`.
- `Replace`: one transaction issuing two statements: (1) clear the associations not in the new set; (2) link the new set (skipped when the set is empty). When `q` is a `*DB` the helper opens a transaction; when it is a `*Tx` it uses a savepoint (`tx.Tx`).
- `Count`: one `SELECT COUNT(*)`.
- `Unlink` and `Count` on BelongsTo return `ErrUnsupported`; `Link` and `Replace` on BelongsTo return `ErrUnsupported` (set the foreign key on the parent instead).

Covers: S-83. Acceptance test: `TestAssociationHelpers`.

- [ ] **Step 1: Write the failing test** — create `assoc_test.go`:

```go
package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/employee"
)

func TestAssociationHelpers(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		// Tags of document 2: none. Link tag 1 (one statement).
		p := &models.Document{ID: 2}
		tag := &models.Tag{ID: 1}
		h.After = nil
		if err := document.Tags.Link(ctx, db, p, tag); err != nil {
			t.Fatal(err)
		}
		if len(h.After) != 1 {
			t.Fatalf("Link must issue exactly one statement, got %d", len(h.After))
		}
		n, err := document.Tags.Count(ctx, db, p)
		if err != nil || n != 1 {
			t.Fatalf("Count after Link: n=%d err=%v", n, err)
		}

		// Replace with {2}: tag 1 removed, tag 2 added, inside one transaction.
		h.After = nil
		if err := document.Tags.Replace(ctx, db, p, &models.Tag{ID: 2}); err != nil {
			t.Fatal(err)
		}
		n, err = document.Tags.Count(ctx, db, p)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("Replace must leave exactly the new set: count=%d", n)
		}

		boss := &models.Employee{ID: 1}
		if err := employee.Reports.Unlink(ctx, db, boss, &models.Employee{ID: 2}); err != nil {
			t.Fatal(err)
		}
		n, err = employee.Reports.Count(ctx, db, boss)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("Unlink must NULL only the listed child FK: count=%d", n)
		}
	})
}
```

Replace the Unlink section of the test with the following. The `employee` package has a nullable foreign key (`employees.manager_id`), so Unlink can clear it:

```go
	boss := &models.Employee{ID: 1}
	if err := employee.Reports.Unlink(ctx, db, boss, &models.Employee{ID: 2}); err != nil {
		t.Fatal(err)
	}
	n, err = employee.Reports.Count(ctx, db, boss)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Unlink must NULL only the listed child FK: count=%d", n)
	}
```

and add the import `"github.com/azrtydxb/momo/internal/relfixture/employee"`; remove the `comment` import.

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run '^TestAssociationHelpers$' -count=1` fails with `document.Tags.Link undefined`.

- [ ] **Step 3: Implement** — create `assoc.go`. Each helper compiles one statement with the internal `Builder` (`compile` from `builder.go`) and runs it with `q.conn().exec(ctx, op, table, c)` (`db.go:163`). Key values come from `ParentKey(p)` and the child's `ChildMeta.Value` for the primary key (`ChildPK` names mapped to `Meta.Columns` indexes by name). The core helper signature:

```go
// pkIndexes maps ChildPK names to Meta column indexes. Missing names are an ErrValidation.
func pkIndexes[C any](m *Meta[C], names []string) ([]int, error) {
	out := make([]int, len(names))
	for i, n := range names {
		found := -1
		for j, col := range m.Columns {
			if col.Name == n {
				found = j
				break
			}
		}
		if found < 0 {
			return nil, &Error{Kind: ErrValidation, Op: "assoc", Err: errors.New("primary key column not in model: " + n)}
		}
		out[i] = found
	}
	return out, nil
}

// pkValues returns the primary key tuple of c, in ChildPK order.
func pkValues[C any](m *Meta[C], idx []int, c *C) []any {
	out := make([]any, len(idx))
	for i, j := range idx {
		out[i] = m.Value(c, j)
	}
	return out
}
```

`Link` (HasOne/HasMany): `UPDATE child SET fk_cols = parent key values WHERE child pk tuple IN (...)`; the SET values come from `ParentKey(p)`; the WHERE uses `inTuple{table: child, cols: ChildPK, keys: pkTuples}`. `Unlink` (HasOne/HasMany): `UPDATE child SET fk_cols = NULL WHERE fk_cols = parent key AND child pk tuple IN (...)`. `Link`/`Unlink` for ManyToMany use the join table `r.Through.Table` with `ParentKey` and `ChildKey` values. `Count` uses `SELECT COUNT(*) FROM child WHERE fk = parent` (HasX) or `FROM jt WHERE parent cols = ...` (M2M). `Replace` runs inside a transaction:

```go
func (r Relation[P, C]) Replace(ctx context.Context, q Querier, p *P, cs ...*C) error {
	fn := func(tx *Tx) error {
		if err := r.unlinkExcept(ctx, tx, p, cs); err != nil {
			return err
		}
		if len(cs) == 0 {
			return nil
		}
		return r.Link(ctx, tx, p, cs...)
	}
	switch qq := q.(type) {
	case *DB:
		return qq.Tx(ctx, TxOptions{}, fn)
	case *Tx:
		return qq.Tx(ctx, fn)
	}
	return &Error{Kind: ErrUnsupported, Op: "replace", Err: errors.New("unknown Querier")}
}
```

`unlinkExcept` issues `UPDATE child SET fk = NULL WHERE fk = parent AND NOT (child pk tuple IN (...))` (HasX) or `DELETE FROM jt WHERE parent cols = ... AND NOT ((child cols) IN (...))` (M2M). With an empty new set, `inTuple` renders `1 = 0`, so `NOT (1 = 0)` clears every association.

Add these to `assoc.go` (they complete the helper set; `pkIndexes` and `pkValues` are above):

```go
// Link sets the foreign key of the listed children (HasOne, HasMany: one UPDATE)
// or inserts join rows (ManyToMany: one INSERT). BelongsTo is set on the parent.
func (r Relation[P, C]) Link(ctx context.Context, q Querier, p *P, cs ...*C) error {
	if len(cs) == 0 {
		return nil
	}
	switch r.Kind {
	case HasOne, HasMany:
		return r.updateChildren(ctx, q, p, cs, linkSet)
	case ManyToMany:
		return r.insertJoinRows(ctx, q, p, cs)
	}
	return r.notLinkable("link")
}

// Unlink clears the foreign key of the listed children (one UPDATE) or deletes
// the listed join rows (ManyToMany: one DELETE).
func (r Relation[P, C]) Unlink(ctx context.Context, q Querier, p *P, cs ...*C) error {
	if len(cs) == 0 {
		return nil
	}
	switch r.Kind {
	case HasOne, HasMany:
		return r.updateChildren(ctx, q, p, cs, linkClear)
	case ManyToMany:
		return r.deleteJoinRows(ctx, q, p, cs, false)
	}
	return r.notLinkable("unlink")
}

// Count returns the number of related children (one SELECT COUNT(*)).
func (r Relation[P, C]) Count(ctx context.Context, q Querier, p *P) (int64, error) {
	key, ok := r.ParentKey(p)
	if !ok {
		return 0, nil
	}
	tr, cols := r.Child, r.ChildCols
	if r.Through != nil {
		tr, cols = r.Through.Table, r.Through.ParentCols
	}
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT COUNT(*) FROM ")
		tr.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tr.Qualifier(), cols: cols, keys: [][]any{key}}.AppendSQL(b)
		if r.Through == nil {
			r.writeScope(b, &loadConfig{})
		}
	})
	if err != nil {
		return 0, err
	}
	rows, err := q.conn().query(ctx, "select", tr.Name, "", c)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var n int64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	return n, rows.Err()
}

// unlinkExcept removes every association of p that is not in cs (one UPDATE or
// one DELETE). Replace calls it before linking the new set.
func (r Relation[P, C]) unlinkExcept(ctx context.Context, q Querier, p *P, cs []*C) error {
	switch r.Kind {
	case HasOne, HasMany:
		return r.updateChildren(ctx, q, p, cs, linkClearExcept)
	case ManyToMany:
		return r.deleteJoinRows(ctx, q, p, cs, true)
	}
	return r.notLinkable("replace")
}

type linkMode uint8

const (
	linkSet linkMode = iota
	linkClear
	linkClearExcept
)

func (r Relation[P, C]) notLinkable(op string) error {
	return &Error{Kind: ErrUnsupported, Op: op, Table: r.Child.Name,
		Err: errors.New(r.Name + ": " + op + " is not defined for " + r.Kind.String() + "; set the foreign key on the parent")}
}

// updateChildren writes the foreign key of cs (linkSet), clears it on cs
// (linkClear), or clears it on every child of p that is not in cs (linkClearExcept).
// Children are addressed by their primary key, read through ChildMeta.
func (r Relation[P, C]) updateChildren(ctx context.Context, q Querier, p *P, cs []*C, mode linkMode) error {
	key, ok := r.ParentKey(p)
	if !ok {
		return &Error{Kind: ErrValidation, Op: "assoc", Table: r.Child.Name, Err: errors.New(r.Name + ": parent key is NULL")}
	}
	idx, err := pkIndexes(r.ChildMeta, r.ChildPK)
	if err != nil {
		return err
	}
	keys := make([][]any, len(cs))
	for i, c := range cs {
		keys[i] = pkValues(r.ChildMeta, idx, c)
	}
	st, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("UPDATE ")
		r.Child.appendFrom(b)
		b.WriteString(" SET ")
		for i, col := range r.ChildCols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(col)
			b.WriteString(" = ")
			if mode == linkSet {
				b.Arg(key[i])
			} else {
				b.WriteString("NULL")
			}
		}
		if r.Discriminator != nil && mode == linkSet {
			b.WriteString(", ")
			b.Ident(r.Discriminator.Column)
			b.WriteString(" = ")
			b.Arg(r.Discriminator.Value)
		}
		b.WriteString(" WHERE ")
		if mode != linkSet {
			inTuple{table: r.Child.Name, cols: r.ChildCols, keys: [][]any{key}}.AppendSQL(b)
			b.WriteString(" AND ")
		}
		if mode == linkClearExcept {
			b.WriteString("NOT (")
		}
		inTuple{table: r.Child.Name, cols: r.ChildPK, keys: keys}.AppendSQL(b)
		if mode == linkClearExcept {
			b.WriteString(")")
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "update", r.Child.Name, st)
	return err
}

// insertJoinRows inserts one join row per child in one INSERT. Existing pairs are
// skipped with ON CONFLICT DO NOTHING, or with a no-op ON DUPLICATE KEY UPDATE on
// MySQL. A dialect with neither capability gets a plain INSERT.
func (r Relation[P, C]) insertJoinRows(ctx context.Context, q Querier, p *P, cs []*C) error {
	pkey, ok := r.ParentKey(p)
	if !ok {
		return &Error{Kind: ErrValidation, Op: "assoc", Table: r.Through.Table.Name, Err: errors.New(r.Name + ": parent key is NULL")}
	}
	ckeys := make([][]any, len(cs))
	for i, c := range cs {
		k, ok := r.ChildKey(c)
		if !ok {
			return &Error{Kind: ErrValidation, Op: "assoc", Table: r.Child.Name, Err: errors.New(r.Name + ": child key is NULL")}
		}
		ckeys[i] = k
	}
	d := q.Dialect()
	cols := append(append([]string{}, r.Through.ParentCols...), r.Through.ChildCols...)
	st, err := compile(d, func(b *Builder) {
		b.WriteString("INSERT INTO ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" (")
		for i, col := range cols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.Ident(col)
		}
		b.WriteString(") VALUES ")
		for i := range cs {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteByte('(')
			vals := append(append([]any{}, pkey...), ckeys[i]...)
			for j, v := range vals {
				if j > 0 {
					b.WriteString(", ")
				}
				b.Arg(v)
			}
			b.WriteByte(')')
		}
		switch {
		case d.OnConflict:
			b.WriteString(" ON CONFLICT DO NOTHING")
		case d.OnDuplicateKey:
			b.WriteString(" ON DUPLICATE KEY UPDATE ")
			b.Ident(cols[0])
			b.WriteString(" = ")
			b.Ident(cols[0])
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "insert", r.Through.Table.Name, st)
	return err
}

// deleteJoinRows deletes the join rows of p for cs (keep false), or the join rows
// of p for every child not in cs (keep true, used by Replace). One DELETE either way.
func (r Relation[P, C]) deleteJoinRows(ctx context.Context, q Querier, p *P, cs []*C, keep bool) error {
	pkey, ok := r.ParentKey(p)
	if !ok {
		return &Error{Kind: ErrValidation, Op: "assoc", Table: r.Through.Table.Name, Err: errors.New(r.Name + ": parent key is NULL")}
	}
	ckeys := make([][]any, len(cs))
	for i, c := range cs {
		k, ok := r.ChildKey(c)
		if !ok {
			return &Error{Kind: ErrValidation, Op: "assoc", Table: r.Child.Name, Err: errors.New(r.Name + ": child key is NULL")}
		}
		ckeys[i] = k
	}
	tbl := r.Through.Table.Qualifier()
	st, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("DELETE FROM ")
		r.Through.Table.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: tbl, cols: r.Through.ParentCols, keys: [][]any{pkey}}.AppendSQL(b)
		b.WriteString(" AND ")
		if keep {
			b.WriteString("NOT (")
		}
		inTuple{table: tbl, cols: r.Through.ChildCols, keys: ckeys}.AppendSQL(b)
		if keep {
			b.WriteString(")")
		}
	})
	if err != nil {
		return err
	}
	_, err = q.conn().exec(ctx, "delete", r.Through.Table.Name, st)
	return err
}
```

Imports for `assoc.go`: `context`, `errors`, and `github.com/azrtydxb/momo/dialect` only if used (it is not in this file, so omit it).

Run the tests:

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run '^TestAssociationHelpers$' -count=1` prints `ok`.

- [ ] **Step 5: Commit** — `git add assoc.go assoc_test.go && git commit -m "feat(relations): Link, Unlink, Replace and Count association helpers"`.

Reconciliation note (2026-11-06): Task 17 differs from the sketch in these ways. (1) Kind is checked before the empty-children shortcut, so `Link`/`Unlink` on BelongsTo return `ErrUnsupported` even with no children, and `Count` on BelongsTo returns `ErrUnsupported`. (2) NULL parent key: `Count` returns 0; `Link`, `Unlink` and `Replace` return `ErrValidation` (`Replace` checks before opening a transaction); NULL child keys are `ErrValidation`. (3) Polymorphic: `Link` writes the discriminator in the same UPDATE, and the clearing modes add the discriminator to the WHERE so another owner type's children are never cleared. (4) Key columns use `Qualifier()`, not the bare table name. (5) Superseded 2026-11-06 (review): a set that exceeds the dialect bind limit (`WithMaxParams`) is chunked with `keysPerStatement`, so S-6 holds; the S-83 wording "exactly one statement" becomes "one statement per chunk" (a small call is still one statement). When `q` is a `*DB` and more than one chunk is needed, `Link` and `Unlink` run the chunks in one transaction; on a `*Tx` they run in the caller's transaction. `Replace` cannot chunk `NOT IN`, so inside its transaction it SELECTs the current child keys (join table for ManyToMany, child table scoped by the discriminator for HasX), diffs them in Go, unlinks only removed keys and links only added keys in chunks, so kept rows are not rewritten; an empty set clears in one statement without the SELECT. Replace of a non-empty set is therefore BEGIN, select, [clear chunks], [link chunks], COMMIT. The helpers write directly (no model hooks, field hooks or UpdatedAt) and ManyToMany `Link` needs a unique or primary key over the join columns. Clearing a NOT NULL foreign key returns `ErrNotNullViolation` with a message that Unlink and Replace need a nullable foreign key. (6) ManyToMany `Count` joins the far table only when it has a soft-delete column, so soft-deleted children are not counted (still one statement). (7) `Replace` of a HasX relation clears the foreign key of every unlisted child including soft-deleted ones. The plan's `momo_test` snippet is extended with per-helper statement counts (Replace is BEGIN, clear, link, COMMIT as hook events) and the added tests `TestAssociationManyToManyUnlinkAndReplaceEmpty`, `TestAssociationHasManyReplaceInTx`, `TestAssociationPolymorphicAndComposite`, `TestAssociationBelongsToUnsupported`, `TestAssociationNullKeys`, plus the internal `TestAssociationChunkSizeAndRunChunks` (`assoc_internal_test.go`) and the chunking, atomicity, 70k-tag, NOT NULL, kept-row and polymorphic-discriminator tests in `assoc_chunk_test.go`. Polymorphic Unlink and Replace run against the fixture after the test makes `images.owner_id` nullable in its scratch schema; composite Unlink is not exercised because the fixture's composite child foreign key is NOT NULL. (8) Re-review 2026-11-06: `Replace` first locks the parent row (`SELECT 1 FROM parent WHERE key = ? FOR UPDATE`, on dialects whose `Locking` capability is set: PostgreSQL and MySQL; skipped on SQLite, which serialises writers) before reading the current keys, so concurrent `Replace` calls on one parent no longer produce unique violations, duplicate join rows or a mixed set under READ COMMITTED; a missing parent row locks nothing and is not an error. Replace is therefore BEGIN, lock, [select], [clear chunks], [link chunks], COMMIT on those dialects (statement counts in the tests add one when `Locking` is set; Replace of an empty set is BEGIN, lock, clear, COMMIT). On a `*Tx` the lock lasts until the caller's transaction ends, and a multi-chunk `Link`/`Unlink` on a `*Tx` runs in the caller's transaction without a savepoint, so earlier chunks stay applied until the caller rolls back (godoc states both). Test `TestAssociationConcurrentReplaceSameParent` (`assoc_concurrent_integration_test.go`) races two `Replace` calls ({1,2} to {1,3,4} and {2,3,5}) 20 times on pooled connections; the shared test helper `ids` in `assoc_chunk_test.go` is now `assocQueryIDs` and the duplicate `mustExec` was dropped after the merge with sprint-2.

---

## Task 18: Graph writes: CreateGraph, FindOrCreate and Upsert

Files:

- Create `graph.go`
- Create `internal/relfixture/tag/momo_gen.go`
- Create `graph_test.go` (package `momo_test`)

Interfaces:

- `func (r Relation[P, C]) Include() Association[P]`
- `type Association[P any] interface{ before(ctx context.Context, tx *Tx, p *P) error; after(ctx context.Context, tx *Tx, p *P) error }`
- `func CreateGraph[P any, PP Modeler[P]](ctx context.Context, db *DB, p *P, includes ...Association[P]) error`
- `func FindOrCreate[T any, PT Modeler[T]](ctx context.Context, q Querier, key []ColumnRef, m *T) (created bool, err error)`
- `func Upsert[T any, PT Modeler[T]](ctx context.Context, q Querier, m *T) error`

No dependency on momo-05: this task writes its own statements with the internal `Builder`. momo-05's insert builder may later replace `insertRow` without changing its callers.

Semantics:

- `CreateGraph` runs in one transaction (`db.Tx`). Insert the parent with momo-05's `Insert`. For each `Include()`d relation, in listed order: BelongsTo children are inserted first and their key is written into the parent with `SetParentKey`; HasOne/HasMany children get `SetChildKey(child, ParentKey(parent))` and are inserted after the parent; ManyToMany children are inserted, then join rows are linked with `Link`. Relations not listed are never written (the test asserts this).
- `FindOrCreate` inserts `m` with `OnConflict(key...).DoNothing()`; if `created` is false, `m` is overwritten by selecting the row with the same key values (`Meta.Value` on the key columns found by name). Race-safe because the decision is made by the unique constraint.
- `Upsert` inserts `m` with `OnConflict(pk...).DoUpdate(SetExcluded for every non-primary-key column that is not read-only, immutable or create-only)`.

Covers: S-84. Acceptance test names: `TestCreateGraphAndFindOrCreate`, `TestUpsertByPK`.

- [ ] **Step 1: Write the failing tests** — create `graph_test.go`:

```go
package momo_test

import (
	"context"
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/relfixture"
	"github.com/azrtydxb/momo/internal/relfixture/comment"
	"github.com/azrtydxb/momo/internal/relfixture/document"
	"github.com/azrtydxb/momo/internal/relfixture/models"
	"github.com/azrtydxb/momo/internal/relfixture/tag"
)

func TestCreateGraphAndFindOrCreate(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()

		doc := &models.Document{ID: 10, Title: "new", Comments: []models.Comment{{ID: 20, Body: "hi", Approved: true}}, Tags: []models.Tag{{ID: 3, Name: "x"}}}
		// Only Comments is listed: Tags must not be written.
		if err := momo.CreateGraph(ctx, db, doc, document.Comments.Include()); err != nil {
			t.Fatal(err)
		}
		n, err := momo.Select[models.Comment, *models.Comment](db).Where(comment.ID.Eq(20)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("listed child not written: %d", n)
		}
		tags, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(3)).Count(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tags != 0 {
			t.Fatal("unlisted relation was written")
		}

		// FindOrCreate race: N goroutines create the same key; exactly one creates.
		var wg sync.WaitGroup
		var mu sync.Mutex
		created := 0
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				m := &models.Tag{ID: 99, Name: "race"}
				c, err := momo.FindOrCreate[models.Tag, *models.Tag](ctx, db, []momo.ColumnRef{tag.ID}, m)
				if err != nil {
					t.Error(err)
					return
				}
				if c {
					mu.Lock()
					created++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		if created != 1 {
			t.Fatalf("exactly one creator expected, got %d", created)
		}
	})
}

func TestUpsertByPK(t *testing.T) {
	relfixture.Each(t, func(t *testing.T, db *momo.DB, h *relfixture.Hook) {
		relfixture.SeedBasic(t, db)
		ctx := context.Background()
		if err := momo.Upsert[models.Tag, *models.Tag](ctx, db, &models.Tag{ID: 1, Name: "golang"}); err != nil {
			t.Fatal(err)
		}
		tag, err := momo.Select[models.Tag, *models.Tag](db).Where(tag.ID.Eq(1)).One(ctx)
		if err != nil || tag.Name != "golang" {
			t.Fatalf("upsert must update every non-key column: %+v err=%v", tag, err)
		}
	})
}
```

Define `tag.ID` only if needed; prefer the generated `tag.ID` descriptor: add `internal/relfixture/tag/momo_gen.go` with this declaration and use `tag.ID.Eq(3)`. Replace the two `tag.ID` uses with `tag.ID` and add the `tag` import.

```go
var ID = momo.NewColumn[int64]("tags", "id", momo.FlagPK)
```

- [ ] **Step 2: Run and expect FAIL** — `go test ./ -run 'TestCreateGraphAndFindOrCreate|TestUpsertByPK' -count=1` fails with `undefined: momo.CreateGraph`.

- [ ] **Step 3: Implement.** Create the fixture package `internal/relfixture/tag/momo_gen.go`:

```go
// Code generated by momo (fixture). DO NOT EDIT.

package tag

import "github.com/azrtydxb/momo"

var ID = momo.NewColumn[int64]("tags", "id", momo.FlagPK)
var Name = momo.NewColumn[string]("tags", "name", 0)
```

Create `graph.go`:

```go
package momo

import (
	"context"
	"errors"
)

// insertMode selects the statement shape insertRow writes.
type insertMode uint8

const (
	insertPlain insertMode = iota
	insertDoNothing
	insertDoUpdate
)

// Association is one relation listed for CreateGraph. It is produced only by
// Relation.Include, so unlisted relations are never written.
type Association[P any] interface {
	before(ctx context.Context, tx *Tx, p *P) error
	after(ctx context.Context, tx *Tx, p *P) error
}

type includedRel[P, C any] struct{ r Relation[P, C] }

// Include lists the relation for CreateGraph: the children held in the parent's
// field are inserted in the same transaction.
func (r Relation[P, C]) Include() Association[P] { return includedRel[P, C]{r} }

// before runs for BelongsTo: the child is inserted first and its key written into
// the parent, which is inserted after it.
func (x includedRel[P, C]) before(ctx context.Context, tx *Tx, p *P) error {
	r := x.r
	if r.Kind != BelongsTo {
		return nil
	}
	c := r.GetOne(p)
	if c == nil {
		return nil
	}
	if _, err := insertRow(ctx, tx, r.ChildMeta, c, nil, insertPlain); err != nil {
		return err
	}
	key, ok := r.ChildKey(c)
	if !ok {
		return &Error{Kind: ErrValidation, Op: "create graph", Table: r.Child.Name, Err: errors.New(r.Name + ": child key is NULL")}
	}
	r.SetParentKey(p, key)
	return nil
}

// after runs for HasOne, HasMany and ManyToMany once the parent has its identity.
func (x includedRel[P, C]) after(ctx context.Context, tx *Tx, p *P) error {
	r := x.r
	switch r.Kind {
	case HasOne:
		if c := r.GetOne(p); c != nil {
			return r.insertChildren(ctx, tx, p, []*C{c})
		}
		return nil
	case HasMany:
		cs := r.GetMany(p)
		ptrs := make([]*C, len(cs))
		for i := range cs {
			ptrs[i] = &cs[i]
		}
		return r.insertChildren(ctx, tx, p, ptrs)
	case ManyToMany:
		cs := r.GetMany(p)
		ptrs := make([]*C, len(cs))
		for i := range cs {
			ptrs[i] = &cs[i]
			if _, err := insertRow(ctx, tx, r.ChildMeta, &cs[i], nil, insertPlain); err != nil {
				return err
			}
		}
		return r.Link(ctx, tx, p, ptrs...)
	}
	return nil
}

// insertChildren writes the parent's key into each child, then inserts it.
func (r Relation[P, C]) insertChildren(ctx context.Context, tx *Tx, p *P, cs []*C) error {
	key, ok := r.ParentKey(p)
	if !ok {
		return &Error{Kind: ErrValidation, Op: "create graph", Table: r.Child.Name, Err: errors.New(r.Name + ": parent key is NULL")}
	}
	for _, c := range cs {
		r.SetChildKey(c, key)
		if _, err := insertRow(ctx, tx, r.ChildMeta, c, nil, insertPlain); err != nil {
			return err
		}
	}
	return nil
}

// CreateGraph inserts p and the children of the listed relations in one
// transaction. Listed BelongsTo children are written before p; the other listed
// children after it. Unlisted relations are not written.
func CreateGraph[P any, PP Modeler[P]](ctx context.Context, db *DB, p *P, includes ...Association[P]) error {
	return db.Tx(ctx, TxOptions{}, func(tx *Tx) error {
		for _, inc := range includes {
			if err := inc.before(ctx, tx, p); err != nil {
				return err
			}
		}
		if _, err := insertRow(ctx, tx, metaOf[P, PP](), p, nil, insertPlain); err != nil {
			return err
		}
		for _, inc := range includes {
			if err := inc.after(ctx, tx, p); err != nil {
				return err
			}
		}
		return nil
	})
}

// FindOrCreate inserts m unless a row with the same key exists; then m holds the
// stored row. The unique constraint on key decides the winner, so concurrent
// callers never create duplicates. created reports whether this call inserted.
func FindOrCreate[T any, PT Modeler[T]](ctx context.Context, q Querier, key []ColumnRef, m *T) (bool, error) {
	meta := metaOf[T, PT]()
	cols := make([]string, len(key))
	for i, k := range key {
		cols[i] = k.ColumnName()
	}
	n, err := insertRow(ctx, q, meta, m, cols, insertDoNothing)
	if err != nil {
		return false, err
	}
	if n == 1 {
		return true, nil
	}
	return false, selectByKey(ctx, q, meta, m, cols)
}

// Upsert inserts m, or updates the row with the same primary key:
// ON CONFLICT (pk) DO UPDATE SET col = excluded.col for every updatable column.
func Upsert[T any, PT Modeler[T]](ctx context.Context, q Querier, m *T) error {
	meta := metaOf[T, PT]()
	_, err := insertRow(ctx, q, meta, m, pkNames(meta), insertDoUpdate)
	return err
}

// insertRow writes v as one row built from m. Identity columns are never sent:
// the database generates them and RETURNING reads them back (ErrUnsupported
// without RETURNING). Read-only columns are excluded from the INSERT; immutable
// columns are excluded from the update. With conflict columns the statement is
// ON CONFLICT (...) DO NOTHING, or DO UPDATE when there is something to update.
// It returns the rows affected: 0 means the conflict path was taken.
func insertRow[T any](ctx context.Context, q Querier, m *Meta[T], v *T, conflict []string, mode insertMode) (int64, error) {
	d := q.Dialect()
	if mode != insertPlain && !d.OnConflict {
		return 0, &Error{Kind: ErrUnsupported, Op: "insert", Table: m.Table, Err: errors.New("ON CONFLICT is not supported by this dialect")}
	}
	returning := m.Identity >= 0
	if returning && !d.Returning {
		return 0, &Error{Kind: ErrUnsupported, Op: "insert", Table: m.Table, Err: errors.New("identity read-back needs RETURNING")}
	}
	isKey := map[string]bool{}
	for _, name := range conflict {
		isKey[name] = true
	}
	var send, update []int
	for i, col := range m.Columns {
		if i == m.Identity || col.Flags&FlagReadOnly != 0 {
			continue
		}
		send = append(send, i)
		if !isKey[col.Name] && col.Flags&FlagImmutable == 0 {
			update = append(update, i)
		}
	}
	if mode == insertDoUpdate && len(update) == 0 {
		mode = insertDoNothing
	}
	c, err := compile(d, func(b *Builder) {
		b.WriteString("INSERT INTO ")
		TableRef{Name: m.Table}.appendFrom(b)
		b.WriteString(" (")
		for k, i := range send {
			if k > 0 {
				b.WriteString(", ")
			}
			b.Ident(m.Columns[i].Name)
		}
		b.WriteString(") VALUES (")
		for k, i := range send {
			if k > 0 {
				b.WriteString(", ")
			}
			b.Arg(m.Value(v, i))
		}
		b.WriteByte(')')
		switch mode {
		case insertDoNothing:
			b.WriteString(" ON CONFLICT (")
			writeIdents(b, conflict)
			b.WriteString(") DO NOTHING")
		case insertDoUpdate:
			b.WriteString(" ON CONFLICT (")
			writeIdents(b, conflict)
			b.WriteString(") DO UPDATE SET ")
			for k, i := range update {
				if k > 0 {
					b.WriteString(", ")
				}
				b.Ident(m.Columns[i].Name)
				b.WriteString(" = ")
				b.QualifiedIdent("excluded", m.Columns[i].Name)
			}
		}
		if returning {
			b.WriteString(" RETURNING ")
			b.Ident(m.Columns[m.Identity].Name)
		}
	})
	if err != nil {
		return 0, err
	}
	if !returning {
		return q.conn().exec(ctx, "insert", m.Table, c)
	}
	rows, err := q.conn().query(ctx, "insert", m.Table, "", c)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	if err := rows.Scan(m.Ptr(v, m.Identity)); err != nil {
		return 0, err
	}
	return 1, rows.Err()
}

func writeIdents(b *Builder, names []string) {
	for i, n := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		b.Ident(n)
	}
}

func pkNames[T any](m *Meta[T]) []string {
	out := make([]string, len(m.PK))
	for i, j := range m.PK {
		out[i] = m.Columns[j].Name
	}
	return out
}

// selectByKey loads the row whose key columns equal those of m into m.
func selectByKey[T any](ctx context.Context, q Querier, m *Meta[T], v *T, cols []string) error {
	key := make([]any, len(cols))
	for i, name := range cols {
		j := columnIndex(m, name)
		if j < 0 {
			return &Error{Kind: ErrValidation, Op: "find", Table: m.Table, Err: errors.New("key column not in model: " + name)}
		}
		key[i] = m.Value(v, j)
	}
	c, err := compile(q.Dialect(), func(b *Builder) {
		b.WriteString("SELECT ")
		for i, col := range m.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.QualifiedIdent(m.Table, col.Name)
		}
		b.WriteString(" FROM ")
		TableRef{Name: m.Table}.appendFrom(b)
		b.WriteString(" WHERE ")
		inTuple{table: m.Table, cols: cols, keys: [][]any{key}}.AppendSQL(b)
	})
	if err != nil {
		return err
	}
	rows, err := q.conn().query(ctx, "select", m.Table, "", c)
	if err != nil {
		return err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return &Error{Kind: ErrNotFound, Op: "find", Table: m.Table}
	}
	dest := make([]any, len(m.Columns))
	for i := range dest {
		dest[i] = m.Ptr(v, i)
	}
	return rows.Scan(dest...)
}
```

Note: `FindOrCreate` and `Upsert` use this path on every dialect that reports `OnConflict`; MySQL reports ErrUnsupported until its `ON DUPLICATE KEY UPDATE` form is added in momo-05.

- [ ] **Step 4: Run and expect PASS** — `go test ./ -run 'TestCreateGraphAndFindOrCreate|TestUpsertByPK' -count=1 -race` prints `ok`.

- [ ] **Step 5: Commit** — `git add graph.go graph_test.go internal/relfixture/tag && git commit -m "feat(relations): CreateGraph, FindOrCreate and Upsert by primary key"`.

Reconciliation note (2026-11-06): Task 18 differs from the sketch in these ways. (1) The sketch's own `insertRow` is not written; every graph row goes through momo-05's `InsertQuery` (`insertModel`/`runInsert` in `graph.go`, building `InsertQuery{q, meta}` from the relation's `ChildMeta`), so field hooks, timestamps, defaults and validation run as for `Insert`, and generated columns are read back with `RETURNING ... ALL` (`ErrUnsupported` for an identity model on a dialect without RETURNING). (2) `Upsert` is already momo-05's builder (`upsert.go`), so the plan's one-call function is named `UpsertByPK(ctx, q, m) error` and wraps `Upsert(q).Model(m)`; it inherits the builder's rules (DO UPDATE over every updatable column, updated-at refreshed, primary key, version, soft-delete and created-at never overwritten). (3) `CreateGraph` takes a `Querier`: on a `*DB` it opens a transaction, on a `*Tx` it runs in a savepoint (`Tx.Tx`), so a failed graph is undone and the caller's transaction stays usable. (4) `FindOrCreate` uses `INSERT ... ON CONFLICT (key) DO NOTHING RETURNING`; no returned row means a conflict and the row is re-selected with `Select(...).WithDeleted().Where(key = value)`, replacing the sketch's `selectByKey`; the stored row replaces `*m` entirely (relation fields reset), and a key column that the model does not map or an empty key is `ErrValidation`. Under REPEATABLE READ or SERIALIZABLE the re-select can miss the winner and returns `ErrNotFound` (godoc). (5) Cycles: a graph is one level deep (children's own relations are never followed), so a self-referencing relation cannot recurse; a model listed as its own child (`e.Manager = e`) is rejected with `ErrValidation` before any write. (6) `relfixture.Hook` gained a mutex because the race test runs statements concurrently through the shared hook. Tests added beyond the plan: `TestFindOrCreateReturnsStoredRow`, `TestCreateGraphBelongsToFirstAndManyToMany`, `TestCreateGraphPolymorphicCompositeAndSelfReference`, `TestCreateGraphRollsBackWholeGraph` (including the savepoint case), `TestCreateGraphRunsFieldHooksOnChildren` and, with hand-written identity models, `TestCreateGraphGeneratedIdentityPropagates`. The plan's test is `TestUpsertByPK` calling `UpsertByPK`. Mutation checks: dropping `SetChildKey`, dropping the RETURNING read-back, always reporting created, and skipping the belongs-to `before` pass each fail the tests.

---

## Final verification (run after Task 18)

- [ ] `go test ./... -count=1 -race` passes on SQLite; with `MOMO_TEST_POSTGRES_URL` set, `go test ./ -run 'TestEagerLoad|TestPipelinedEagerLoad|TestJoinLoadToOne' -count=1` passes on PostgreSQL.
- [ ] `go list -deps ./ | grep -c 'internal/relfixture'` prints `0` (the runtime package never imports the fixture).
- [ ] `go vet ./...` prints nothing.
