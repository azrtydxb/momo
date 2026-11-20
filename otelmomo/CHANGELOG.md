# Changelog

## [0.2.0](https://github.com/azrtydxb/momo/compare/otelmomo/v0.1.0...otelmomo/v0.2.0) (2026-11-20)


### ⚠ BREAKING CHANGES

* obs.NewSlowQuery, momolog.New and otelmomo.New return concrete types instead of momo.Hook; momo.NewNPlusOneHook, momo.WithQueryScope and momo.NPlusOneFinding are deprecated for obs.NewStatementNPlusOne, obs.WithScope and obs.StatementFinding.

### Code Refactoring

* one N+1 detector and one hook constructor shape ([#275](https://github.com/azrtydxb/momo/issues/275)) ([6bcee50](https://github.com/azrtydxb/momo/commit/6bcee50d8c3184c481c5b9441d3370ea71fc8434)), closes [#269](https://github.com/azrtydxb/momo/issues/269)

## 0.1.0 (2026-11-10)

_The first release of `otelmomo`, the OpenTelemetry integration for momo. It needs momo v0.5.0 or later. See [docs/observability.md](https://github.com/azrtydxb/momo/blob/v0.5.0/docs/observability.md)._

### Features

- **otelmomo:** a span per momo statement and a statement-duration histogram, wired in as a momo query hook ([#70](https://github.com/azrtydxb/momo/pull/70))
