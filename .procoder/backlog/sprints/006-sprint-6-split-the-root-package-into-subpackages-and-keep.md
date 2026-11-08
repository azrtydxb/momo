# Sprint 6: split the root package into subpackages and keep docs current by construction

Status: active
Created: 2026-11-08

## Goal

Deliver the 2026-11-08 decision "Root-package structure and always-current docs" (epic momo-14-structure-and-docs, plan in `.procoder/research/package-split.md`). Users import feature APIs from small subpackages (`fts`, `jsonop`, `agg`, `timeop`, `sqlfn`, `advisory`, `vecsearch`, `vecop`, `arrayop`, `obs`) instead of one root package with a hundred files. Non-API mechanics live in `internal/`. An architecture test and a file-prefix guard keep it that way. The docs can no longer go stale silently:

- version strings have one source;
- the capability matrix and type table are generated with a drift check;
- every identifier the docs name must exist;
- an apidiff gate requires a docs change for every API change;
- `docs/upgrading.md` maps every moved identifier.

This is a breaking change before v1, released later as 0.4.0 through release-please once the kuvryn-license gate allows it.
