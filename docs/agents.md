# Using these docs with an AI coding agent

Momo is meant to be written by agents as well as people. The documentation is
built so an agent can read it without scraping HTML.

## Where an agent should read

- [Agent guide](ai-guide.md): one self-contained file with setup, modeling,
  migrations, queries, relations, vectors, errors, testing and the rules. If an
  agent reads one page, it reads this one.
- `llms.txt` on the documentation site lists every page with a link to its raw
  Markdown. `llms-full.txt` contains the Markdown of every page in one file.
  Each page also has a raw `.md` twin next to its HTML. All three are generated
  at build time and exist under every published version of the site.
- The repository root has its own `llms.txt` for agents that read the
  repository instead of the site.

## Canonical patterns

The shapes below are the ones the guide teaches. The linked examples run
against PostgreSQL in CI.

- Describe a table as a struct that embeds `momo.Model`, run `momo generate`,
  and build queries from the generated descriptors. See
  [getting started](getting-started.md) and the
  [quickstart example](../examples/quickstart/).
- Insert with `Insert[T]` and `Model`, read with `Select[T]` and `Where`, change
  rows with `Update[T]` and `Set`, remove them with `Delete[T]` or `SoftDelete`.
  See [crud-zero-null](../examples/crud-zero-null/).
- Run related statements in `db.Tx` and use the `tx` it passes in. See
  [transactions](../examples/transactions/).
- Load relations explicitly with `Load`, `LoadAll` or `Preload`. See
  [relations](relations.md).
- Change a schema by editing the model and running `momo migrate diff`, then
  review the SQL it wrote. See [migrations](migrations.md).

## AGENTS.md snippet

Copy the block below into the `AGENTS.md` (or `CLAUDE.md`) of a repository that
uses Momo. It is the short form of the rules in section 12 of the
[agent guide](ai-guide.md); a test keeps it complete.

<!-- momo:agents-snippet:start -->

```markdown
## Momo (Go ORM)

- PostgreSQL with pgvector is the only supported database for migrations and vector builders. SQLite (cgo driver, not on Windows) and MySQL 8.4, MySQL 9.7 and MariaDB 11.8 run queries and writes; check docs/capabilities.md before relying on a feature there.
- Run `momo generate` after every model change and commit the generated files. Never edit generated files.
- A non-pointer model field is NOT NULL; a pointer field is nullable.
- Pass user input only as arguments to `momo.SQL`, `momo.TypedSQL`, `momo.Exec` and `momo.Scalars`. Never build SQL by string concatenation.
- Never bypass `ErrUnfilteredMutation` with `AllRows`; add a `Where`.
- Check errors with `errors.Is` against the `momo.Err*` sentinels. Never match error text.
- Use only the `tx` handle inside `db.Tx`. Retry the whole transaction on `ErrSerialization`, never inside it.
- Never edit a migration that has been applied. Write a new one with `momo migrate diff`.
- Run `momo generate --check` in CI.
- Run `momo migrate validate` in CI.
- Test against a real PostgreSQL with pgvector. Do not mock `momo.Querier`.
- Full reference: https://azrtydxb.github.io/momo/latest/ai-guide.md
```

<!-- momo:agents-snippet:end -->
