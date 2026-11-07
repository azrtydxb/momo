# Tests: consolidate test files named after review rounds

Status: done
Created: 2026-11-07
Epic: momo-01-foundation
Sprint: 005-sprint-5-security-and-maintainability-hardening

## Description

Maintainability item 2: about 16 test files are named after review rounds (_review, _polish, _more, _fixes, _repair, _hardening) and load alone has 14 files. Audit report: .procoder/research/security-audit-2026-11-07.md and .procoder/research/maintainability-audit-2026-11-07.md.

## Acceptance criteria

- [x] Test files are organised by the production file they test; no review-round suffixes remain
- [x] A repo check refuses those suffixes
- [x] Test count unchanged or higher (CI)

## Evidence

<!-- Filled at close time: the commands run and what their output proved,
     one line per criterion. Empty evidence keeps the story open. -->

- Criterion 1 (organised by production file, no suffixes): 19 files (`*_review*`, `*_polish*`, `*_more*`, `*_fixes`, `*_repair`, `*_hardening*`, `relpred_shapes`, `in_tuple_edges`) were folded into the files named for their feature (bulk_unit, expr_ops, emit_descriptors, graph/graph_unit, in_tuple_internal, joinload, load_nested/load_unit, migrate/baseline, raw_args_internal, relpred, select_validation, timestamps, typed_expr, upsert, with their integration siblings); build tags and packages were asserted equal per merge. `find . -name '*_test.go' | grep -E '_(review|polish|more|fixes|repair|hardening)'` prints nothing.
- Criterion 2 (repo check): `TestNoReviewRoundTestFiles` in internal/repocheck/testconv_test.go.
- Criterion 3 (test count): inventory before and after with `grep -rE '^func (Test|Fuzz|Benchmark|Example)[A-Za-z0-9_]*' --include='*_test.go' . | sed -E 's#^(\./)?(.*/)?[^/:]*_test\.go:func ((Test|Fuzz|Benchmark|Example)[A-Za-z0-9_]*).*#\2:\3#' | sort` gave 1234 lines before and 1235 after; `diff before.txt after.txt` shows only `> internal/repocheck/:TestNoReviewRoundTestFiles`, so no test was lost. CI run 42407906061 on 01ba968 is fully green (lint, unit, race, macos, windows, integration pg16/pg17, fuzz-smoke).
