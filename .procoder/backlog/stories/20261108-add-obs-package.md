# Add momo/obs: ready-made hooks (recorder, N+1 detector, slog)

Status: open
Created: 2026-11-08
Epic: momo-14-structure-and-docs
Sprint: -

## Description

The brief asked for tenant filter and N+1 hook to move to `momo/obs`; reading the code shows neither exists in the root (section 3 of `.procoder/research/package-split.md`). The root has only the hook interfaces, which must stay because `DB` calls them. This story adds the missing implementations as a new, non-breaking package. Promote `internal/obstest.Recorder` (implements `momo.Hook`), add an N+1 detector built on `momo.RelationHook` and `QueryEvent` that reports when one parent load triggers per-row child queries, and a `log/slog` hook that honours `RedactedArg`. Tenant isolation stays row-level security through `WithTxSetup`; no tenant helper is added here. Depends on story export-expression-extension-api only for the architecture list.

## Acceptance criteria

- [ ] `obs.Recorder` records query events and relation events, is safe for concurrent use, and `internal/obstest` re-exports it so existing tests keep working
- [ ] `obs.NPlusOne` reports a finding when a loop issues the same relation load statement more than a threshold number of times in one context, and stays silent for one batched `Load` (unit test with the fake driver, plus a PostgreSQL integration test)
- [ ] the `slog` hook never logs a value bound from a sensitive column (test uses a sensitive column and asserts the log output contains `[redacted]`)
- [ ] `obs` imports only `momo` and the standard library (architecture test green); no root file changes
- [ ] `docs/ai-guide.md` has a section on hooks that names the package; `apidiff` shows additions only

## Evidence
