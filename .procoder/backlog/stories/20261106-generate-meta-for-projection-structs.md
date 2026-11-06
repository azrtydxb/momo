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

- [ ] `momo generate` emits `MomoMeta` and a `Meta[T]` for every struct that embeds `momo.Projection`, with `Identity`, `Version`, `SoftDelete`, `CreatedAt` and `UpdatedAt` set to -1 and `Columns`, `Ptr` and `Value` covering every exported field in declaration order. Tested by a golden-file test in `gen/`.
- [ ] A column name set by the `momo` struct tag on a projection field is honored; without one the name follows the same naming rule as model fields. Tested in `gen/`.
- [ ] A projection with an unsupported field type fails generation with an error naming the struct and field, the same as for models. Tested in `gen/`.
- [ ] `examples/projection-pagination/models` and `examples/vector-search/models` contain no hand-written `MomoMeta` for projections, and their example tests still pass.
- [ ] `momo generate --check` is clean on both examples after the change.
- [ ] A docs page or section on projections (declaring one, `From` and `Columns`, the generated metadata) is linked from `docs/README.md`, and the "hand-written MomoMeta" note in the projections section is removed.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->
