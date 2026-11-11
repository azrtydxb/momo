# Registry: the SQL of Insert.Model depends on the model's values

Status: done 2026-11-10
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`QueryRegistry` builds a statement without a database, but `Insert.Model(&m)` omits zero-valued dbdefault columns, so the registered SQL differs from the executed SQL unless the sample model is set as a real call sets it. Document this in the registry section, and give the typed insert an explicit column list if it has none, so its SQL shape can be fixed. GitHub issue #79.

## Acceptance criteria

- [x] the tests are shown failing on CI before the change
- [x] the registry docs state that `Insert.Model` SQL depends on dbdefault field values and show how to register a stable shape
- [x] if the typed insert lacks an explicit column list, one is added with a test that its SQL does not depend on values

## Evidence

- The typed insert already has explicit column lists that do not depend on values: InsertQuery.Values lists exactly the assigned columns, and Omit fixes which dbdefault columns Model leaves out. So no new column-list method was added; the criterion "if the typed insert lacks an explicit column list" does not apply. The remaining shape dependence is Model itself, which is documented.
- Docs: docs/testing.md "Listing the SQL: a query registry" (the dependence and three ways to register a stable shape), docs/ai-guide.md registry paragraph, docs/upgrading.md 0.7.0. Green docscheck on ci-agent run 42911526147 (dcbc1b1) and run 42915567322 (c4cb09a).
- Tests in raw_registry_insert_shape_test.go: TestRegistryInsertModelSQLDependsOnDBDefaultValues, TestRegistryInsertValuesSQLDoesNotDependOnValues, TestRegistryInsertModelOmitFixesTheShape. They are green on run 42915567322. They describe existing behavior, so they were green from the start (run 42904412952 for the red commit). To show they catch a break, throwaway branch worktree-agent-ad3b-mutation (deleted) broke Values (drops all but one assignment), Omit (ignored) and the dbdefault drop, and ci-agent run 42912148728 failed exactly those three tests in the unit job.
