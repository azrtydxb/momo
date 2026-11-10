# Changelog

## 0.1.0 (2026-11-10)


### ⚠ BREAKING CHANGES

* `advisory.XactLockString` and `advisory.TryXactLockString` take a trailing `...advisory.Option`, so assigning them to a variable of the old func type no longer compiles. `momo generate` now wires methods named `BeforeInsert`, `BeforeUpdate`, `BeforeDelete` and `AfterLoad` into `Meta.Hooks`, and a hook-named method with another signature fails generation. `momo generate --from-db` adds the `dbdefault` tag to NOT NULL PostgreSQL columns with a computed default, so regenerated models stop writing Go zero values to those columns. The CLI moved from `cmd/momo` into package `cli`. See docs/upgrading.md, "Upgrading to 0.5.0".

### Features

* sprint 7 Orbit port issues ([#54](https://github.com/azrtydxb/momo/issues/54)-[#69](https://github.com/azrtydxb/momo/issues/69)) and backlog stories ([#70](https://github.com/azrtydxb/momo/issues/70)) ([2b4435c](https://github.com/azrtydxb/momo/commit/2b4435c78e0cf7f92bd3704baaacb1a16d7eaa88))
