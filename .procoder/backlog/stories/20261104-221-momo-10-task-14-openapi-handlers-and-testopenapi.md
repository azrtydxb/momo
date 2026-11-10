# momo-10 Task 14: OpenAPI handlers and TestOpenAPIGenerator (validation and HTTP)

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Implements plan momo-10-ecosystem-generators Task 14: OpenAPI handlers and TestOpenAPIGenerator (validation and HTTP) (`.procoder/plans/momo-10-ecosystem-generators.md:1313`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-65. Files: `gen/openapi/handlers.go`, `gen/openapi/plugin.go`, `internal/integration/openapi/openapi_test.go`, `internal/integration/openapi/example/handlers/handlers_gen.go`, `internal/integration/openapi/testdata/oas31-schema.json`.

## Acceptance criteria

- [ ] `TestOpenAPIGenerator` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-65] `TestOpenAPIGenerator` validates the document against the OpenAPI 3.1 schema and exercises generated handlers with httptest; fails if the document and handlers disagree on a field.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence
- Built: gen/openapi/handlers.go, plugin.go, internal/integration/openapi/openapi_test.go, testdata/oas31-schema.json and oas31-dialect-base.json with SHA256SUMS, committed example handlers and openapi.yaml, gen/openapi/handlers_test.go. The Error schema property is now `error` (it was `message`), so the document matches the handlers' `{"error": ...}` bodies.
- Deviation from the plan: the plugin writes openapi.yaml, not openapi.json, because Writer.Text requires a generated-file header and JSON has no comments; the YAML is the same document (TestPluginGenerate compares them). 
- [S-65] TestOpenAPIGenerator: document valid against the OpenAPI 3.1 meta-schema, committed outputs current, GET keys equal the Document schema properties, POST/GET/PUT/list filter/keyset paging/400/DELETE 204/404 over httptest on PostgreSQL. CI run 42779257792 (headSha b6b30a8): unit, integration (postgres), integration-mysql (mysql84, mysql97, mariadb), fuzz-smoke, bench-module and apidiff-docs green; lint red only on gosec of the protoc stubs (fixed in the next commit). TestGraphQLGenerator, TestProtoGenerator and TestOpenAPIGenerator PASS in run 42774579231 (regen-tmp, commit 41a3405, PostgreSQL with MOMO_REQUIRE_INTEGRATION=1 and the pinned protoc).
- Open: criterion 1 not ticked. Red proof: the tests were written with the code, so no run failed on `undefined` alone; per the 2026-11-08 decision a throwaway red branch (implementation reverted) is still needed to tick this criterion.
