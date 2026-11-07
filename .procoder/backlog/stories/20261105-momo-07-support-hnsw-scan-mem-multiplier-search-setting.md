# momo-07 support hnsw.scan_mem_multiplier search setting

Status: done 2026-11-06
Created: 2026-11-05
Epic: momo-07-vectors-pgvector
Sprint: 003-sprint-3-postgresql-hardening-migration-safety-bugs-query

## Description

pgvector 0.8.0 added the `hnsw.scan_mem_multiplier` setting, which bounds how much memory an iterative HNSW scan may use as a multiple of `work_mem`. Users who run filtered ANN queries with iterative scan on large result sets need to raise it, and today they can only do so through `Tx.SetLocal` with a raw name. This story adds it to `SearchSettings` as a typed field, applied through `Tx.SetSearch` and `DB.WithSearch` like the other settings: transaction-scoped, validated, and gated on pgvector 0.8 or newer. It was left out of momo-07 Task 6 because the plan did not list it.

## Acceptance criteria

- [x] `SearchSettings` gains an `HNSWScanMemMultiplier` field (float, zero means unchanged) that `searchSettingPairs` emits as `hnsw.scan_mem_multiplier`, formatted without exponent notation; a unit test pins the pair and its order.
- [x] `Validate` rejects values outside pgvector's accepted range (1 to 1000) with `ErrValidation`; unit tests cover both bounds and a value just outside each.
- [x] Setting the field triggers the same pgvector 0.8 version check as the iterative settings and returns `ErrUnsupported` on an older extension; a fake-driver test proves no `set_config` is issued on 0.7.x.
- [x] An integration test applies the field inside `WithSearch`, reads it back with `current_setting`, and shows it reset after the transaction on a one-connection pool.
- [x] The field doc comment states the unit, the range and the 0.8 requirement, and the `procoder check` gate is clean.

## Evidence

- Criterion 1: `TestSearchSQL` pins the pair `hnsw.scan_mem_multiplier` and its order (after `hnsw.max_scan_tuples`); `TestSearchScanMemMultiplierFormat` pins formatting without exponent (1, 1000, 2.5, 100, 1.25). Mutation: dropping the pair fails both.
- Criterion 2: `TestSearchSettingsValidation` rejects 0.99, 1000.01, -1, NaN and +Inf with ErrValidation and accepts 1 and 1000.
- Criterion 3: `TestSearchScanMemMultiplierNeedsPgvector08` (fake driver): 0.7.4 returns ErrUnsupported with no `set_config` issued, 0.8.0 succeeds. Mutation: removing the field from `usesIterativeGUCs` fails it.
- Criterion 4: CI run 42302749577 (`TestSearchScanMemMultiplierScoped`: value set inside `WithSearch`, back to its prior value after the transaction on a one-connection pool).
- Criterion 5: field doc comment states unit (multiplier of work_mem), range 1 to 1000 and the 0.8.0 requirement; the CI lint job (fmt, vet, staticcheck) passed on the same run.
