# momo-10 Task 14: OpenAPI handlers and TestOpenAPIGenerator (validation and HTTP)

Status: open
Created: 2026-11-04
Epic: momo-10-ecosystem-generators
Sprint: -

## Description

Implements plan momo-10-ecosystem-generators Task 14: OpenAPI handlers and TestOpenAPIGenerator (validation and HTTP) (`.procoder/plans/momo-10-ecosystem-generators.md:1313`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-65. Files: `gen/openapi/handlers.go`, `gen/openapi/plugin.go`, `internal/integration/openapi/openapi_test.go`, `internal/integration/openapi/example/handlers/handlers_gen.go`, `internal/integration/openapi/testdata/oas31-schema.json`.

## Acceptance criteria

- [ ] `TestOpenAPIGenerator` exists as written in the plan, failed before the implementation, and passes now
- [ ] [S-65] `TestOpenAPIGenerator` validates the document against the OpenAPI 3.1 schema and exercises generated handlers with httptest; fails if the document and handlers disagree on a field.
- [ ] `procoder check` gate is clean and the task's commit is made

## Evidence

