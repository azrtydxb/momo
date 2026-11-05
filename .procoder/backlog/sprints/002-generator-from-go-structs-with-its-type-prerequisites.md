# Sprint 2: Generator from Go structs, with its type prerequisites

Status: active
Created: 2026-11-05

## Goal

A developer can write ordinary Go structs, with `momo:` tags and a `MomoConfig` companion, run `momo generate`, and get reflection-free, compiling model code:

- column and relation descriptors;
- `Meta` with typed hooks wiring left for momo-05;
- lookup by name;
- deterministic, header-marked output that refuses to overwrite handwritten files;
- naming presets matching GORM, Bun and Ent;
- the generator plugin interface;
- the `momo.yaml` config.

This sprint also lands the three type-only prerequisites the generator consumes: momo-09 Task 1 (QueryEvent pool-wait fields and `RecordPoolWait`), momo-04 Task 1 (schema managed-object types) and momo-06 Task 2 (the `Relation` descriptor).

Out of scope, because they wait for momo-05's write builders and `HookSet`: momo-02 Tasks 25 (gate), 26, 27, 29 and 30.

Done means every pulled story is closed and the generator golden tests, the example-model build and the full race suite pass, plus the PostgreSQL integration gate.
