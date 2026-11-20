# Compatibility and deprecation policy

This page says what Momo promises to keep working once it is at version 1,
what it does not promise, how a deprecated API is retired, and how that
differs from the pre-v1 releases you can install today. The promise only
starts at v1; until then read [how pre-v1 differs](#how-pre-v1-differs-today).

## What the promise covers

From v1, a minor or patch release does not break a program or a
repository that worked with an earlier `1.x` release. The promise covers these
surfaces:

| Surface                  | What stays compatible                                                                                                                                                                                                                    |
| ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Exported Go API          | every exported identifier of the covered packages: names, signatures, types, struct fields, interface method sets, constants and the documented behavior, including the sentinel errors that `errors.Is` matches                         |
| Generated code shape     | code that `momo generate` wrote with any `1.x` release compiles and behaves with every later `1.y` runtime; this is why the descriptor constructors that look internal, such as `momo.NewColumn`, are covered                            |
| CLI flags and exit codes | the commands, flags and flag values in `momo help`, the exit codes 0, 1, 2 and 3 and what each means, and the text `momo generate --check` uses for a stale file; the wording of diagnostics and of human-readable output is not covered |
| `momo.yaml` keys         | a key that `gen/config` reads keeps its name, type and meaning; a new key is optional and has a default that keeps the old behavior; `version: 1` keeps loading                                                                          |
| Snapshot, history, sum   | `schema.FormatVersion`, `migrate.SnapshotFormat`, `migrate.SumFormat` and `migrate.FormatVersion` (the history table) keep reading what an earlier `1.x` wrote; a release may add optional fields but never changes the meaning of one   |
| Model declarations       | the `momo` struct tag grammar and option names, the `momo.Model`, `momo.View` and `momo.Projection` markers and the `MomoConfig` method: an option keeps its meaning and a new one is an addition                                        |
| Generator plugin API     | `gen.PluginAPIVersion` stays at 1 for the whole `1.x` line; a plugin built for it keeps running                                                                                                                                          |

Compatible means source compatible for the exported Go API, as `apidiff`
(`golang.org/x/exp/cmd/apidiff`) judges it: adding an identifier, a method to a
struct or a field to a struct is compatible; adding a method to an exported
interface that users implement, removing or renaming anything, or changing a
signature is not. The `apidiff-docs` CI job checks this on every pull request
(see [the gate](#how-the-gate-enforces-it)).

A newer snapshot, history table or `momo.sum` than a binary understands is
refused with `schema.ErrNewerFormat` rather than read wrongly; the reverse,
an older file read by a newer binary, always works.

Behavior bug fixes are not breaking changes. A fix that makes Momo refuse
what it used to accept wrongly (an invalid identifier, a wrong dialect
statement) is a fix, and the changelog says so. A security fix may break
compatibility in a minor release when there is no other way; the release notes
and [upgrading](upgrading.md) say how to adapt.

## What is excluded

These are outside the promise. They can change in any release.

| Excluded                                                                         | Why                                                                                                                   |
| -------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `gen/graphql`, `gen/grpc`, `gen/openapi`                                         | ecosystem generators and their runtime helpers follow gqlgen, protobuf and OpenAPI tooling that Momo does not control |
| `gen/apiquery`                                                                   | the helper surface of the generated API handlers, tied to the generators above                                        |
| `importer` and `importer/...`                                                    | readers of other ORMs' models and migration history; they track those tools' formats                                  |
| `internal/...`                                                                   | not importable; includes `internal/testdb`, `internal/suite` and the other test support                               |
| `momotest`, `momotest/sqlitetest`, `migrate/migratetest`                         | test helpers: their assertions, fixtures and defaults may change to catch more mistakes                               |
| `bench` and `examples/...`                                                       | separate benchmark module and runnable examples                                                                       |
| the `otelmomo` module                                                            | versioned and tagged on its own (`otelmomo/vX.Y.Z`), so it carries its own promise from its own v1                    |
| the exact SQL text, the wording of errors and diagnostics, the layout of reports | the sentinel errors and error types are covered; the SQL Momo writes and the messages it prints are not               |
| anything marked experimental in its doc comment                                  | not covered by the promise, but the gate does not read doc comments: list the package or declare the change breaking  |

The `apidiff-docs` gate knows the excluded packages from
`scripts/apidiff-experimental.txt`, read from the merge base so that a pull
request cannot exempt its own change. The file lists the `gen/graphql`,
`gen/grpc`, `gen/openapi`, `gen/apiquery` and `importer` rows above. The other
rows are outside the gate already (`internal`, nested modules) or are not API
(the SQL text, messages). The gate does not filter by doc comment: a pull
request that changes an identifier marked experimental in a covered package
must be declared breaking, or the package must be listed first. Adding a
package to the covered set later is a compatible change; moving a covered
package to the excluded set is not allowed within `1.x`.

## Interfaces and dependencies with their own rules

- `migrate.Dialect` and the optional interfaces a migration dialect may
  implement are implemented only by Momo's `migrate/postgres`,
  `migrate/sqlite` and `migrate/mysql`. A `1.x` release may add a method to
  them; code outside Momo calls a dialect but does not implement one.
- `driver/pgxdriver` is the pgx v5 adapter, and pgx types appear in its
  signatures (`pgxdriver.New` takes a `*pgxpool.Pool`). It follows pgx v5 for
  the whole `1.x` line; a later pgx major version gets a new driver package
  instead of a change to this one. `pgxdriver.PgxTx` and
  `pgxdriver.UnwrapPgxTx` are the supported way to move between a pgx
  transaction and a `momo.Tx`.
- The gate runs `apidiff` under one build configuration, so identifiers that
  exist only in a cgo build, such as those of `driver/sqlitedriver`, are not
  compared by it. A change to them is reviewed by hand.

## The expression extension API

Code outside Momo may define its own typed expressions, and the surface for
that is deliberately small. These names are the extension API, covered like any
other exported identifier, and nothing else in the root package is an
extension point:

| Name                          | Why a minimal external expression needs it                                                                                                                                  |
| ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `momo.Expr`                   | the one method, `AppendSQL`, that every fragment implements                                                                                                                 |
| `momo.Expression`             | the typed form: an `Expression[bool]` is a `momo.Predicate`, so the compiler checks operand types                                                                           |
| `momo.Ops`                    | embedding `momo.Ops[T]` gives the host the comparison and ordering methods (`Eq`, `In`, `Between`, `Asc` and the rest) without writing them                                 |
| `momo.NewOps`                 | binds the embedded `momo.Ops` to the finished host; there is no other way to build one                                                                                      |
| `momo.Typed`                  | gives a plain `momo.Expr` a result type, for a fragment that needs no comparison methods of its own                                                                         |
| `momo.Flagged`                | lets a wrapper forward the flags of the column it wraps, so a value compared with a sensitive column stays masked in hook events through the wrapper                        |
| `momo.ColumnFlags`            | the result type of `momo.Flagged`                                                                                                                                           |
| `(*momo.Builder).Ident`       | writes a quoted identifier; names never go into the SQL as text                                                                                                             |
| `(*momo.Builder).Arg`         | binds a value as a parameter; values never go into the SQL as text                                                                                                          |
| `(*momo.Builder).Dialect`     | lets the expression choose its SQL or refuse a dialect                                                                                                                      |
| `(*momo.Builder).SetErr`      | reports a bad argument or an unsupported dialect, with a `*momo.Error` of kind `momo.ErrValidation` or `momo.ErrUnsupported`                                                |
| `(*momo.Builder).WriteString` | writes the fixed SQL text of the expression (keywords, operators, punctuation); without it an expression could only write identifiers and parameters, never a function call |

The example attached to `momo.NewOps` in the root package's tests is a worked
expression built from these names alone, and CI compiles and runs it.

Everything else that agg, arrayop, fts, jsonop, rangeop, sqlfn, timeop and
vecop needed (binding a value with the sensitive-column mask, rendering an
operand to learn whether it read a sensitive column, a FILTER clause, a
bound-value check, name validation and a window wrapper) is behind an internal
bridge that only the packages of this module can import. Those names are not
promised, and a limit follows from that: a value that a custom expression binds
with `Builder.Arg` itself is not masked when the expression compares with a
sensitive column, so compare through the `momo.Ops` methods when an operand can
be one. The names that left the extension API stay as deprecated forwarders for one
release; [upgrading](upgrading.md) lists them.

### Generated-code support

`momo.NewColumn`, `momo.NewNullColumn`, `momo.NewReadOnlyColumn`,
`momo.NewNullReadOnlyColumn`, `momo.NewCreateOnlyColumn`,
`momo.NewImmutableColumn` and the six `momo.NewVectorColumn` variants exist for
the code `momo generate` writes. Their flags parameter is a `momo.ColumnFlags`,
built from the `momo.Flag*` constants, and `momo.ColumnMeta.Flags` has the same
type. They are covered by the generated code shape promise above, so they stay
stable even though application code should use the generated column variables
instead of calling them.

## Deprecation

An API leaves in two steps: it is deprecated, and much later it is removed.

1. A deprecated identifier, flag or `momo.yaml` key gets a `Deprecated:`
   paragraph in its doc comment that names the replacement
   (`// Deprecated: use X instead.`). Tools such as staticcheck report uses of it.
   The replacement exists in the same release, so a user can move at once.
2. The release notes (`CHANGELOG.md`) and [upgrading](upgrading.md) list it
   with a rewrite recipe. A deprecated CLI flag or `momo.yaml` key keeps working
   and prints a warning to standard error.
3. A deprecated identifier stays for at least one full minor release after the
   one that announced it. In the covered surface it is then removed only in the
   next major version, because removing it earlier is an incompatible change.
   In an excluded package it may be removed from the second minor release after
   the announcement.
4. A deprecated identifier keeps its documented behavior and keeps its tests.
   It receives security fixes, but not new features.

## How the gate enforces it

The `apidiff-docs` job runs `make apidiff-gate` on every pull request
(`scripts/apidiff-docs-gate.sh`). It compares the exported API of every
non-internal package in the root module between the base branch and the pull
request.

- Before v1 the gate only demands a docs change for an exported change, as
  [contributing](contributing.md) describes. It never fails an incompatible
  change on its own.
- From v1 it also fails an incompatible change in a covered package unless
  the pull request is declared breaking: the title has a `!` after the type
  (`feat!: remove X`; the type is matched in any case), or the pull request
  body or a commit message has a line that starts in column 0 with
  `BREAKING CHANGE: ` or `BREAKING-CHANGE: ` and a description. A declared
  breaking change ships in the next major version; release-please reads the
  same markers to choose it.
- A major version above 1 also needs a new module path,
  `github.com/azrtydxb/momo/v2`. release-please does not change the module
  path, `go.mod` or the import paths, so that release is a manual step the
  maintainer takes then; the gate does not check it.

The gate reads the module version from `.release-please-manifest.json` and the
excluded packages from `scripts/apidiff-experimental.txt`, both on the merge
base, so a pull request cannot switch the gate off or exempt its own change by
editing either file. It reads the manifest only when a covered package has an
incompatible change, and a manifest it cannot read counts as pre-v1. The exact
rule is in the header of `scripts/apidiff-docs-gate.sh`; the filter that drops
the excluded packages is `scripts/apidiff-incompatible.sh`, and
`internal/repocheck` tests it against apidiff-shaped output.

## How pre-v1 differs today

Momo is pre-v1. None of the promises above applies to the releases you can
install now, and this is what holds instead:

- A `0.y` minor release may break the exported API, the generated code, a CLI
  flag, a `momo.yaml` key or a file format. release-please runs with
  `bump-minor-pre-major`, so a breaking change raises the minor number, and the
  commit is marked `feat!:` or has a `BREAKING CHANGE:` footer.
- There is no deprecation window. An identifier can be removed in the release
  after the one that deprecated it, or in the same release when no replacement
  is needed.
- The `apidiff-docs` gate does not fail an incompatible change; it asks for a
  docs change only.
- Snapshot, history and sum files carry the format versions above and a newer
  one is refused, but a `0.y` release can still raise a version; read
  [upgrading](upgrading.md) before each upgrade.
- Pin an exact version in `go.mod`, read `CHANGELOG.md` before you upgrade and
  run `momo generate` again afterwards; [supported versions](supported-versions.md)
  keeps the stability table.

Which packages are excluded and the deprecation rules are written down now so
that a review of the exported API before v1 can change what needs to
change; they take effect when the version tag does.
