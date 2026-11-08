# momo-11 Task 3: Bun static reader and fixture module

Status: done 2026-11-08
Created: 2026-11-04
Epic: momo-11-adoption
Sprint: -

## Description

Implements plan momo-11-adoption Task 3: Bun static reader and fixture module (`.procoder/plans/momo-11-adoption.md:267`). The plan task is the executable specification: follow its steps in order (failing test first, then the implementation, then the passing run) and its Interfaces exactly. Spec coverage: S-91. Files: `importer/bun/read.go`.

## Acceptance criteria

- [x] `TestImportBun` exists (adapted as listed under Deviations: PostgreSQL instead of SQLite, schema built by the real generator), failed before the implementation, and passes now
- [x] `TestReadReportsNullzeroWithPosition` exists as written in the plan, failed before the implementation, and passes now
- [x] [S-91, part] `TestImportBun` passes for the part of S-91 this task delivers; the full criterion is verified in story 227.
- [x] `procoder check` gate is clean and the task's commit is made

## Evidence

- Red: ci-agent run 42467843322 on dc41272 (tests, fixture module and Makefile only, on top of sprint-5) failed: `importer/bun/read_test.go: undefined: Read` and `undefined: tag` in the unit and integration jobs. The fixture's `TestImportBun` needs `importerbun.Read`, which did not exist either; the integration job stopped at the root module's compile error before it reached the fixture module, so the fixture test's red is the same missing function. The throwaway branch is deleted.
- Green: ci-agent run 42469906666 on 3b0bff3 (all jobs: lint including vet and tidy-check of the new fixture module, unit, fuzz-smoke, integration postgres with MOMO_REQUIRE_INTEGRATION=1, and the three MySQL jobs). The integration log shows `ok github.com/azrtydxb/momo/importer/bun` and `ok github.com/azrtydxb/momo/internal/compat/bunfixture`: Bun's own `db.NewCreateTable().WithForeignKeys()` creates the schema on PostgreSQL, the imported models are emitted and loaded by the real generator (gen.Load), and migrate.Diff against Bun's schema reports exactly one difference, the composite unique constraint `uq_profile_region_slug` the importer reports as dropped-index.
- importer/bun/read_test.go: the plan's `TestReadReportsNullzeroWithPosition` (position `models.go:<line>:<col>`), a determinism and no-Unsupported/no-ImplicitSave check on the fixture, Bun tag parser cases, the Go type to pgdialect column type table, serial case rules, default table names, unique groups, embedded structs, belongs-to resolution, every unsupported path, hooks and options, named types and import aliases.
- Bun semantics were checked against github.com/uptrace/bun v1.3.0 and dialect/pgdialect v1.3.0 (fetched at the tag): schema/table.go (field and relation processing, embedded ambiguity, unique groups, foreign key rules, `References()`), schema/sqltype.go and dialect/pgdialect/sqltype.go and dialect.go (type mapping and the serial rule), internal/tagparser (ported verbatim into importer/bun/tag.go), internal/underscore.go and jinzhu/inflection (gen/naming.Bun) and query_table_create.go (what CreateTable emits).
- The commit gate (procoder check) ran on each commit of this story and passed; the nested fixture module is linted through an untracked go.work, which CI never sees.

## Deviations from the plan

- The fixture compares against PostgreSQL (pgdialect, pgx stdlib), not SQLite: only the PostgreSQL migration dialect exists. The plan's test imported two packages both named `bun`; the importer is imported as `importerbun`. The test schema is built with `db.NewCreateTable().Model(m).WithForeignKeys()`. The fixture has a third model (Profile) covering int32, bool, bytes, a `type:` override, an `on_delete` action, a soft-delete column, an `embed:` prefix and a composite unique group.
- The plan says relation tags are reported as `implicit-save`. In Bun v1.3.0 that is false: relation fields are kept out of the column list (`Table.addField` puts them in `relFields`) and Insert and Update never write them, so reporting implicit-save would mislead. No implicit-save is reported (the test asserts it). What is reported instead for a belongs-to relation is a `naming` change: Bun creates the foreign key only when the table is created with WithForeignKeys() (and then only if the key column is not part of the primary key, or the model is a many-to-many junction) or by a migration, while the import always declares `<table>_<column>_fkey`, so a database without it gets the constraint on the next migrate diff. `rel:has-one`, `rel:has-many`, `m2m:` and polymorphic fields are reported as `dropped-field` (no column, Momo declares relations on the model that holds the key); the junction struct of an m2m is imported as a model.
- `scanonly` is reported as `dropped-field` (no column, never written) rather than being the exception to a relation report.
- Plan tags not read: `m2m:` (reported, see above). Added beyond the plan: `embed:`/anonymous embedding with Bun's ambiguity rules, `unique:group`, `column:`, `autoincrement`, `on_delete`/`on_update`, `join:` for belongs-to, hooks.
- Bun semantics that differ from GORM and are followed exactly: only `pk` marks a key (a field named ID is not a key); a plain `pk` is not auto-increment; every column is nullable unless `notnull`, `pk` or `autoincrement`; `default:` is raw SQL taken verbatim (no string quoting); `autoincrement` becomes a serial type only for the upper-case types SMALLINT, INTEGER and BIGINT, so `type:bigint` in lower case or a non-integer type keeps the plain type and Bun creates no sequence (reported as `naming`, the import creates none either); a struct, map or slice column is jsonb; sql.NullInt32/NullInt16/NullByte map to jsonb in v1.3.0 (reported as unsupported unless a `type:` is given); a type defined over a qualified type is mapped by its kind.
- Other reports: `nullzero` on a value type (`zero-value-handling`), `BeforeAppendModel` hook (`go-default`), scan hooks (`implicit-save`), `BeforeCreateTable`/`AfterCreateTable` hooks (`unsupported`, they can change the DDL), `soft_delete` and `bun.NullTime`/named types (`renamed-type`), `SetTableNameInflector` calls (`naming`), composite or repeated unique constraints (`dropped-index`), `alt`/`skipupdate`/`json_use_number` (`dropped-field`), `array`/`hstore`/`composite`/`multirange`/`msgpack`/`identity`, unknown options, a pointer type with `notnull`, composite belongs-to, a relation that references a non-key column, embedded structs from other packages, schema-qualified table names, `select:` and values a momo tag cannot carry (a comma, for example `numeric(10,2)`) as `unsupported`.
- Known limits, stated in the Read doc comment: indexes, foreign keys and columns created outside the struct tags (db.NewCreateIndex, migrations, AfterCreateTable) are not visible to a static reader; `identity` columns and `numeric(p,s)` types cannot be carried by importer.Emit and are reported as unsupported rather than guessed.
- The fixture module's go.mod and go.sum were produced by a temporary GitHub Actions job (`go mod tidy`); bun and pgdialect v1.3.0 (published 2026-10-01), pgx v5.12.0. The throwaway branch is deleted.

## Scope of this story

The `momo import bun` command and the importer documentation page belong to story 228 (the import command), and the migration guides to stories 237 and 238; neither is part of this reader and fixture story. Story 227 verifies the full S-91 criterion.
