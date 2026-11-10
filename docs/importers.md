# Importing models from other ORMs

Momo can read the models of an existing GORM, Bun or Ent project and write the
equivalent Momo structs. Three packages do the reading:

| Package         | Reads                                                     |
| --------------- | --------------------------------------------------------- |
| `importer/gorm` | GORM structs and their `gorm:"..."` tags, `gorm.Model`    |
| `importer/bun`  | Bun structs and their `bun:"..."` tags                    |
| `importer/ent`  | the Ent `schema` package (fields, edges, indexes, mixins) |

Each package exports one function, `Read(dir string)`, that returns the neutral
`[]importer.Model` and an `*importer.Report`. `importer.Emit` turns the models
into Momo source through the generator's writer.

The readers are static. They parse the Go files of `dir` with `go/parser` and
never compile, load or execute your code, so reading a project cannot run its
`init` functions, hooks or migrations. The flip side is that they see only what
is written as a literal in those files: a naming strategy set in another
package, or an index added by a migration, is not visible. Each reader's
package comment lists its limits.

## The `momo import` command

```sh
momo import gorm --src ./old/models --out ./models
momo import bun  --src ./old/models --out ./models --report import-report.txt
momo import ent  --src ./ent/schema --out ./models --package models
```

The command runs the reader, prints the import report to standard output and
writes the Momo structs to `<out>/models.go` with a generated-file header.
`--package` sets the Go package name (the default is derived from `--out`),
and `--report` also writes the report to a file.

Exit codes: `0` the models were written, `1` the report holds an `unsupported`
change and nothing was written, `2` a usage or input error. Fix or convert the
listed constructs by hand, or remove them from the source, and run it again.
The command never overwrites a file that does not carry the generated-file
header.

## Importing migration history

The `importer/history` package converts the migration history of another tool
into a Momo migration directory, so a database that already ran those
migrations does not run them again. Four readers produce the input:

| Package                          | Reads                                                                                             |
| -------------------------------- | ------------------------------------------------------------------------------------------------- |
| `importer/history/atlas`         | an Atlas directory (`<version>_<name>.sql`, `atlas.sum`) and the rows of `atlas_schema_revisions` |
| `importer/history/goose`         | a goose directory and the `goose_db_version` table                                                |
| `importer/history/golangmigrate` | `<version>_<title>.up.sql` and `.down.sql` files and `schema_migrations`                          |
| `importer/history/bun`           | Bun's SQL migrations and the `bun_migrations` table                                               |

Each package exports `Read`, which returns a `history.Source`. `history.Convert`
refuses a source that is dirty, that holds migrations the tool never applied, or
that holds migrations it cannot convert (Bun and goose Go-function migrations).
A source version that is not a 14-digit timestamp is mapped to
`19700101000000` plus its position in the list, and the mapping is reported as a
`version-mapped` change; the original version stays in the history `name`
column. `history.Baseline` replays the converted SQL on an empty scratch
database to write one schema snapshot per migration, `history.WriteImport`
writes the directory and `history.HistoryRows` gives the `momo_migrations` rows
that mark every migration as applied. The Atlas reader verifies `atlas.sum`
with Atlas's directory hash and refuses a directory whose files were edited.
golang-migrate records only its current version, so every migration up to it is
read as applied.

## The import report

A Momo model is never an exact copy of an ORM model, because the ORMs differ in
behaviour you do not see in the struct: GORM replaces a zero value with the
column default, Ent saves an edge only when its setter was called, and so on.
The importer does not paper over these differences. Every one becomes a
`Change` in the report, with the source position (`file:line:col`), the model,
the field and a message. `Report.String()` prints one line per change, sorted by
position, in the form `source: kind: Model.Field: message`.

Read the report before you trust the converted models. `Report.Has(kind)` and
`Report.Of(kind)` filter it in code. The kinds are:

| Kind                  | Meaning                                                                                                                                  |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `zero-value-handling` | the source ORM treats a zero value as unset and applies a default; Momo writes the zero value (use a pointer to leave it unset)          |
| `implicit-save`       | the source ORM saves an association together with its parent; Momo writes only the model you pass                                        |
| `go-default`          | the source ORM computes a default in Go (timestamps, generated values); Momo needs a tag or a database default                           |
| `unsupported`         | something the reader cannot convert (an unreadable tag, an unknown builder call, a type with no mapping); convert it by hand             |
| `renamed-type`        | the field type changes on import, for example `gorm.DeletedAt` becomes `*time.Time`, and the automatic filter does not carry over        |
| `dropped-field`       | a field that does not become a column (ignored with `-`, scan-only, relation fields) is not imported                                     |
| `dropped-index`       | an index Momo's tags cannot express (a composite index, a second index tag on a field) is not imported; declare it in the companion file |
| `naming`              | a table, column, constraint or foreign key name that Momo derives itself, or a naming strategy the reader cannot see; check the names    |
| `version-mapped`      | a migration version was renumbered while importing migration history (`importer/history`)                                                |
| `go-side-rule`        | a validator, immutable field, hook or policy that the source ORM runs in Go and Momo does not                                            |

The importer never drops one of these silently: an empty report means the
reader found nothing to flag, not that it checked behaviour it cannot see.
