# momo-07 support hnsw.scan_mem_multiplier search setting

Status: open
Created: 2026-11-05
Epic: momo-07-vectors-pgvector
Sprint: -

## Description

pgvector 0.8.0 added the `hnsw.scan_mem_multiplier` setting, which bounds how much memory an iterative HNSW scan may use as a multiple of `work_mem`. Users who run filtered ANN queries with iterative scan on large result sets need to raise it, and today they can only do so through `Tx.SetLocal` with a raw name. This story adds it to `SearchSettings` as a typed field, applied through `Tx.SetSearch` and `DB.WithSearch` like the other settings: transaction-scoped, validated, and gated on pgvector 0.8 or newer. It was left out of momo-07 Task 6 because the plan did not list it.

## Acceptance criteria

- [ ] `SearchSettings` gains an `HNSWScanMemMultiplier` field (float, zero means unchanged) that `searchSettingPairs` emits as `hnsw.scan_mem_multiplier`, formatted without exponent notation; a unit test pins the pair and its order.
- [ ] `Validate` rejects values outside pgvector's accepted range (1 to 1000) with `ErrValidation`; unit tests cover both bounds and a value just outside each.
- [ ] Setting the field triggers the same pgvector 0.8 version check as the iterative settings and returns `ErrUnsupported` on an older extension; a fake-driver test proves no `set_config` is issued on 0.7.x.
- [ ] An integration test applies the field inside `WithSearch`, reads it back with `current_setting`, and shows it reset after the transaction on a one-connection pool.
- [ ] The field doc comment states the unit, the range and the 0.8 requirement, and the `procoder check` gate is clean.

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. -->
