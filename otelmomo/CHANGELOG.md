# Changelog

## [0.2.0](https://github.com/azrtydxb/momo/compare/otelmomo/v0.1.0...otelmomo/v0.2.0) (2026-11-20)

_`otelmomo.New` returns its concrete hook type, matching the other momo hook constructors. It needs momo v0.20.0 or later; a caller that stores the result as a `momo.Hook` keeps compiling. See [docs/observability.md](https://github.com/azrtydxb/momo/blob/v0.20.0/docs/observability.md)._

### ⚠ BREAKING CHANGES

- **otelmomo:** `otelmomo.New` returns `*otelmomo.Hook` instead of `momo.Hook` ([#275](https://github.com/azrtydxb/momo/pull/275), closes [#269](https://github.com/azrtydxb/momo/issues/269))

## 0.1.0 (2026-11-10)

_The first release of `otelmomo`, the OpenTelemetry integration for momo. It needs momo v0.5.0 or later. See [docs/observability.md](https://github.com/azrtydxb/momo/blob/v0.5.0/docs/observability.md)._

### Features

- **otelmomo:** a span per momo statement and a statement-duration histogram, wired in as a momo query hook ([#70](https://github.com/azrtydxb/momo/pull/70))
