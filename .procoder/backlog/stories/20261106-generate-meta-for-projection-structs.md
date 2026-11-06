# Generate Meta for projection structs

Status: open
Created: 2026-11-06
Epic: momo-02-generator
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

## Description

As a Momo user (or an AI coding agent copying the examples), I declare a struct
that embeds `momo.Projection` to read a subset of columns or an aggregate, and I
want `momo generate` to emit its `MomoMeta`, as it does for models. Today `gen/`
has no `momo.Projection` handling: `examples/projection-pagination` and
`examples/vector-search` hand-write `MomoMeta` and a `Meta[T]` with `Identity`,
`Version`, `SoftDelete`, `CreatedAt` and `UpdatedAt` all set to -1, and the docs
do not cover projections beyond a capabilities row. Done means a projection
needs no hand-written metadata, and the docs say how to declare one.

## Acceptance criteria

- [x] `momo generate` emits `MomoMeta` and a `Meta[T]` for every struct that embeds `momo.Projection`, with `Identity`, `Version`, `SoftDelete`, `CreatedAt` and `UpdatedAt` set to -1 and `Columns`, `Ptr` and `Value` covering every exported field in declaration order. Tested by a golden-file test in `gen/`.
- [x] A column name set by the `momo` struct tag on a projection field is honored; without one the name follows the same naming rule as model fields. Tested in `gen/`.
- [x] A projection with an unsupported field type fails generation with an error naming the struct and field, the same as for models. Tested in `gen/`.
- [x] `examples/projection-pagination/models` and `examples/vector-search/models` contain no hand-written `MomoMeta` for projections, and their example tests still pass.
- [x] `momo generate --check` is clean on both examples after the change.
- [x] A docs page or section on projections (declaring one, `From` and `Columns`, the generated metadata) is linked from `docs/README.md`, and the "hand-written MomoMeta" note in the projections section is removed.

## Evidence

- Criterion 1: TestEmitMetaProjectionGolden compares the emitted MomoMeta of two projections against gen/testdata/golden/meta/projection/momo_gen.go (all five indexes -1, Columns, Ptr and Value in declaration order); TestProjectionOnlyPackageEmits covers a package with only projections.
- Criterion 2: the same golden shows `momo:"column:points"` honored and `HTTPCode` named http_code by the naming rule.
- Criterion 3: TestProjectionUnsupportedFieldNamesStructAndField expects "projection Odd: field Value: ... complex128"; breaking the check made it fail.
- Criterion 4: the hand-written MomoMeta is removed from both example models packages, and the example tests pass in RUNID.
- Criterion 5: `momo generate --check` exits 0 in examples/projection-pagination and examples/vector-search.
- Criterion 6 (also the upgrade note for hand-written projection metadata in docs/projections.md): docs/projections.md is linked from docs/README.md, and the hand-written MomoMeta notes are gone from README and docs/ai-guide.md.
