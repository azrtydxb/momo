# Registry: the SQL of Insert.Model depends on the model's values

Status: open
Created: 2026-11-10
Epic: -
Sprint: 008-sprint-8-fix-the-kuvryn-orbit-port-issues-73-to-79-and-the
Type: story

## Description

`QueryRegistry` builds a statement without a database, but `Insert.Model(&m)` omits zero-valued dbdefault columns, so the registered SQL differs from the executed SQL unless the sample model is set as a real call sets it. Document this in the registry section, and give the typed insert an explicit column list if it has none, so its SQL shape can be fixed. GitHub issue #79.

## Acceptance criteria

- [ ] the tests are shown failing on CI before the change
- [ ] the registry docs state that `Insert.Model` SQL depends on dbdefault field values and show how to register a stable shape
- [ ] if the typed insert lacks an explicit column list, one is added with a test that its SQL does not depend on values

## Evidence
