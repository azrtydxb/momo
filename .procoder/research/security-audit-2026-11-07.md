# Security audit: github.com/azrtydxb/momo (main @ 9bc7868, v0.3.0)

Method: read-only source review (Read/Grep/git, `gh api` read-only, OSV queries, pgx v5.12.0 source from the local module cache). Nothing was built, run or modified. Scope: root package, `dialect/`, `driver/pgxdriver/`, `migrate/` (+`postgres/`), `cmd/momo/`, `gen/`, `vector/`, `types/`, workflows, go.mod, repo settings.

Only the PostgreSQL path exists (pgxdriver + migrate/postgres). SQLite/MySQL dialect descriptors exist but have no driver or migration dialect, so findings touching them are latent.

## Summary

| Severity | Count |
| -------- | ----- |
| Critical | 0     |
| High     | 0     |
| Medium   | 4     |
| Low      | 10    |
| Info     | 5     |

No exploitable SQL injection was found in the value, identifier or raw-SQL paths. The weak spots are API contracts that do not hold end to end (LIKE escaping), error text that leaks bound values, controls documented but not built, and repository/process hygiene.

---

## Medium

### M1. `EscapeLike` contract only holds for `ContainsLiteral`; `Like`/`ILike` have no ESCAPE clause (wildcard injection)

Confidence: High.

- `allowlist.go:38-45` documents `EscapeLike` as escaping `%`, `_` and `!` with `!` "inside a LIKE pattern rendered with ESCAPE '!'". Only `containsLiteral` emits `ESCAPE '!'` (`allowlist.go:78`).
- `expr.go:476` `Like` and `expr.go:483-496` `ilike` bind the pattern with no ESCAPE clause. The `Like` doc says only "escape wildcards in user input yourself" without naming the escape character.
- There is no case-insensitive literal helper (no `ILikeLiteral`, prefix or suffix helper), so the natural code for case-insensitive search is `ILike(col, "%"+EscapeLike(q)+"%")`.
- Exploit: `momo.ILike(User.Email, "%"+momo.EscapeLike(q)+"%")` with attacker input `%`. PostgreSQL's default LIKE escape is backslash, so `!%` is a literal `!` followed by a wildcard. The attacker enumerates all rows in a search endpoint meant to be scoped, or sends `%a%a%a...` patterns for CPU cost. Searching for a literal `!` also silently fails.
- Fix: emit `ESCAPE '!'` in `Like`/`ILike` (breaking change) or add `ILikeLiteral`/prefix/suffix helpers. Name the escape char in the `Like` doc.

### M2. Client-side pgx errors quoting bound values are not redacted on Query/Exec/Begin paths

Confidence: Medium (pgx text seen in v5.12.0 `query_test.go:1075`: "unable to encode 705 into binary format for point"; not exercised here).

- `ClassifyError` (`driver/pgxdriver/classify.go:39`) returns any non-`*pgconn.PgError` unchanged. Only COPY and batch go through `copyErr` (`bulk.go:45-66`), whose comment says pgx encode errors "quote the offending value". `driver.go:82-100` and `driver.go:177-190` return the raw error.
- Exploit: a user-controlled value of a type pgx cannot encode for the target OID (an `int` bound to `point`, a custom `Valuer`). The error contains the value. Handlers that echo `err.Error()` or log it leak request data. SQLSTATE class 22 server errors are already redacted, so this is the remaining gap.
- Related, lower impact: `vector/vector.go` parse errors and `types/hstore.go` (`invalid hstore %q`) echo database text.
- `pgxdriver.NewPool` (`tracer.go:76-80`) returns pgx's `ParseConfigError` unchanged; pgx documents its password redaction as best-effort (`pgconn/errors.go:148-152`). The CLI is protected because `l.secrets` holds the whole URL; library callers are not.
- Fix: apply the `copyErr` policy in `ClassifyError`: non-PgError, non-context, non-momo errors become `driverError{cause, "pgx: client error"}` with `Unwrap` preserved.

### M3. Security controls promised in docs/ARCHITECTURE.md are not implemented

Confidence: High (grep of non-test code finds none).

- `docs/ARCHITECTURE.md:129` says `Compiled` carries a sensitive-args mask used by hooks. `Compiled` is `{SQL, Args}` (`builder.go`), and `FlagSensitive` (`meta.go:34`) only affects the generated allowlist. With `WithArgLogging` (`db.go:94,197-198`) every argument, including `momo:"sensitive"` columns such as passwords, reaches hooks verbatim. `ev.Args` is also the live bound slice, so a hook can mutate bound values.
- `docs/ARCHITECTURE.md:126` lists `MaxRows(n)` and `WithMaxInList(n)`. Neither exists (see L5).
- `docs/ARCHITECTURE.md:11,109` describes a `momovet` analyzer that flags non-constant `SQL(...)`. None exists, so the "sql must be a trusted constant" contract (`expr.go:604-612`, `select.go:900-914`) has no enforcement.
- Per the CLI itself, `lint`, `rollback`, `rebase` are not available (`cmd/momo/migrate.go` notAvailable map). Hand-written migrations get no destructive-statement guard; `-- momo:destructive` is informational (`migrate/files.go:21`).
- Impact: readers of the architecture doc assume protections that are absent.
- Fix: implement the mask, or amend the doc and mark these as planned. Document that `WithArgLogging` logs secrets.

### M4. Repository security configuration

Confidence: High (`gh api`, read-only).

- Private vulnerability reporting is disabled (`/private-vulnerability-reporting` returns `{"enabled":false}`), yet `SECURITY.md` tells reporters to use it. A reporter hits a dead end and may open a public issue.
- `main` has no branch protection and no rulesets (404). Open Scorecard alerts: BranchProtection #45, CodeReview #47, TokenPermissions #46 (`release-please.yml:8` declares write at top level).
- Secret scanning, push protection, non-provider patterns and dependabot security updates are all `disabled`.
- Default workflow token permission is `write` and workflows may approve PRs. Current workflows override this with explicit `permissions:`, so it is latent.
- No CODEOWNERS, so nothing protects `.github/workflows/`, `migrations/` or `momo.sum`.
- Fix: enable private vuln reporting, secret scanning with push protection, branch protection and CODEOWNERS; set the default token to read.

---

## Low

### L1. Migration integrity model is self-attesting

Confidence: High; design limitation.

- `momo.sum` (`migrate/sum.go`) is an unsigned hash list beside the files. `momo migrate hash` (`cmd/momo/migrate.go:678`) rewrites it and accepts every edit (it prints the changes). `rehash --accept` (`migrate/rehash.go:42-109`) then overwrites the history checksum. Both are single, unconfirmed commands with no operator identity; the history note records only a timestamp.
- Scenario: someone with repo write access edits an applied migration, runs `hash`, then `rehash --accept` per database. Tamper detection is silently disabled. It protects against accidents, not malice.
- Mitigation: CODEOWNERS on `migrations/`, a CI step failing if `momo.sum` lines for existing files change, record `current_user` in the rehash note.
- Related: `Baseline --force` (`migrate/baseline.go`) has no audit beyond `apply_id='baseline'`.

### L2. History table is referenced unqualified (search_path hijack)

Confidence: Medium.

- Every history statement uses bare `momo_migrations` (`migrate/runner.go`, `rehash.go`, `history.go`, `postgres/session.go:15-17`). `resolveSchemas` (`engine.go`) only checks `current_schema()` equals `Schemas[0]`.
- Scenario: on PostgreSQL before 15, or wherever PUBLIC has CREATE on the first schema in search_path, a low-privilege user pre-creates `momo_migrations` with an INSERT trigger or forged "applied" rows. The next `apply` by a privileged role fires the trigger with the migrator's privileges, or skips migrations.
- Fix: schema-qualify the history table from `current_schema()` and verify its owner.

### L3. `FromSQLDB`/`FromConnConfig` do not enforce "values are always bound"

Confidence: High on the facts.

- The native driver rejects `QueryExecModeSimpleProtocol` (`driver.go:20-40`) because it interpolates client-side. `FromConnConfig` (`sqldb.go:59-62`) uses `cfg.DefaultQueryExecMode` "as given", and `FromSQLDB` (`sqldb.go:39`) cannot inspect the `*sql.DB`.
- Scenario: an app behind PgBouncer sets simple protocol (a common workaround) and momo's bound-parameter guarantee silently becomes pgx's client-side escaping. pgx's sanitizer has had CVEs before (e.g. CVE-2024-27304, fixed long ago; none known for 5.12.0).
- Also: pgx v5.12.0 (`conn.go:524-526`) forces simple protocol for any statement with zero arguments, on every path. Raw `SQL`/`Exec`/`Scalars` fragments with no `?`, and all migration statements, can therefore be multi-statement. This only matters if the "trusted constant" contract is broken, but with M3 (no analyzer) it removes a backstop.
- Fix: reject simple protocol in `FromConnConfig`; document the stdlib limitation.

### L4. TLS guidance is incomplete

Confidence: High.

- `TLSWarning` (`cmd/momo/migrate_redact.go:15-52`) fires only for URL-form strings with `sslmode=disable` and a remote host. No warning when `sslmode` is unset (pgx default `prefer` falls back to plaintext), for `allow`/`prefer`, for keyword/value DSNs (`url.Parse` fails, returns ""), or for the `PGSSLMODE` env var.
- The warning recommends "`sslmode=require` or stronger". In pgx/libpq `require` encrypts but does not verify the server certificate unless a root cert is supplied, so it is MITM-able. `docs/ai-guide.md:118` correctly says `verify-full`.
- README snippets use `sslmode=disable` (`README.md:170,409`).
- Fix: warn on non-verifying modes for remote hosts, recommend `verify-full`, parse DSN form too.

### L5. No bind or row caps on typed query paths

Confidence: High.

- `inList.AppendSQL` (`expr.go:352-374`) and `NotIn` have no `MaxParams` check, unlike raw `In` (`raw_args.go:355`) and inserts. A request-supplied slice above 65535 builds and then fails at the driver with a pgx client error. It is not memory-unsafe, but costs O(n) allocation and a 500.
- `SelectQuery.All` (`select.go:674-699`) loads all rows; `Limit` is optional; `MaxRows` is not implemented (M3).
- Scenario: `Where(ID.In(ids...))` or `.Limit(n)` driven by user input is an application-level DoS unless the caller caps it.
- Rejected: regex DoS (all regexps are Go RE2, linear time). Graph writes are one level deep (`graph.go:30-33`). Vector decode bounds dimensions before allocating (`vector_codec.go:480-538`, `MaxDim` 16000, `MaxSparseNNZ` 16000).

### L6. Advisory lock key is predictable and global to the database

Confidence: High.

- `lockID("momo:<schema>.momo_migrations")` = fnv64a (`migrate/postgres/lock.go:21-26`, `engine.go:353`). Any role that can connect can `pg_advisory_lock(<id>)` and block every migrator until `ErrLocked` (default wait 30s). Accidental collision with other apps is about 2^-64.
- Release on panic and error paths is sound: `defer end(err)` runs reset then unlock detached from ctx, with `pg_advisory_unlock_all` plus `Discard` as fallback (`lock.go:55-75`).
- Residual: `defer func() { _ = c.Release() }()` ignores `ErrPinnedBusy` (`engine.go`). Negligible.

### L7. `drift`/`baseline` execute model DDL on the live database

Confidence: High.

- `canonicalize` (`migrate/postgres/canonicalize.go:192-300`) runs `BEGIN`, `CREATE SCHEMA momo_canon_*` and the declared DDL (defaults, CHECK/view/function text from models or snapshots) in the target database, then `ROLLBACK`. It needs CREATE on the database and takes locks.
- The CLI usage describes `drift` as a "compare" command. A drift job pointed at production runs arbitrary expression SQL from models there. Rollback is detached with a `Discard` fallback. Impact is bounded by trust in the model and snapshot text.
- Fix: document it, or support a scratch database.

### L8. DDL renderer splices developer/snapshot-controlled text unvalidated

Confidence: High; trusted-source assumption.

- Raw into DDL (`migrate/postgres/render.go`): column `Type`/`Default`/`Generated` (`columnDef`), `checkDef` Expr, index `Where` and part `Expr`, `Partition.Key` and `PartitionOf.Bound`, policy `Using`/`Check`, view `Query`, function `Args`/`Returns`/`Body`, trigger `When`.
- Migration statements execute with zero args, hence over the simple protocol (L3), so `)` followed by `; DROP ...` in any of them is a multi-statement.
- Sources are struct tags (developer), snapshot JSON (committed, checksummed) and DB catalogs. Output lands in a reviewed `.up.sql`, so exploitation needs repo write access. A hostile or compromised snapshot or tag still yields arbitrary SQL under the migration role.
- The quoted paths are correct: identifiers via `AppendIdent` (`dialect.go:131-140`), string literals via `quoteLiteral` (NUL rejected, `''`, `E''` with doubled backslashes), enum values, FK actions (whitelist), index method and collation (quoted), nulls order (whitelist), `With` values (`safeValue` or quoted), trigger function (anchored regexp), `$momo$` tag collision (`createFunction`).
- Fix: optionally reject top-level `;` in expression fields using the existing splitter.

### L9. Go toolchain floor allows vulnerable stdlib; module advisories

Confidence: Medium.

- `go.mod:3` is `go 1.26.0` with no `toolchain` line. OSV lists about 20 stdlib advisories for 1.26.0 (GO-2026-4599 onward). CI uses setup-go "1.26"/"1.27" (latest patch) and runs govulncheck; consumers and CLI builds inherit whatever toolchain builds them.
- OSV reports no advisories for: mysql v1.10.1, pgx v5.12.0, yaml v3.0.6, x/tools v0.52.0, x/text v0.43.0, x/sync v0.24.0, x/mod v0.42.0, edwards25519 v1.2.0, puddle v2.2.3, nor for the test-only `internal/namingrec` module (gorm 1.31.2, bun 1.3.0, ent 0.14.6, atlas, hcl v2.18.1). `go-sql-driver/mysql` is a direct dependency used only in `internal/testdb`; consumers still pull it in.

### L10. CI/CD hygiene

Confidence: High.

- Good: no `pull_request_target`, no `run:` step interpolates untrusted `github.event.*`, read-only default permissions in ci/gosec/codeql, `persist-credentials: false` everywhere, pr-title only hands the token to the action.
- Actions use mutable tags (`@v7`, `@v5`...). Scorecard alerts #71-#102 flag this; the project accepts it as policy. Tag risk concentrates in `release-please` (contents+pull-requests write on every main push).
- `release-please.yml:8` has write at workflow level (Scorecard #46); move it to the job.
- `gosec.yml` runs `-no-fail` (informational); the blocking run is `make gosec` in `ci.yml` lint.
- CI test credentials (`postgres:momo`, `momo_app:momo_app`) in `ci.yml:152-154`, `Makefile:23-25`, `internal/testdb/compose.yaml:32` are for ephemeral service containers. Not secrets.

---

## Info

- I1. `Tx.DriverTx()` (`db.go:358`) exposes the raw driver tx whose Commit/Rollback ignore the `adopted` guard. Documented escape hatch. Normal `Tx.Commit/Rollback` return `ErrAdoptedTx` (`db.go:339-374`) and savepoint scopes inherit `adopted`.
- I2. `Tx.SetLocal` (`search.go:196-201`) binds name and value but can set `role` or `search_path`. Documented as never from untrusted input.
- I3. CLI output of DB-sourced strings (drift/status object names, error text) is not stripped of control characters, so a DB user can inject ANSI escapes into terminal output.
- I4. Redaction (`migrate/redact.go`) is regex best-effort, as documented. Known gaps: URL password with a raw `/` or whitespace, MySQL DSN passwords with space or quote. The CLI additionally redacts the full URL and parsed password verbatim (`migrate.go:351-390`). `sslkey` paths are not redacted (low value). Engine methods other than `Apply`/`RunMigration` return unredacted errors to library callers.
- I5. `dirFS` treats a non-directory (a typo'd `migrate.dir`) as an empty directory (`cmd/momo/migrate.go:312-317`), so `apply`/`plan` report "no pending migrations" with exit 0. Also `Discover` and `ComputeSum` read files separately (small TOCTOU, `engine.go:61-70`), and `os.DirFS` follows file symlinks out of the directory (documented).

---

## Checked and rejected (false-positive candidates)

- Identifier injection. All identifiers go through `AppendIdent`, which doubles the quote per dialect (`dialect.go:131-140`). NUL in an identifier truncates the wire string inside an open quote, a syntax error only. Function names (`expr_funcs.go:41-56`) and aliases (`expr.go:626-640`) use `validName` (letters/digits/`_`/`$`, 63 bytes max). `Cast` type allows only words plus `(n[,m])`. ORDER BY direction and NULLS are fixed strings (`expr.go:230-249`). Lock options are whitelisted (`select.go:307-318`). Join kinds, `ROW_NUMBER`/`DENSE_RANK`, vector operators are constants. `Substr` positions are `strconv.Itoa(int)`. `INDEXED BY`, `WithTableName`, CTE and savepoint names go through `Ident`. PG truncates names over 63 bytes silently (collision risk, not injection).
- SQLite inline literals. `sqliteLiteral` (`builder.go`) handles bool/int/finite float/string with `'` doubling and rejects NUL. No SQLite driver exists yet.
- Raw SQL placeholder rewriting. `scanRaw` (`raw_args.go`) skips strings (incl. `E''` and MySQL backslash handling), quoted identifiers, `--`/`#`/nested block comments and tag-aware dollar quotes, and protects `??`, `?|`, `?&`, `@>`, `@?`, `@@`, `<@`. It fails closed on unterminated constructs and on mixing `$n` with `?`. Documented limits: `standard_conforming_strings=off`, `NO_BACKSLASH_ESCAPES`, MySQL `/*! */`.
- Keyset cursors. Never decoded by the library; `After(values...)` binds each value; `Allowlist` maps external sort keys to descriptors without echoing names.
- Generator plugins. `momo generate` runs only built-in plugins (`cmd/momo/generate.go` selectPlugins); no dynamic loading, no plugin path in `momo.yaml`. Companion config is read statically via go/packages, user code is not executed. Caveat: `go list` runs in the target repo, so running `momo` in an untrusted checkout is as trusting as `go build`.
- Generated-code injection and path traversal. Every string emitted into Go source uses `strconv.Quote` (`gen/emit_*.go`). Output paths are checked by `Writer.contained` with symlink resolution (`gen/writer.go:62-86`), and files without the header are never overwritten. `writeNewFiles` requires `filepath.Base(n) == n`, refuses dotted names, never replaces existing files (hard link / `O_EXCL`); only `momo.sum` is renamed over.
- Migration file names must match `^\d{14}_[a-z][a-z0-9_]*\.(up|down)\.sql$`; `diff` name matches `^[a-z][a-z0-9_]*$`; symlinked directories are refused; dotfiles are skipped.
- Adopted-transaction commit bypass. `AdoptTx` Commit/Rollback refuse (I1).
- Unique-violation values in errors. `ClassifyError` uses only `pe.Message`, never `pe.Detail` (`classify.go`); data exceptions are withheld; history rows redact class 22 (`runner.go` redact). Residual gap is M2.
- Secrets in repo and history. Regex sweeps over the tree and all 108 commits (private keys, AKIA, ghp_, github_pat_, xox, URL credentials) found only the ephemeral CI credentials (L10) and doc placeholders.
- Vector values go as bound text/binary args, validated for dimension and finiteness. `SearchSettings` values are range-checked and bound via `set_config(?, ?, true)`.
