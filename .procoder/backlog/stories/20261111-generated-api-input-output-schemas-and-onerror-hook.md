# Generated APIs: separate input and output schemas, an error hook and cancellation codes

Status: done 2026-11-10
Created: 2026-11-11
Epic: momo-10-ecosystem-generators
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

Two gaps the second review of the ecosystem generators found and the fix round did not close.

The OpenAPI document and the gRPC messages use one schema for what a client sends and what the server returns. That is why the key, identity, read-only and defaulted columns had to be taken out of `required`, and why `id` shows up as an optional property that a create and an update ignore. A client generator then types `id` as optional on a response that always has it.

The generated servers also give the application no place to see what went wrong. An error that maps to 500 or Internal is answered with a fixed message and nothing else, so it cannot be logged with its cause, counted or traced, and a request the client cancelled is answered as a server error (HTTP 500, gRPC Internal) instead of 499 or Canceled.

## Acceptance criteria

- [x] The OpenAPI document has an input schema per model (`<Model>Input`, the writable properties, `required` as today) and an output schema (`<Model>`, every column, `required` listing the not-null columns including the key); create and PUT reference the input, every response references the output; a test checks that `id` is required in the output and absent from the input, and the document still validates against the OpenAPI 3.1 meta-schema
- [x] The gRPC service takes and returns the same message as today, with the field comments and the docs stating which fields a request ignores; a test pins the comment for the key and read-only fields
- [x] The `openapi` plugin option `on_error` (a Go function name in the handlers package, signature `func(ctx context.Context, op string, err error)`) and the `grpc` equivalent are called once for every error that is answered, with the operation name and the original error, before the fixed message is written; the generated code compiles without the option, and a test with a failing statement shows the hook sees the original error while the client sees only the fixed message
- [x] A request whose context was cancelled or whose deadline passed is answered with 499 (HTTP; 504 for a deadline) and codes.Canceled or codes.DeadlineExceeded (gRPC), and the GraphQL presenter answers `CANCELED`, each with a fixed message; a test per transport cancels the context mid-request
- [x] `docs/capabilities.md` lists the new options and codes and `docs/upgrading.md` lists the changed response schema names
- [x] `procoder check` is clean over the changed files and the ci-agent run on the final HEAD is green, with the run id recorded here

## Evidence

- Red: the tests were committed first (9bce415). ci-agent run 42828927513 (throwaway branch worktree-agent-acf2b-red-gen) unit job FAIL TestInputAndOutputSchemas, TestOperationsReferenceTheRightSchema, TestRequiredProperties, TestFieldNotesStateWhatARequestIgnores and TestCancellationStatuses. The cancellation tests of the three transports (nested module) did not run there because make stops at the first failing module, so they were run alone against the same commit on throwaway branch worktree-agent-acf2b-red-cancel, run 42839333874: FAIL TestCancelledRequestIsNot500 (openapi), TestCancelledCallIsNotInternal (grpc), TestPresenterMapsContextErrors and TestCancelledRequestIsCanceled (graphql). The on_error hook tests (TestOnErrorOption, TestServerOnError, TestPluginOnError, TestOnErrorHookSeesTheOriginalError in openapi and grpc) use API that did not exist, so before the change they could only fail to compile.
- Implementation bffc636 and c1e1da6 (the first commit left out the Input schema, the Input body reference and the 499/504 responses because of an edit that did not apply; ci-agent run 42830494932 on be0a15c failed those tests and c1e1da6 fixed it). Output schema `<Model>` lists the not-null columns including the key in `required`; `<Model>Input` holds the writable properties, and create and PUT reference it; responses reference `<Model>`. Examples, goldens and the OpenAPI document come from CI regen runs 42829010774 and 42835741955; the document validates against the OpenAPI 3.1 meta-schema in TestOpenAPIGenerator.
- docs/capabilities.md lists the on_error option, the schemas and the codes; docs/upgrading.md lists the changed schema names, the hook and the 499/504, Canceled/DeadlineExceeded and CANCELED codes.
- Green: ci-agent run 42839158680 (sha 5e86d37, all 9 jobs success), including TestInputAndOutputSchemas, TestOnErrorHookSeesTheOriginalError, TestCancelledRequestIsNot500, TestCancelledCallIsNotInternal and TestCancelledRequestIsCanceled.
- Review round 2026-11-10 (fixed in ccfd470, tests first in 7ee9b7b, red on ci-agent run 42843378815): the on_error hook is called through a generated runErrorHook that recovers a panic (TestOnErrorHookPanicIsRecovered in gen/openapi and gen/grpc; there is no runtime test that a panicking hook still gets the request answered); the `<Model>Input` name clash is checked only for a model with a create or update route (TestInputNameClashOnlyWhenTheInputSchemaIsWritten); the openapi plugin reports a diagnostic for on_error without handlers (TestOnErrorWithoutHandlersIsReported); the stale ServerSource doc about reading the row first is fixed (TestServerSourceDocDoesNotClaimAReadFirst); the GraphQL presenter falls back to the request context (TestPresenterUsesTheContextAsAFallback, red on run 42843393686, throwaway branch worktree-agent-acf2b-red-review-gql). Generated files from regen run 42843735549.
