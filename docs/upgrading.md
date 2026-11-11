# Upgrading

Momo is pre-v1 and makes no compatibility promise between releases; see the
[stability policy](supported-versions.md#stability-policy). Read this page and
the `CHANGELOG.md` entry of a release before you upgrade, and expect that
later releases can break more.

## Upgrading to 0.8.0

### `momovet` accepts a `//momovet:ok <reason>` directive

A raw-SQL call whose text is built from constants and allowlisted identifiers can now be marked as vetted with a line directive: a trailing `//momovet:ok <reason>` comment on the call's line, or the same comment alone on the line directly above it. A trailing comment never covers the next line. The directive silences the report for that call only. A directive without a reason, or one that suppresses nothing, is reported itself, so add a reason to every directive. See [momovet](momovet.md).

## Upgrading to 0.7.0

### `--from-db` writes only the functions the modeled tables use

`momo generate --from-db` used to write every database function into the
`MomoConfig` of the alphabetically first model. It now writes only the
functions that a modeled table uses: the ones its triggers call, its column
defaults, generated columns, checks, index expressions and predicates and its
policies call, and the functions those functions call in turn (their bodies and
argument defaults). A call is found by name, and so is a `'name'::regproc` or
`'name(args)'::regprocedure` literal (also `pg_catalog`-qualified, schema
qualified or with quoted names) and a `to_regproc('name')` call. Each one is written on the model of the
table that uses it. A function that no modeled
table uses is not written, and it is no longer in the init snapshot, so a
database-wide function that nothing modeled needs does not appear in the
models, and the model files no longer change when an unrelated function does.

Triggers and policies moved the same way: they are written on the model of
their own table instead of the first model. A function that a view calls stays
on the first model, with the view, unless a modeled table also uses it: then it
goes on that table's model. After you regenerate, a model file that held
other tables' functions changes (a `.new` file next to a hand-edited one, see
"Running it again" in [migrations](migrations.md)). A function that was written
and is no longer used is not in the new init snapshot either; add it to
`migrate.exclude` if the database keeps it and you do not want drift to report
it as unmanaged.

SQL that is built at run time cannot be followed. A function that another
function reaches only through `EXECUTE` with a name put together from pieces
(`EXECUTE 'SELECT ' || prefix || '_target()'`), or through any other dynamic
SQL, is not written, and a fresh database built from the models then lacks it.
If your functions do that, use `introspect.functions: all`, which keeps the old
behaviour:

```yaml
introspect:
  functions: all # every database function on the first model
```

### `introspect.only` limits `--from-db` to a list of tables

`introspect.only` is a list of table name globs, the inverse of
`introspect.skip`. When it is set, a table that matches none of the globs gets
no model, no place in the init snapshot, and `momo migrate` leaves the live
table alone (drift, baseline and diff treat it as excluded), so you do not need
a `migrate.exclude` entry for every table you have not moved yet:

```yaml
introspect:
  only: [accounts, projects, project_*]
  skip: [project_notes] # skip applies within only
```

A glob matches the table name, or `schema.name` for a table outside the
current schema (`only: ["app.*"]`), the same way `migrate.exclude` does. A table
in the current schema (usually `public`) has an empty schema in the model, so
only its bare name is tried: `only: [public.users]` matches nothing when
`public` is the current schema; write `only: [users]`. An empty entry in `only`
is an error. `skip` applies within `only`: a table must match `only` and not
match `skip`. A skipped table still needs its `migrate.exclude` entry. An empty
or absent `only` changes nothing.

If `only` is set and no table matches, `--from-db` fails with an error that
says `introspect.only` matched no table, instead of reporting an empty
database. When some tables match, a glob that matched none is named in a
warning, so a typo does not hide in the rest of the list.

What `only` hides, and what it does not:

- It hides tables and, with them, their triggers and policies. Inspection does
  not read a table outside `only`, so a table the schema model cannot represent
  no longer fails `--from-db`, `baseline` or `drift`. `migrate diff` ignores
  such a table in the snapshot as well, so a snapshot that still holds one
  never plans `DROP TABLE` for it.
- A partition child is a table of its own, so `only: [orders]` leaves out
  `orders_2024`. Name the children too: `only: [orders, "orders_*"]`.
- A model for a table outside `only` (one you wrote by hand) is never migrated:
  `migrate diff` plans neither a `CREATE TABLE` for it nor a `DROP TABLE` for a
  snapshot table that is outside `only`.
- Views, functions, enums and extensions of the modules you have not moved are
  not hidden: drift still lists them as unmanaged, and `--from-db` still writes
  the views and enums. Hide them with `migrate.exclude` (globs over the name),
  for example `exclude: [legacy_*, audit_log_view]`.
- A foreign key from a modeled table to a table outside `only` is left out of
  the models and the snapshot, with a warning, and the baseline reports it as
  a difference. Move the target table into `only`, or accept the report. The
  column itself is still modeled, without the relation.

Library users: `migrate.Config.OnlyTables` and `migrate.InspectOptions.Only`
are new fields. `OnlyTables` is what `introspect.only` sets: when it is not
empty, a table that matches none of its globs is hidden from drift, baseline
and diff, with its triggers and policies, and the PostgreSQL inspector does not
read it. The MySQL and SQLite inspectors ignore `migrate.InspectOptions.Only` and read
every table, which is filtered afterwards, so a table they cannot represent can
still fail inspection. `migrate.MatchObject` is the glob rule shared by
`Exclude`, `OnlyTables` and the inspector. An empty or malformed entry is an
error that names `migrate.Config.OnlyTables`.

### `skip` globs match `schema.name`

`introspect.skip` globs now match the same way `introspect.only` and
`migrate.exclude` do: the bare table name, or `schema.name` for a table outside
the current schema. A schema-qualified `skip` glob (`skip: ["app.legacy_*"]`)
that did nothing before, because it was compared with the bare table name only,
now takes effect and drops those tables from the models and the init snapshot.
Check your `skip` list for such a glob before you regenerate.

### `introspect.dbdefault: all` tags every column that has a default

By default `--from-db` writes `dbdefault` only on NOT NULL columns whose
default is computed (`now()`, `gen_random_uuid()`), so regenerating does not
change what an existing model writes. A column such as
`version bigint NOT NULL DEFAULT 1` is generated as `` `momo:"default:1"` ``,
and `InsertQuery.Model` writes 0 for a model that left `Version` unset.
`introspect.dbdefault: all` adds `dbdefault` to every NOT NULL PostgreSQL column
that has a default, constant ones included, so an unset field takes the default
and the stored value is read back:

```yaml
introspect:
  dbdefault: all
```

A nullable column is never tagged, because a nil pointer would stop meaning
NULL. The zero value now means "use the default" on those columns: a `bool` or
number field can no longer write `false` or `0` through `Model` (use
`InsertQuery.Values`). Hand-written models can use the `dbdefault` tag on any
NOT NULL column that has a database default: see "Database defaults" in the
[AI guide](ai-guide.md).

### `--from-db` singularizes -se plurals correctly

`momo generate --from-db` turns a table name into a model name by trying
singular candidates and keeping the first one the naming preset pluralizes
back to the table name. The order was wrong for words in `-se`: `releases`
became `Releas`. The rule now is: a plural in `-sses`, `-xes`, `-zzes`,
`-ches`, `-shes` or `-uses` (but not `-ouses` or `-auses`) loses `es`; any
other plural in `-es` loses only the `s`; a short list of words that no
spelling rule separates (`aliases`, `biases`, `canvases`, `gases`, `atlases`,
`lenses`, `irises`, `tomatoes`, `buffaloes`, `potatoes`, `heroes`, `echoes`,
`vetoes`, `caches`, `niches`, `quizzes`, `uses`) is listed by name.

The generated model name changes, on the next regenerate, for these tables:

- Tables `releases`, `cases`, `courses`, `responses`, `licenses`, `bases`,
  `houses`, `purposes`, `causes`, `clauses` and `uses`: Releas, Cas, Cours,
  Respons, Licens, Bas, Hous, Purpos, Caus, Claus and Us become Release, Case,
  Course, Response, License, Base, House, Purpose, Cause, Clause and Use.
- Tables `sizes`, `prizes`, `mazes`, `caches` and `niches`: Siz, Priz, Maz,
  Cach and Nich become Size, Prize, Maze, Cache and Niche.
- Table `quizzes`: Quizze (gorm, bun) or Quizz (prisma) becomes Quiz under the
  `gorm`, `bun` and `prisma` presets; `momo` gives `Quizz` and `ent` gives
  `Quizze` (see below).
- Table `waltzes`: Waltz becomes Waltze, because a `-zes` word is read as `-ze`,
  as in `sizes`.

Not every preset can give every word the right name. The `gorm` and `bun`
presets pluralize a word that ends in `s` to itself, and `ent` matches its
rules case-sensitively on the Go name, so some words keep a wrong name there:
`ent` gives `Statuse`, `Buse`, `Quizze` and `Aliase`; `momo` gives `Quizz`
(it pluralizes `quiz` to `quizes`); `gorm`, `bun` and `ent` give `Biase`,
`Canvase`, `Gase`, `Atlase`, `Lense` and `Irise`; `-oes` words get `Tomatoe`
style names under presets with no `-oes` rule (`momo`, `ent`, and `gorm` and
`bun` for `potatoes`, `heroes`, `echoes` and `vetoes`). These were wrong before
too.

To keep an old name, or to fix a wrong one, pin it in `momo.yaml`; the pin wins
over the guess:

```yaml
introspect:
  tables:
    <table>: { name: Releas }
```

for example `releases: { name: Releas }`. A regenerate renames the struct in
the generated file, so update the code that uses it, or pin the old name first.

### `github.com/google/uuid` types need no type tag

A model or projection field of `uuid.UUID`, `*uuid.UUID` or `[]uuid.UUID` from
`github.com/google/uuid` now maps to `uuid` or `uuid[]` on PostgreSQL (`TEXT`
on SQLite; `[]uuid.UUID` has no SQLite column) with no `momo:"type:uuid"` tag.
A tag you already wrote is still honored and can stay or go. The generator
recognizes the type by its import path, so it adds no dependency to momo. Your
own module imports `github.com/google/uuid` only because your models already
do. `arrayop` predicates on a `[]uuid.UUID` column bind with a `::uuid[]`
cast, like `[]types.UUID`.

`momo generate --from-db` keeps writing `types.UUID` unless you set
`introspect.uuid: google` in `momo.yaml`; then every `uuid` column becomes
`uuid.UUID` (`*uuid.UUID` when nullable) and every `uuid[]` column
`[]uuid.UUID`, with no type tag and an import of `github.com/google/uuid`. An
`introspect.types` rule for `uuid` still wins. A `--from-db` re-run after you
set the switch regenerates the model files that nobody edited; edited files
conflict as usual.

The mapping is for PostgreSQL and SQLite. On MySQL these types are unsupported,
as `types.UUID` is, and `introspect.uuid` has no effect there. A named type
over `uuid.UUID` (`type ID uuid.UUID`) is not mapped; give it `type:uuid`.

### `Query` and `QueryRows` read into a plain struct

`momo.Query[T]` and `momo.QueryRows[T]` now accept any struct, with no
generated metadata: fields scan by column name (`momo:"column:x"` tag, else the
snake_case of the field name), and a result column with no field, or a field
with no result column, is an `ErrValidation` that names it. A model, projection
or `Row1`..`Row10` still scans through its generated metadata. The type
parameters changed from `[T any, PT Modeler[T]]` to `[T any]`: a call that
names only `T` compiles as before, and a call that named `PT` explicitly must
drop it. A type that is not a struct and has no metadata is now an
`ErrValidation` at run time instead of a compile error: `Query[*Task]` (a
pointer type) now fails at run time, not at compile time.

A struct without generated metadata, for example a model before `go generate`
has run or after a stale generation, now compiles and takes the reflection path
instead of failing to compile, so a stale or missing generation shows up only at
run time. Run `go generate`, and keep a test that calls `Query` for each model.

### New: `One` on writes with `RETURNING`

`InsertQuery.One`, `UpdateQuery.One` and `DeleteQuery.One` run the statement
and return the single returned row as `T`, so a sqlc `:one` style write no
longer needs the `Rows` / `ScanModel` / `Close` loop. Without a `Returning`
list it returns every column. It returns `ErrNotFound` when no row came back
(an `ON CONFLICT DO NOTHING` insert, or a guarded update or delete that matched
nothing) and `ErrTooManyRows` when more than one did.

Without `WithImplicitTx` or an explicit transaction the statement has already
changed every matched row; run `One` inside `db.Tx` and roll back on
`ErrTooManyRows`. `InsertQuery.One` is for a single-row insert: a multi-row
`Models` insert is `ErrValidation`, use `Rows`. `AfterLoad` runs before the
write commits, so an `AfterLoad` error rolls the write back, unlike
`SelectQuery.One`. MySQL has no `RETURNING` and returns `ErrUnsupported`;
momo has no `RETURNING` form for MariaDB `UPDATE` and `DELETE` (MariaDB itself
has `DELETE ... RETURNING`), so those return `ErrUnsupported`. This is
additive; nothing to change.

## Upgrading to 0.6.0

### The Using and From guard is stricter

`UpdateQuery.From` and `DeleteQuery.Using` refuse a statement whose `Where`
does not reference the target table, with `ErrUnfilteredMutation`. The guard
now follows `Or`, `Not` and `JoinSQL`: an `Or` counts only when every branch
references the target, an `And` when any term does, and `Not` swaps the two
rules. `JoinSQL` with `AND` or `OR` follows the same rules, and `JoinSQL` with
any other separator never counts. Statements that passed in 0.5.0 and are now
refused: `Where(momo.Or(PostCols.ID.Eq(1), SrcCols.Title.Eq("x")))`, where one
`Or` branch is off the target, and
`Where(momo.Not(momo.And(PostCols.ID.Eq(1), SrcCols.Title.Eq("x"))))`, which is
an `Or` of negations.

Add a condition on the target to every branch, or call `AllRows()` when the
statement is meant to touch every row. The guard checks that the condition
references the target, not that it restricts it: `t.id = t.id` still passes.

### OpenAPI error bodies use the key `error`

The OpenAPI document and the generated handlers now describe and send errors
as `{"error": "<message>"}`. The `Error` schema property was `message` in 0.4.0.
A client that reads `message` from an error response must read `error`. Error
messages are fixed strings: a constraint violation no longer shows the table or
constraint name.

### Generated handlers and servers

The ecosystem generators (`graphql`, `grpc`, `openapi`, enabled in the
`plugins:` section of `momo.yaml`) generate servers that do no authentication
and no authorization. Wrap the `http.ServeMux`, the `grpc.Server` or the
GraphQL handler. Turn off operations of the `openapi` and `grpc` servers with
`ops` or `read_only: true` (the `graphql` plugin has neither option); see the
[capabilities](capabilities.md) page for the options, the error mapping, the
update semantics and the GraphQL limits.

PUT and gRPC Update differ on purpose. An OpenAPI PUT must carry every required
property and writes only the properties it carries, so a property it leaves out
keeps whatever value the database has, including one a concurrent request just
wrote. A gRPC Update has no way to leave a scalar out (a proto3 scalar at its
zero value is a value), so it writes every writable column the message holds;
a vector, a timestamp or a defaulted scalar left unset is not written. Neither
reads the row first and writes it back.

### View models no longer produce a table

A model that embeds `momo.View` maps a view, so the generated schema, and with
it `momo migrate diff`, holds no table for it. A view model gets no
`CREATE TABLE`, and a foreign key never points at it. Declare the view itself
with `c.View` in `MomoConfig`.

The generated servers expose only list and get for a view model: there is no
create, update or delete route, RPC or mutation, and naming a write operation
for a view model in `ops` is a generator error.

If you add `momo.View` to a model whose table really exists in the database,
`momo migrate diff` now plans a `DROP TABLE` for that table (AutoMigrate skips
it). Review the diff before you apply it.

`momo.RefreshMaterializedView` refreshes a materialized view (PostgreSQL only).

### OpenAPI input and output schemas

The OpenAPI document now has two schemas per model. `<Model>` (for example
`Document`) is the output: every column, with the not-null columns, the key
included, listed in `required`. A client generator now types `id` as required
on a response. `<Model>Input` (`DocumentInput`) is new and is what a create and
a PUT take: only the writable properties, so it has no key and no read-only,
identity or serial column, and its `required` list is what it was on `<Model>`
in 0.5.0 (not null, no database default). Every response keeps referencing
`<Model>`; the request bodies of create and PUT now reference `<Model>Input`.
A client generated from the old document sends the same JSON and keeps working;
regenerate it to get the new types. A model whose name plus `Input` is also a
model name (`Doc` and `DocInput`) is a generator diagnostic.

The gRPC messages are unchanged: a model message is still what a client sends
and receives. The field comments in the `.proto` file say which fields a request
ignores (the key on Create, read-only columns on Create and Update, and the
version, soft-delete and timestamp columns on Update).

### Error hook and cancellation codes

The `openapi` and `grpc` plugins take the option `on_error`, the name of a Go
function in the generated package with the signature
`func(ctx context.Context, op string, err error)`. The generated code calls it
once for every error it answers from a statement (a database error, a missing
row, a cancelled context), with the operation (the `operationId` for OpenAPI,
for example `getDocument`; `DocumentService.Get` for gRPC) and the original
error, before it writes the fixed message. Log, count or trace the error there.
A panic in the hook is recovered, so it cannot stop the request from being
answered. The `openapi` option needs `handlers: true`, because only the
generated handlers call the hook. Without the option nothing changes.

A request whose context was cancelled is now answered 499 (OpenAPI), `Canceled`
(gRPC) and `CANCELED` (GraphQL) with a fixed message, and one whose deadline
passed 504, `DeadlineExceeded` and `CANCELED`. These were 500 and `Internal`
before. A client or proxy that retries on 500 will no longer retry them.

## Upgrading to 0.5.0

### Regenerating with `--from-db` after the `dbdefault` tag

`momo generate --from-db` now adds the `dbdefault` tag to every NOT NULL
PostgreSQL column whose default is computed (`gen_random_uuid()`, `now()`,
`CURRENT_TIMESTAMP`, `nextval(...)` and other function calls). A Go zero value
is no longer written for such a column: `InsertQuery.Model` and `InsertQuery.Models` leave it out,
the database default applies, and the value is read back with `RETURNING`.
Review the regenerated models: a `bool`, number, empty string or zero time in
such a field can no longer be written explicitly through `Model` (use `InsertQuery.Values`
or remove the tag), and a plain MySQL database refuses a zero value with
`ErrUnsupported` because it cannot read the default back. Hand-written models
are unaffected until they add the tag. See "Database defaults" in the
[AI guide](ai-guide.md).

### Advisory string helpers take options

`advisory.XactLockString` and `advisory.TryXactLockString` take a trailing
`...advisory.Option` (for `advisory.Hash64`). A call site is unchanged, but
the function types changed: code that assigns one of these functions to a
variable or field of the old func type no longer compiles. Wrap the call in a
closure instead.

### Model hook methods are now wired by `momo generate`

`momo generate` detects methods named `BeforeInsert`, `BeforeUpdate`,
`BeforeDelete` and `AfterLoad` on a model and fills `Meta.Hooks` with them.
Two behaviors change for an existing model after you regenerate:

- A method with one of these names and the exact signature
  `func(context.Context) error` now runs: before inserts, updates and deletes,
  and after loads. Before, such a method was never called. If you had a method
  with a hook name that was meant as plain code, rename it before you
  regenerate.
- A method with one of these names and any other signature makes
  `momo generate` fail with a positioned diagnostic
  (`hook BeforeInsert on Doc must have signature func(context.Context) error`),
  and a hook-named method on a projection is a diagnostic as well (a projection
  runs no hooks). Fix the signature or rename the method.

If you set `Meta.Hooks` by hand in a hand-written `Meta`, nothing changes.
Generated files are rewritten by `momo generate`; the `Hooks:` field appears
only for models that have a hook method.

### View models

A model that embeds `momo.View` (new) is marked as a view. Its writes fail with
`ErrValidation`. Existing models are unaffected unless you add the marker.

## Upgrading to 0.4.0

Release 0.4.0 moves feature-specific helpers out of the root package `momo`
into subpackages. Every move is a source break: the old identifier is gone and
the new one lives under a new import path. The release notes are the 0.4.0
entry of the [changelog](https://github.com/azrtydxb/momo/blob/main/CHANGELOG.md).

Why there is no alias: a Go method lives in the package of its receiver type,
and the subpackages import `momo`, so `momo` cannot import them back to keep
the old names. A move therefore deletes the old identifier outright.

### Identifier table

One row per moved identifier. When a method became a function, the old name is
the method and the new name is the function, which takes the former receiver
as an argument.

| Old identifier                      | New import path                      | New identifier                    |
| ----------------------------------- | ------------------------------------ | --------------------------------- |
| `momo.ArrayContains`                | `github.com/azrtydxb/momo/arrayop`   | `arrayop.Contains`                |
| `momo.ArrayContainedBy`             | `github.com/azrtydxb/momo/arrayop`   | `arrayop.ContainedBy`             |
| `momo.ArrayOverlaps`                | `github.com/azrtydxb/momo/arrayop`   | `arrayop.Overlaps`                |
| `momo.AnyOf`                        | `github.com/azrtydxb/momo/arrayop`   | `arrayop.AnyOf`                   |
| `momo.TSExpr`                       | `github.com/azrtydxb/momo/fts`       | `fts.Expr`                        |
| `momo.ToTSVector`                   | `github.com/azrtydxb/momo/fts`       | `fts.ToVector`                    |
| `momo.WebsearchToTSQuery`           | `github.com/azrtydxb/momo/fts`       | `fts.WebsearchToQuery`            |
| `momo.PlainToTSQuery`               | `github.com/azrtydxb/momo/fts`       | `fts.PlainToQuery`                |
| `momo.PhraseToTSQuery`              | `github.com/azrtydxb/momo/fts`       | `fts.PhraseToQuery`               |
| `momo.ToTSQuery`                    | `github.com/azrtydxb/momo/fts`       | `fts.ToQuery`                     |
| `momo.TSMatch`                      | `github.com/azrtydxb/momo/fts`       | `fts.Match`                       |
| `momo.TSRank`                       | `github.com/azrtydxb/momo/fts`       | `fts.Rank`                        |
| `momo.TSRankCD`                     | `github.com/azrtydxb/momo/fts`       | `fts.RankCD`                      |
| `momo.TSHeadline`                   | `github.com/azrtydxb/momo/fts`       | `fts.Headline`                    |
| `momo.Substr`                       | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Substr`                    |
| `momo.SubstrFrom`                   | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.SubstrFrom`                |
| `momo.Lower`                        | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Lower`                     |
| `momo.Upper`                        | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Upper`                     |
| `momo.Length`                       | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Length`                    |
| `momo.LengthExpr`                   | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.LengthExpr`                |
| `momo.CurrentSchema`                | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.CurrentSchema`             |
| `momo.CurrentSchemaExpr`            | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.CurrentSchemaExpr`         |
| `momo.Case`                         | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Case`                      |
| `momo.CaseExpr`                     | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.CaseExpr`                  |
| `momo.Numeric`                      | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Numeric`                   |
| `momo.ArithExpr`                    | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.ArithExpr`                 |
| `momo.Add`                          | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Add`                       |
| `momo.AddExpr`                      | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.AddExpr`                   |
| `momo.Sub`                          | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Sub`                       |
| `momo.SubExpr`                      | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.SubExpr`                   |
| `momo.Mul`                          | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Mul`                       |
| `momo.MulExpr`                      | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.MulExpr`                   |
| `momo.CoalesceExpr`                 | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.CoalesceExpr`              |
| `momo.Cast`                         | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Cast`                      |
| `momo.CastExpr`                     | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.CastExpr`                  |
| `momo.Concat`                       | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.Concat`                    |
| `momo.ConcatExpr`                   | `github.com/azrtydxb/momo/sqlfn`     | `sqlfn.ConcatExpr`                |
| `momo.JSONText`                     | `github.com/azrtydxb/momo/jsonop`    | `jsonop.Text`                     |
| `momo.JSONPath`                     | `github.com/azrtydxb/momo/jsonop`    | `jsonop.Path`                     |
| `momo.JSONContains`                 | `github.com/azrtydxb/momo/jsonop`    | `jsonop.Contains`                 |
| `momo.JSONHasKey`                   | `github.com/azrtydxb/momo/jsonop`    | `jsonop.HasKey`                   |
| `momo.JSONTextExpr`                 | `github.com/azrtydxb/momo/jsonop`    | `jsonop.TextExpr`                 |
| `momo.AggExpr`                      | `github.com/azrtydxb/momo/agg`       | `agg.Call`                        |
| `momo.Aggregate`                    | `github.com/azrtydxb/momo/agg`       | `agg.Func`                        |
| `momo.StringAgg`                    | `github.com/azrtydxb/momo/agg`       | `agg.String`                      |
| `momo.ArrayAgg`                     | `github.com/azrtydxb/momo/agg`       | `agg.Array`                       |
| `momo.JSONBAgg`                     | `github.com/azrtydxb/momo/agg`       | `agg.JSONB`                       |
| `momo.PercentileCont`               | `github.com/azrtydxb/momo/agg`       | `agg.PercentileCont`              |
| `momo.PercentileDisc`               | `github.com/azrtydxb/momo/agg`       | `agg.PercentileDisc`              |
| `momo.JSONPair`                     | `github.com/azrtydxb/momo/agg`       | `agg.Pair`                        |
| `momo.JSONField`                    | `github.com/azrtydxb/momo/agg`       | `agg.Field`                       |
| `momo.JSONObjectExpr`               | `github.com/azrtydxb/momo/agg`       | `agg.ObjectExpr`                  |
| `momo.JSONBBuildObject`             | `github.com/azrtydxb/momo/agg`       | `agg.BuildObject`                 |
| `momo.ToJSONExpr`                   | `github.com/azrtydxb/momo/agg`       | `agg.JSONExpr`                    |
| `momo.ToJSONB`                      | `github.com/azrtydxb/momo/agg`       | `agg.ToJSONB`                     |
| `momo.EmptyJSONArray`               | `github.com/azrtydxb/momo/agg`       | `agg.EmptyJSONArray`              |
| `momo.ExtractField`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractField`             |
| `momo.ExtractExpr`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractExpr`              |
| `momo.Extract`                      | `github.com/azrtydxb/momo/timeop`    | `timeop.Extract`                  |
| `momo.ExtractYear`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractYear`              |
| `momo.ExtractMonth`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractMonth`             |
| `momo.ExtractDay`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractDay`               |
| `momo.ExtractHour`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractHour`              |
| `momo.ExtractMinute`                | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractMinute`            |
| `momo.ExtractSecond`                | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractSecond`            |
| `momo.ExtractQuarter`               | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractQuarter`           |
| `momo.ExtractDow`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractDow`               |
| `momo.ExtractISODow`                | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractISODow`            |
| `momo.ExtractDoy`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractDoy`               |
| `momo.ExtractEpoch`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.ExtractEpoch`             |
| `momo.TimeExpr`                     | `github.com/azrtydxb/momo/timeop`    | `timeop.Expr`                     |
| `momo.AtTimeZone`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.AtTimeZone`               |
| `momo.AtTimeZoneExpr`               | `github.com/azrtydxb/momo/timeop`    | `timeop.AtTimeZoneExpr`           |
| `momo.IntervalUnit`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalUnit`             |
| `momo.Interval`                     | `github.com/azrtydxb/momo/timeop`    | `timeop.Interval`                 |
| `momo.IntervalOf`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalOf`               |
| `momo.IntervalExpr`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalExpr`             |
| `momo.IntervalSecond`               | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalSecond`           |
| `momo.IntervalMinute`               | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalMinute`           |
| `momo.IntervalHour`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalHour`             |
| `momo.IntervalDay`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalDay`              |
| `momo.IntervalWeek`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalWeek`             |
| `momo.IntervalMonth`                | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalMonth`            |
| `momo.IntervalYear`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.IntervalYear`             |
| `momo.AddInterval`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.AddInterval`              |
| `momo.SubInterval`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.SubInterval`              |
| `momo.Now`                          | `github.com/azrtydxb/momo/timeop`    | `timeop.Now`                      |
| `momo.CurrentTimestamp`             | `github.com/azrtydxb/momo/timeop`    | `timeop.CurrentTimestamp`         |
| `momo.TruncUnit`                    | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncUnit`                |
| `momo.DateTrunc`                    | `github.com/azrtydxb/momo/timeop`    | `timeop.Trunc`                    |
| `momo.TruncSecond`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncSecond`              |
| `momo.TruncMinute`                  | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncMinute`              |
| `momo.TruncHour`                    | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncHour`                |
| `momo.TruncDay`                     | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncDay`                 |
| `momo.TruncWeek`                    | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncWeek`                |
| `momo.TruncMonth`                   | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncMonth`               |
| `momo.TruncQuarter`                 | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncQuarter`             |
| `momo.TruncYear`                    | `github.com/azrtydxb/momo/timeop`    | `timeop.TruncYear`                |
| `momo.Tx.AdvisoryXactLock`          | `github.com/azrtydxb/momo/advisory`  | `advisory.XactLock`               |
| `momo.Tx.AdvisoryXactLockString`    | `github.com/azrtydxb/momo/advisory`  | `advisory.XactLockString`         |
| `momo.Tx.TryAdvisoryXactLock`       | `github.com/azrtydxb/momo/advisory`  | `advisory.TryXactLock`            |
| `momo.Tx.TryAdvisoryXactLockString` | `github.com/azrtydxb/momo/advisory`  | `advisory.TryXactLockString`      |
| `momo.DB.AdvisoryXactLock`          | `github.com/azrtydxb/momo/advisory`  | `advisory.XactLock`               |
| `momo.DB.AdvisoryXactLockString`    | `github.com/azrtydxb/momo/advisory`  | `advisory.XactLockString`         |
| `momo.DB.TryAdvisoryXactLock`       | `github.com/azrtydxb/momo/advisory`  | `advisory.TryXactLock`            |
| `momo.DB.TryAdvisoryXactLockString` | `github.com/azrtydxb/momo/advisory`  | `advisory.TryXactLockString`      |
| `momo.SearchSettings`               | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.Settings`              |
| `momo.SearchSettings.Validate`      | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.Settings.Validate`     |
| `momo.IterativeScan`                | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.IterativeScan`         |
| `momo.IterativeOff`                 | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.IterativeOff`          |
| `momo.IterativeRelaxedOrder`        | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.IterativeRelaxedOrder` |
| `momo.IterativeStrictOrder`         | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.IterativeStrictOrder`  |
| `momo.Tx.SetSearch`                 | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.Apply`                 |
| `momo.DB.WithSearch`                | `github.com/azrtydxb/momo/vecsearch` | `vecsearch.With`                  |
| `momo.DistanceExpr`                 | `github.com/azrtydxb/momo/vecop`     | `vecop.Expr`                      |
| `momo.L2Distance`                   | `github.com/azrtydxb/momo/vecop`     | `vecop.L2`                        |
| `momo.InnerProductDistance`         | `github.com/azrtydxb/momo/vecop`     | `vecop.InnerProduct`              |
| `momo.CosineDistance`               | `github.com/azrtydxb/momo/vecop`     | `vecop.Cosine`                    |
| `momo.L1Distance`                   | `github.com/azrtydxb/momo/vecop`     | `vecop.L1`                        |
| `momo.HammingDistance`              | `github.com/azrtydxb/momo/vecop`     | `vecop.Hamming`                   |
| `momo.JaccardDistance`              | `github.com/azrtydxb/momo/vecop`     | `vecop.Jaccard`                   |

### Rewrite recipe

1. Commit your work first, because the next steps edit files in place, then
   fetch the release: `go get github.com/azrtydxb/momo@v0.4.0`.
2. Save the raw markdown of this page (not the rendered HTML) as `upgrading.md`
   in your module root and run the block below from there. It turns the table
   into `gofmt -r` rules and applies them to the Go files that import
   `github.com/azrtydxb/momo` by that exact path, skipping `vendor/`. It
   rewrites the rows of plain identifiers (functions, types and constants),
   stops with an error when it finds no file or generates no rule, and does not
   touch a file that imports momo under an alias or with a dot import; the
   compiler lists what is left there.

   ```sh
   files=$(grep -rl --include='*.go' 'github.com/azrtydxb/momo"' . | grep -v /vendor/)
   rules=$(awk -F'`' 'NF == 7 && $1 ~ /^\| *$/ && $2 ~ /^momo\.[A-Za-z0-9]+$/ { printf "gofmt -r \"%s -> %s\" -w \"$@\"\n", $2, $6 }' upgrading.md)
   [ -n "$files" ] || { echo "no Go file imports momo" >&2; exit 1; }
   [ -n "$rules" ] || { echo "no rules: upgrading.md must be the raw markdown of this page" >&2; exit 1; }
   echo "$rules" | sh -s -- $files
   ```

3. Rewrite the `momo.Tx.*` and `momo.DB.*` rows by hand, because the receiver
   moves into the argument list: `recv.Old(ctx, args)` becomes
   `pkg.New(ctx, recv, args)`. A `DB` advisory call must pass a transaction
   (see the paragraph on the advisory locks below). The `Validate` method of the old
   `SearchSettings` type is a plain rename: step 2 rewrites the type, and
   `Validate` stays a method of `vecsearch.Settings`.
4. In every file, import each new package that now appears in it (`arrayop.`,
   `fts.`, ...) by the full path in the second column of the table, then build.
   The compiler lists whatever is left.

A row never changes the SQL a query produces; only the Go name and import path
change.

One type gained methods: `sqlfn.CurrentSchemaExpr` now has the comparison and
ordering methods (`Eq`, `In`, `Asc`, ...) the other function results have, so
`sqlfn.CurrentSchema().Eq("app")` compiles, where the old root function's
result had no such method.

The advisory lock methods became functions: `tx.AdvisoryXactLock(ctx, 42)`
is now `advisory.XactLock(ctx, tx, 42)`, and the same for `advisory.XactLockString` and the
`Try` forms. The transaction is the second argument and its type is
`*momo.Tx`. The `DB` methods always returned `momo.ErrValidation`, because a
transaction-scoped lock outside a transaction is released at once; the
functions turn that into a compile error, so a caller that used the `DB` form
must open a transaction with `db.Tx` or `db.Begin`. A dialect other than
PostgreSQL still fails with `momo.ErrUnsupported`.

### What did not move

These stay in `momo`, with the reason:

- `Func` and `FuncExpr`: the generic SQL function escape hatch, which the new
  packages build on.
- `Coalesce`, `Value`, `SQL` and `TypedSQL`: bound-value and raw-fragment
  building blocks of the expression core. `sqlfn.CoalesceExpr` takes expression
  arguments instead.
- The vector column types (`VectorColumn` and its variants): generated code
  references them.
- The hook interfaces (`Hook`, `QueryEvent`, `RelationHook`, `RelationEvent`):
  `DB` stores the hooks and calls them.
- The basic aggregates (`Agg[T]`, `CountAll`, `Sum`, `Avg`, `Min`, `Max`):
  `Having` and select lists are typed on `Agg`.
- `Tx.SetLocal`: a method of `Tx`, and other helpers call it.
