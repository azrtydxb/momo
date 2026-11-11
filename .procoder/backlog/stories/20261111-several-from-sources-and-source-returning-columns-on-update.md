# Several FROM sources and source RETURNING columns on UPDATE and DELETE

Status: done 2026-11-11
Created: 2026-11-11
Epic: -
Sprint: 009-fix-every-open-github-issue-86-to-108-the-kuvryn-orbit-port
Type: bug
Severity: s2

## Description

GitHub issue #93. `UpdateQuery.From` and `DeleteQuery.Using` took exactly one
source, and `Returning` listed only the target model's columns, rendered
unqualified. Claim-and-return queues (`WITH due AS (... FOR UPDATE SKIP LOCKED)
UPDATE t SET ... FROM due, subscription s WHERE ... RETURNING t.id, s.url`)
could not be written and stayed raw SQL.

Reproduce: build an update with a CTE named due and a second table as sources,
`From(momo.CTE("due"), otherTable)`, and `Returning` the target id and the
other table's url. Observed: `From` takes one
argument, so the second source does not compile; a source column in `Returning`
was matched by name against the target's columns (a wrong column when the names
match, `ErrValidation` otherwise); RETURNING was unqualified, so a column that the
source and the target share was ambiguous on PostgreSQL. Expected: several sources
render as a comma list, `Returning` reads source and CTE columns qualified and the
rows scan into a projection or plain struct, and the unfiltered-mutation guard
keeps refusing a source that nothing constrains.

The issue also notes that docs/capabilities.md said a DML body in a CTE needs
Returning although write_with.go does not enforce it. A body that nothing reads
is valid PostgreSQL without Returning, so the code stays as it was and the docs
now say that Returning is needed only when the outer statement reads the CTE.

Root cause: the builders stored one `TableSource` and `renderReturning` /
`returningIndexes` knew only the target's `Meta` columns.

## Acceptance criteria

- [x] a regression test pins the fix: red before the change, green after
- [x] `From` and `Using` take several sources and render them as a comma list on PostgreSQL, SQLite (`From`) and MySQL (multi-table form)
- [x] `Returning` accepts source and CTE columns, rendered qualified on PostgreSQL, and `ScanUpdate` / `ScanDelete` scan them into a plain struct
- [x] with two or more sources the guard refuses a source that no `Where` predicate references (`ErrUnfilteredMutation`) and `AllRows` still confirms it
- [x] SQLite and MySQL refuse a source column in RETURNING with `ErrUnsupported`; `Rows` refuses it with `ErrValidation`
- [x] an unreferenced DML CTE body without `Returning` still builds on PostgreSQL (TestUnreadDMLCTEBodyWithoutReturningBuilds), and the docs say Returning is needed only when the CTE is read
- [x] docs updated: `docs/ai-guide.md`, `docs/capabilities.md`, `docs/upgrading.md`

## Evidence

- Red: ci-agent run 42979367881 (branch worktree-agent-red-93, tests only, sha 036b1cd). The unit job failed to build: "too many arguments in call to ... From" and "... Using" at write_from_multi_test.go lines 34, 44, 57, 113 and 131, plus undefined momo.ScanUpdate and momo.ScanDelete in write_from_multi_integration_test.go. The new TestDMLCTEBodyNeedsReturning is in the same red tree, hidden behind the build failure.
- Green: ci-agent run 42988441475, success, headSha 001e3aded61a87de1e2106f3c78fa0776542d33b (all jobs, including integration postgres, integration-mysql, lint and apidiff-docs).
- Tests: TestUpdateFromTwoSourcesSQL, TestReturningSourceColumnsSQL, TestReturningSourceColumnsRefused, TestFromUsingGuardSeveralSources, TestUnreadDMLCTEBodyWithoutReturningBuilds, TestFromUsingGuardSeveralSourcesCombinators, TestUnnamedSourceIsUnconstrained (unit); TestUpdateFromTwoSourcesReturningIntegration, TestReturningSharedNameQualifiedIntegration, TestDeleteUsingTwoSourcesReturningIntegration (PostgreSQL integration).
- Docs changed: docs/ai-guide.md, docs/capabilities.md, docs/upgrading.md (Upgrading to 0.8.0), docs/reference.md (from the regenerated-docs artifact of run 42984977321).
