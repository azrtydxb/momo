# Generator emits VectorColumn for vector fields

Status: open
Created: 2026-11-05
Epic: momo-07-vectors-pgvector
Sprint: 002-generator-from-go-structs-with-its-type-prerequisites

## Description

Story 158 built `momo.VectorColumn` and the typed distance functions, but the generator still emits a plain `momo.Column[vector.Vector]` for a vector field. `CosineDistance`, `L2Distance` and the other metrics take a `VectorColumn`, so every user must hand-write a `momo.NewVectorColumn` call and keep its table, column name and dimension in step with the model. The README similarity-search section and `examples/quickstart/main.go` show exactly that workaround. This story makes the generator emit a `momo.VectorColumn[V]` (with the dimension from the `dim` tag) for fields of the four `vector` package types, so `post.Embedding` can be passed straight to a distance function and the workaround disappears from the docs.

## Acceptance criteria

- [x] A golden for a model with `vector.Vector`, `vector.HalfVector`, `vector.SparseVector` and `vector.Bits` fields shows `momo.VectorColumn[...]` built with `momo.NewVectorColumn` and the tagged dimension; it failed before the change and passes after.
- [x] A compile test calls `momo.CosineDistance` on a generated vector column (and `HammingDistance` on the Bits column) and builds the SQL; a stand-in `Vector` type that is not from the `vector` package still emits a plain column.
- [x] A generated `VectorColumn` satisfies `momo.ColumnRef`, so existing generated code (lookup, meta, indexes) compiles unchanged; regenerated `momo_gen.go` files are committed and `momo generate --check` passes for each.
- [x] `examples/quickstart/main.go` and the README similarity-search section use the generated column with no hand-written `NewVectorColumn`; the README doc-sync test passes.
- [x] Integration tests for vector, distance and search paths pass against PostgreSQL with pgvector, and `procoder check` is clean.

## Evidence

- Criterion 1: before the change, TestEmitDescriptorsVectorColumns failed (golden for the tagvocab Article model did not exist or differed); after emission it passes, and the golden shows four VectorColumn fields with dims 3, 4, 5 and 8.
- Criterion 2: TestGeneratedVectorColumnDistance in internal/testmodels/example did not compile before (Column cannot be used as VectorColumn); it now passes and a wrong-sized query vector gives ErrValidation. TestEmitDescriptorsStandInVectorStaysPlain passes for the stand-in type.
- Criterion 3: the regenerated document/momo_gen.go and models/post/momo_gen.go compile with unchanged lookup and meta code; `momo generate --check` exits 0 in internal/testmodels/example and examples/quickstart.
- Criterion 4: examples/quickstart/main.go and the README use post.Embedding; `go test ./examples/...` passes including the README doc-sync test. docs/vectors.md and example_vector_test.go use document.Embedding.
- Criterion 5: the vector, distance and search integration tests pass against PostgreSQL with pgvector (`-tags integration -race`); make fmt-check vet staticcheck gosec tidy-check is clean. Mutation: making isVectorType always true failed both new gen tests; restored, green.
- Review fixes, parity: VectorColumn now delegates to an inner Column and offers every Column method except Lt/Le/Gt/Ge/Between (no total order on vectors); NullVectorColumn adds SetNull. TestGeneratedVectorColumnMethodParity (compile and value test), TestVectorColumnIsNotNullFilterSQL, TestVectorColumnSetNullWritesNull, TestVectorColumnSetExcludedInUpsert and TestVectorColumnNullableKeyset cover it. Mutations that return 0 from Flags, swap SetExcluded for SetDefault, IsNotNull for IsNull, and SetNull for SetDefault each failed a test; restored, green.
- Review fixes, permissions: read_only, create_only and immutable vector fields emit ReadOnlyVectorColumn, CreateOnlyVectorColumn and ImmutableVectorColumn; the distance builders take a VectorSource[V] constraint they all satisfy, so a read_only embedding can be searched while keeping its write restriction. docs/vectors.md no longer carries the caveat. TestEmitDescriptorsVectorPermissions and its chunk golden pin the emitted types; ignoring the access mode in VectorDescriptorType failed it.
- Review fixes, type tags and aliases: the same fixture holds vector(3), halfvec(4), sparsevec(5) and bit(8) type tags asserting dim 3, 4, 5 and 8; disabling the type-tag branch of vectorDim failed it. A field of an alias to *vector.Vector now loads and emits a nullable column (the flattener unaliases the field type); removing the Unalias failed the test.
