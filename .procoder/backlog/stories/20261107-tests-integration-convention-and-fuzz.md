# Tests: one integration-test convention and every fuzz target in CI

Status: done 2026-11-07
Created: 2026-11-07
Epic: momo-01-foundation
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 3 and fuzz note: integration tests use three conventions (build tag, filename, runtime skip) and untagged ones silently skip on the macOS and Windows jobs; 5 of 15 fuzz targets never run in CI. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] Every integration test uses the integration build tag and testdb; a CI check fails on a database test without the tag
- [x] macOS and Windows jobs state they run unit tests only
- [x] Every fuzz target runs in fuzz-smoke

## Evidence

- Tag convention: database tests moved or tagged into files with the integration build tag; TestIntegrationTagConvention in internal/repocheck fails on an untagged file that imports testdb, testopen or exampletest, uses relfixture.Each and friends, or names a MOMO_TEST_ variable. CI run 42406029851 (commit 3673713) is green on lint, unit, race, compile-fail and all four integration jobs, which run the tagged suite with MOMO_REQUIRE_INTEGRATION=1.
- macOS and Windows: the jobs are named "unit tests only, no database" in ci.yml, and untagged builds now contain no database test, so nothing can skip silently there. Both jobs green in run 42406029851.
- Fuzz: FUZZ_TARGETS lists all 16 Fuzz functions (added FuzzUUIDScan, FuzzTimeOfDayScan, FuzzDateScan, FuzzJSONScan, gen/tags FuzzParse); TestFuzzTargetsListed compares the list with the tree. fuzz-smoke green on Go 1.26 and 1.27 in run 42406029851 with FUZZTIME=20000x (an execution count, which avoids the time-based coordinator deadline behind the FuzzBuilder context deadline exceeded flake).
