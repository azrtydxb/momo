# Add momo/obs: ready-made hooks (recorder, N+1 detector, slog)

Status: done
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: 006-sprint-6-split-the-root-package-into-subpackages-and-keep

## Description

The brief asked for tenant filter and N+1 hook to move to `momo/obs`; reading the code shows neither exists in the root (section 3 of `.procoder/research/package-split.md`). The root has only the hook interfaces, which must stay because `DB` calls them. This story adds the missing implementations as a new, non-breaking package. Promote `internal/obstest.Recorder` (implements `momo.Hook`), add an N+1 detector built on `momo.RelationHook` and `QueryEvent` that reports when one parent load triggers per-row child queries, and a `log/slog` hook that honours `RedactedArg`. Tenant isolation stays row-level security through `WithTxSetup`; no tenant helper is added here. Depends on story export-expression-extension-api only for the architecture list.

## Acceptance criteria

- [x] `obs.Recorder` records query events and relation events, is safe for concurrent use, and `internal/obstest` re-exports it so existing tests keep working
- [x] `obs.NPlusOne` reports a finding when a loop issues the same relation load statement more than a threshold number of times in one context, and stays silent for one batched `Load` (unit test with the fake driver, plus a PostgreSQL integration test)
- [x] the `slog` hook never logs a value bound from a sensitive column (test uses a sensitive column and asserts the log output contains `[redacted]`)
- [x] `obs` imports only `momo` and the standard library (architecture test green); no root file changes
- [x] `docs/ai-guide.md` has a section on hooks that names the package; `apidiff` shows additions only

## Evidence

- Red: ci-agent run 42600801428 on the throwaway branch `worktree-agent-ab65fec48e7c4efa8-red` (compiling stubs plus the tests, branch deleted afterwards): unit failed in TestRecorder*, TestNPlusOne and TestSlog* with behavioural failures (for example "want 1 finding, got []", "log lacks [redacted]").
- Green: ci-agent run 42617487009, headSha ed5a85a (all jobs success), the last code commit; the only later commit changes this story file.
- Recorder: `obs.Recorder` records query and relation events, copies Args, tested concurrently (`TestRecorderConcurrent`); `internal/obstest` aliases `obs.Event` and `obs.Recorder`, and its existing users compile and pass unchanged.
- N+1: `obs.NPlusOne` counts relation loads per relation in an `obs.WithScope` context through `RelationHook`; fake-driver tests (`TestNPlusOne`) and the PostgreSQL test `TestNPlusOneAgainstDatabase`. The root already has `NewNPlusOneHook` (statement shapes); this one counts loads and is the loop-over-`Load` detector the story describes.
- slog: satisfied through the existing `momolog` package, not a second API. `momolog` already was the repo's slog hook, so `obs` adds no duplicate; `momolog.New` now also implements `momo.RelationHook` (relation load records). `TestNeverLogsSensitiveValue` (`momolog/momolog_unit_test.go`) binds a sensitive column with `Values(true)` and `WithArgLogging`, and asserts `[redacted]` is present and the value absent; `TestValuesAreOptIn`, `TestLogsErrors` and `TestLogsRelationLoads` cover the opt-in rule, the err attribute and the relation record. The behaviour was red-proved first as `obs.NewSlog` (run 42600801428), then moved to `momolog`.
- Architecture: `feature obs` in `internal/repocheck/testdata/packages.txt`, `TestFeaturePackagesImportOnlyAllowed` green; no root file changed.
- Docs: `docs/observability.md` (new, in the README index and mkdocs nav), `docs/ai-guide.md` hooks bullet, `docs/ARCHITECTURE.md`; the apidiff-docs job is green.
