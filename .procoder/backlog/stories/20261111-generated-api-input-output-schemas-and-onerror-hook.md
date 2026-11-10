# Generated APIs: separate input and output schemas, an error hook and cancellation codes

Status: open
Created: 2026-11-11
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Two gaps the second review of the ecosystem generators found and the fix round did not close.

The OpenAPI document and the gRPC messages use one schema for what a client sends and what the server returns. That is why the key, identity, read-only and defaulted columns had to be taken out of `required`, and why `id` shows up as an optional property that a create and an update ignore. A client generator then types `id` as optional on a response that always has it.

The generated servers also give the application no place to see what went wrong. An error that maps to 500 or Internal is answered with a fixed message and nothing else, so it cannot be logged with its cause, counted or traced, and a request the client cancelled is answered as a server error (HTTP 500, gRPC Internal) instead of 499 or Canceled.

## Acceptance criteria

- [ ] The OpenAPI document has an input schema per model (`<Model>Input`, the writable properties, `required` as today) and an output schema (`<Model>`, every column, `required` listing the not-null columns including the key); create and PUT reference the input, every response references the output; a test checks that `id` is required in the output and absent from the input, and the document still validates against the OpenAPI 3.1 meta-schema
- [ ] The gRPC service takes and returns the same message as today, with the field comments and the docs stating which fields a request ignores; a test pins the comment for the key and read-only fields
- [ ] The `openapi` plugin option `on_error` (a Go function name in the handlers package, signature `func(ctx context.Context, op string, err error)`) and the `grpc` equivalent are called once for every error that is answered, with the operation name and the original error, before the fixed message is written; the generated code compiles without the option, and a test with a failing statement shows the hook sees the original error while the client sees only the fixed message
- [ ] A request whose context was cancelled or whose deadline passed is answered with 499 (HTTP; 504 for a deadline) and codes.Canceled or codes.DeadlineExceeded (gRPC), and the GraphQL presenter answers `CANCELED`, each with a fixed message; a test per transport cancels the context mid-request
- [ ] `docs/capabilities.md` lists the new options and codes and `docs/upgrading.md` lists the changed response schema names
- [ ] `procoder check` is clean over the changed files and the ci-agent run on the final HEAD is green, with the run id recorded here

## Evidence
