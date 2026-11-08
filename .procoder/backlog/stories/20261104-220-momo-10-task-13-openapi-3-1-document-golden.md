# momo-10 Task 13: OpenAPI 3.1 document (golden)

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 13: OpenAPI 3.1 document (golden) (`.procoder/plans/momo-10-ecosystem-generators.md:1246`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: supporting work for the plan. Files: `gen/openapi/doc.go`, `gen/openapi/doc_test.go`, `gen/openapi/testdata/example.openapi.json`.

## Acceptance criteria

- [x] `TestOpenAPIDocumentGolden` exists as written in the plan, failed before the implementation, and passes now
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Files: gen/openapi/doc.go, doc_test.go, testdata/example.openapi.json.
- TestOpenAPIDocumentGolden (and TestDocumentRules) red: CI run 42455374066 (tests alone on sprint-5) failed with "undefined: Document". Green: CI run 42456595252 (commit 5f4ffe6). The golden was written from the first run's output (run 42455728306) after review against the 3.1 rules: nullable columns use the type array form and the nullable keyword never appears; relations, sensitive and unmapped columns are not properties; list parameters are limit, after and one filter parameter per non-vector column; a composite key has no id path.
- Review fixes (red: CI run 42458914148): a table name must be one unreserved path segment (no slash, brace, percent or dot-only name) and a duplicate collection path or an invalid component name is a diagnostic; uint32 is int64 with minimum 0, uint64 has no format and minimum 0, and uint8 and uint16 are int32 with minimum 0; the base Go type is carried in the property instead of being looked up by column (TestDocumentHostileTables, TestDocumentUnsignedWidths).
- Not done here: validation against the official meta-schema and the HTTP handlers (Task 14, story 221).

