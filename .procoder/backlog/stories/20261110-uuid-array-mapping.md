# Types: uuid[] mapping and typed array operators on UUIDs (issue #57)

Status: done
Created: 2026-11-10
Epic: momo-05-writes-queries
Sprint: 007-sprint-7-finish-every-open-backlog-story-auditing-each

## Description

A `uuid[]` column has no Go mapping, so it needs a `type:` override and `arrayop` predicates on it are untyped. Apps hold such columns (`department_ids`) and query them with `= ANY($1::uuid[])`. The ask is in GitHub issue #57, from the Kuvryn Owl Enterprise port.

## Acceptance criteria

- [x] `[]types.UUID` maps to `uuid[]` on PostgreSQL (generator, inspector, `--from-db`), with a round-trip integration test (insert, select, nil vs empty)
- [x] `[]uuid.UUID` from github.com/google/uuid maps too if it needs no new dependency in the root module; otherwise the story records why not
- [x] `arrayop.AnyOf`, `Overlaps` and `Contains` are typed on the UUID array column and render `= ANY($1::uuid[])` / `&&` / `@>` (unit tests pin SQL, integration test runs them)
- [x] the generated type-mapping table and the docs list the mapping; issue #57 is referenced in the commit

## Evidence

CI: ci-agent run 42778745331, conclusion success, headSha 810772720409f6f94b76b89c13e649f630cc5237 (all 9 jobs).

- Criterion 1: gen/typemap/typemap.go (postgresElem), gen/fromdb/mapping.go (postgresArrays, `uuid[]` -> `[]types.UUID`), tests gen/typemap/typemap_test.go, gen/fromdb/mapping_test.go, gen/array_columns_test.go; live round trip (values, empty slice, nil) is TestTypeMapping/postgres/[]types.UUID in col_type_mapping_integration_test.go. The driver sends a []types.UUID as a uuid[] text literal (driver/pgxdriver/vector_codec.go convertUUIDArrayArgs, unit test uuid_array_test.go), because pgx cannot encode the elements of a driver.Valuer slice.
- Criterion 2: []uuid.UUID is not mapped. The scalar uuid.UUID has no mapping either, and github.com/google/uuid is only an indirect dependency of the root module (go.mod), so mapping it would add a direct dependency. Use types.UUID or a `type:` override. Recorded in docs/TYPE_MAPPING.md.
- Criterion 3: arrayop/arrayop.go castFor renders `::uuid[]`; SQL pinned in TestArrayOperatorsGolden, live in TestArrayOperatorsOnUUIDLive (arrayop/arrayop_integration_test.go).
- Criterion 4: docs/TYPE_MAPPING.md row `[]types.UUID`, docs/ai-guide.md, docs/capabilities.md; TestTypeMappingDoc and gen-check pass. Commits reference #57.
