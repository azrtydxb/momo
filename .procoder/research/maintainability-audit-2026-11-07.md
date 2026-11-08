# Momo maintainability audit

Repo: github.com/azrtydxb/momo, branch main, HEAD 9bc7868 (v0.3.0). Read-only audit; nothing was compiled or run. Function lengths and nesting were measured with an awk scan of `func ... }` spans and by reading. Treat line numbers as of HEAD.

Effort: S = under half a day, M = 1-3 days, L = a week or more. Priority: H / M / L.

## 0. Size and shape

| Area                              | Non-test Go                                             | Test Go                | Notes                                                                             |
| --------------------------------- | ------------------------------------------------------- | ---------------------- | --------------------------------------------------------------------------------- |
| root `momo`                       | 48 files, 10.9k lines                                   | 134 files, 17.9k lines | 107 external (`momo_test`), 27 internal (`package momo`); 37 tagged `integration` |
| migrate (+postgres)               | 22+9 files, 5.9k+2.6k                                   | 33+22 files, 6.1k+3.6k | `diff.go` 1164, `engine.go` 838                                                   |
| gen (+tags,naming,typemap,config) | 17+10 files, 4.3k+0.9k                                  | 24+9 files             | `schema.go` 758, `companion.go` 733                                               |
| cmd/momo                          | 5 files, 1.5k                                           | 15 files, 1.8k         | `migrate.go` 1032                                                                 |
| driver/pgxdriver                  | 9 files, 2.1k                                           | 24 files, 4.9k         | test:src is 2.3:1                                                                 |
| internal/namingrec                | 10.5k lines of committed `ent` output (separate go.mod) |                        | ~11% of all Go lines in the repo; none of it is momo logic                        |
| .procoder/                        | 344 tracked markdown files, 42.7k lines                 |                        | plans/specs alone are 34k lines, more than the root package                       |

Whole repo: 637 Go files, 95.8k lines. Without namingrec and examples it is about 80k.

Files over about 800 lines: `insert.go` 1072, `select.go` 947, `migrate/diff.go` 1164, `cmd/momo/migrate.go` 1032, `migrate/postgres/render.go` 866, `migrate/engine.go` 838, `docs/ai-guide.md` 1360. Under the Go idiom of one concept per file, only `insert.go`, `diff.go`, `cmd/momo/migrate.go` and `ai-guide.md` are real problems. The others are long but cohesive.

Import graph is clean and acyclic: root -> dialect, vector, schema; schema -> dialect; migrate -> root + schema; migrate/postgres -> root + migrate; gen -> schema/dialect only; pgxdriver -> root. `internal/repocheck/importgraph_test.go` enforces the core rules. There is no cycle. The structural weakness is that root is a hub: it is at once the user query API, the driver contract (`Driver`, `DriverTx`, `Rows`, `PinnedConn`, `Pinner`, `Copier`, `Batcher`, ...) and the relation engine. `migrate` and `migrate/postgres` import the whole query builder only to get `momo.PinnedConn`, `momo.Driver`, `momo.TxOptions`.

---

## 1. Structure

### S1. `InsertQuery` is a five-mode god type; `insert.go` is 1072 lines mixing builder, renderer, chunk planner and transaction scope (High, M-L)

- `insert.go:32-55` `insertCore` carries mutually exclusive modes: `models`, `hasValues`, `rows/hasRows`, `sel/selCols` (FromSelect) and `upsert/withIdentity`. Compatibility is enforced at render time by hand-written cross-checks: `insert.go:296-308`, `insert_select.go:~170-182`, `insert.go:623`, `upsert.go`.
- `renderStatement` (`insert.go:287-442`, 156 lines) validates, plans fields, resolves hooks, then renders VALUES for two modes. `valueRows` (450-537, 88 lines) has 3-4 nesting levels. `renderConflict` (622-694, 73 lines). `plan` (~760-840) is about 80 lines.
- `insert.go:~850-900` `beginScope` plus `chunkScope`/`chunkRows` (about 150 lines) is a second, private implementation of "run in a tx or savepoint, roll back on error, re-panic". `db.go:404-420` (`DB.Tx`) and `db.go:445-465` (`Tx.Tx`) have the same shape.
- Why it hurts: every new INSERT feature (this is how conflict-where, FromSelect, Upsert arrived) touches 3-4 of these functions, and the mode matrix is only tested by example.
- Refactor: split into `insert.go` (builder methods), `insert_render.go` (pure rendering from a resolved `insertPlan` struct, produced by one `resolve()` step that does all validation), `insert_chunk.go` (plan + chunkRows). Make `beginScope` a shared `runAtomic(ctx, q, fn)` in `db.go` used by `DB.Tx`, `Tx.Tx` and chunked insert (also the two relation writers `assoc.go:152`, `graph.go:193` call `inTx`). Resolve to a single `mode` enum instead of four booleans.

### S2. `SelectQuery.renderBody` is 160 lines and `ExprSelect` is a second, partial SELECT renderer (High, M)

- `select.go:424-583`: WITH, DISTINCT, select list, FROM, INDEXED BY, JOIN, keyset, soft-delete, projection soft-delete, WHERE, GROUP, HAVING, ORDER (keyset variant and normal), LIMIT/OFFSET (with SQLite `LIMIT -1`), lock. Up to 4 nesting levels.
- `insert_select.go:66-110` `ExprSelect.renderBare` re-implements FROM, soft-delete marker, WHERE, `WHERE true` for SQLite ON CONFLICT and scope handling; `selectArity` is added to `SelectQuery` in `insert_select.go:~95-105` (arity logic lives in the wrong file).
- The `onConflictFollows bool` parameter threaded through `render/renderBare/renderBody` is a SQLite-only workaround that SQLite-less users pay for in every signature.
- Refactor: split `renderBody` into `renderWith`, `renderSelectList`, `renderFrom`, `renderWhere`, `renderTail` methods on `selectCore` (which does not need `T`). Let `ExprSelect` hold a `selectCore` and reuse `renderFrom/renderWhere` so soft-delete and FROM DUAL logic exist once. Move `selectArity` into `select.go`.

### S3. Dialect branching is done two ways (High-Medium, M)

- `dialect.Dialect` defines capability booleans (`dialect.go:36-52`) and the code mostly uses them (`b.d.NullsOrdering`, `b.d.Locking`, ...). But 19 sites in root compare `d.Name == dialect.SQLite/MySQL`: `builder.go:145`, `explain.go:28,46`, `expr.go:302,310`, `expr_funcs.go:110,136,368`, `insert_select.go:85,214`, `in_tuple.go:57,133`, `insert.go:374`, `raw_args.go:90,126,296`, `select.go:493,569`, `search.go:216`.
- Three of the four named dialects have no driver, so these branches are exercised only through `fakedriver` and golden strings, never a real database. README admits the SQLite ON CONFLICT path is "not yet run against a real SQLite" (README:299).
- Refactor: replace name checks with capability fields (`LimitNeedsClause`, `DistinctFromSyntax`, `FromDual`, `IndexedBy`, `ConflictTargetInlineLiterals`) so adding a dialect is a data change, and add a test that fails if root source mentions `dialect.SQLite`. Longer term, either delete the SQLite/MySQL branches until a driver exists or put them behind a tracked list (see S4).

### S4. About 60 lines of core builder code exist for a dialect nobody can run (Medium, S)

- `builder.go:214-273` `sqliteLiteral` / `sqliteLiteralErr` (inline float/string/bool rendering for SQLite ON CONFLICT predicates), `builder.go:145` `conflictTarget` flag, `insert.go:645-660`, `select.go:493-499` (`IndexedBy`), `select.go:569-572`, `expr.go` doc strings, `docs/capabilities.md:106` (a 600-character table cell on this one feature).
- Why it hurts: it is user-visible API surface (`SelectQuery.IndexedBy`, `ConflictQuery.Where` inline-literal contract) with a permanent documentation and test cost and zero users.
- Refactor: gate behind a documented "reserved for SQLite milestone" list; or remove `IndexedBy` and `sqliteLiteral` until M2 and record the removal in the changelog (pre-1.0, breaking changes are already allowed).

### S5. Root package carries the driver contract, relation engine and write helpers in one namespace (Medium, L)

- Relation machinery is spread over `relation.go`, `load.go`, `loader.go`, `joinload.go`, `preload.go`, `relpred.go`, `assoc.go`, `graph.go`, `relhook.go` (about 2.5k lines) all as methods on one generic `Relation[P,C]`; `Relation.Count` (`assoc.go:305`), `Relation.From` (`relpred.go:156`), `Relation.With...` collide in name with `SelectQuery.Count/From`.
- Driver contract (`driver.go`, `driver_optional.go`, `driver_access.go`, `onconn.go`) has to be exported from root because `migrate` and `pgxdriver` import it; that forces users to see 20+ driver-author types in godoc.
- Refactor (low-risk first): move the driver interfaces into `momo/driver` (type aliases left in root for one release). Do not split the relation engine into a subpackage (generics + unexported `Meta` access make it painful); instead add a `doc.go` index and keep each file one concern.

### S6. Smaller structure notes

- `allowlist.go` holds `Allowlist`, `EscapeLike` and `ContainsLiteral` (`allowlist.go:12-80`). Move the LIKE helpers to `expr_funcs.go` (S).
- `search.go` holds `SearchSettings`, `Tx.SetLocal` (a general-purpose transaction method, `search.go:196`), `Tx.SetSearch`, `DB.WithSearch`. `SetLocal` belongs in `db.go` (S).
- Options are defined in four places: `db.go` (`WithHook`, `WithArgLogging`), `clock.go` (`WithClock`), `options_maxparams.go` (`WithMaxParams`), `fields.go` (`WithFieldHooks`). A one-file `options.go` costs nothing (S).
- `DB.fieldHooks map[any]any` (`fields.go:69-73`) is a type-erased registry keyed by `*Meta[T]`. It works but cannot be checked by the compiler; a typed `fieldHookRegistry` with a generic `hooksFor[T]` accessor would localise the one cast (S, Low).
- `migrate/postgres/render.go:47-184` `renderChange` (138 lines) mixes inline cases (AddPrimaryKey, DropIndex, DropView, CreateFunction) with delegation to `renderXxx` helpers. `Change.Before/After` are `any` payloads re-typed with `get[T]` at every case (Medium, M): make one `renderXxx` per kind with a typed payload accessor on `Change`.
- `cmd/momo/migrate.go:889-1032` (`writeNewFiles`, `stageTemp`, `publishNew`, `syncDir`, about 140 lines) is a self-contained atomic-write library hidden in a CLI file. Move to `internal/atomicfs` with its own tests (S, Medium).

---

## 2. Duplication

### D1. Comparison method set is written seven times (High, M)

- `Column[T]` (`expr.go:76-120`), `Agg[T]` (`expr.go:539-546`), `exprOps[T]` (`expr_ops.go:36-76`), and four wrapper types in `permissions.go`: `ReadOnlyColumn`, `NullReadOnlyColumn`, `CreateOnlyColumn`, `ImmutableColumn` (each about 30 forwarding one-liners, `permissions.go:17-190`), plus `vectorBase` in `vector_column.go:52`.
- The code already contains the fix: `exprOps` says "so each method exists once", but `Column` and `Agg` do not use it. As a result `Agg[T]` has `Eq..Ge` but lacks `EqExpr`, `In`, `Between`, `IsNull` (a HAVING inconsistency visible in the API), and any new operator (e.g. `NotBetween`, `Like` as a method) needs 7 edits.
- Refactor: make `Column[T]` embed a small `cmp[T]` that takes `Expr` (not `self`) and let all wrappers embed `Column[T]` while shadowing only the write methods they forbid. Go cannot remove a promoted method, so the permission wrappers should embed an unexported `readable[T]` comparison struct that excludes `Set*`. One generator or generic base eliminates about 250 lines. Add a compile-time interface test (`var _ interface{Eq(T) Predicate...}`) so the sets cannot drift.

### D2. Query-and-scan boilerplate (Medium, S)

- `compile -> conn().query -> defer Close -> Next -> Scan -> Err -> Close` is hand-written in `select.go:821-853` (`Count`), `scalars.go:11-33`, `assoc.go:251-303` (`currentKeys`), `assoc.go:305-358` (`Relation.Count`), `graph.go:~215-270`, `load.go:105-130`.
- Two styles of deferred close coexist: `defer r.Close()` (select.go:679, scalars.go) and `defer func(){ _ = rows.Close() }()` (assoc.go:283, load.go:125).
- Refactor: `func queryEach(ctx, q Querier, op, table string, c Compiled, row func(Rows) error) error` and `queryOneInt64`. About 80 lines removed and a single place for hook/close semantics.

### D3. `Each` and `Iter` (`select.go:746-775`, `779-817`) are the same loop (Low, S)

`Iter` can be `Each` wrapped in `iter.Seq2` via a `pull` helper or share an internal `stream(ctx, yield func(*T) bool) error`.

### D4. Error construction has at least 10 helpers and 28 inline literals (High, S)

- Helpers: `InsertQuery.verr` (`insert.go:90`), `UpdateQuery.fail` (`update.go:~87`), `DeleteQuery.fail` (`delete.go:67`), `graphErr` (`graph.go:36`), `Relation.assocErr` (`assoc.go:372`), `Relation.invalid` (`relation.go:77`), `rawErr` (`raw_args.go:238`), `searchInvalid` (`search.go:117`), `joinedLoader.unsupported` (`joinload.go:70`), `Builder.unsupported` (`builder.go:176`).
- `select.go` alone has 16 inline `&Error{Kind: ErrValidation, Op: "build", Table: sq.meta.Table, Err: ...}` literals (lines 188, 239, 244, 250, 255, 282, 292, 313, 326, 342, 344, 463, 475, 490, ...), 28 across the root package.
- Plus `errors.New("momo: query has no Querier")` five times (`select.go:386,823`, `insert.go:265`, `update.go:206`, `delete.go:102`).
- Refactor: in `errors.go` add `func newError(kind error, op, table string, err error) *Error`, `func validation(op, table, format string, args ...any) *Error`, and `var errNoQuerier`. Replace all helpers. Effect: one place to add context (e.g. the model name) later, and `errors.Is` behaviour is uniform (see E1).

### D5. `views/functions/triggers/policies` diff blocks in `migrate/diff.go` (High, S)

- `diff.go:780-813`, `817-849`, `853-886`, `888-926` are copy-paste with four map-building loops each (fromN/toN/fromO/toO) and differ only in key function, change kinds and equality. A generic `diffNamed[T]` already exists at `diff.go:666` for table children but is not used here.
- Refactor: a generic `diffObjects` helper taking the differ, the from and to lists, their normalised forms, a key function and the create, drop and replace emitters. Removes about 110 lines. Also the biggest function in the file, `tables` (`diff.go:242-367`, 126 lines), deserves splitting into rename detection and per-table diff.

### D6. Pinned-connection state machine implemented twice in pgxdriver (Medium, M)

- `driver/pgxdriver/pin.go` (`pinned`, `pinnedRows`, `live/done/Release/Discard`) and `driver/pgxdriver/sqldb.go:185-390` (`sqlPinned`, `pinnedSQLRows`, `live/done/take/clearTx/Release/Discard`) solve the same busy-count / release / discard problem over pgx and database/sql.
- Refactor: extract a generic `pinCore` (busy flag, closed flag, ErrPinnedBusy logic) and keep only the thin pgx/sql specifics. The ErrPinnedBusy semantics currently have to be fixed twice (each has its own test file: `pin_test.go` 494 lines, `sqldb_pin_test.go`).
- Also `pgxRows`/`sqlRows` wrap Scan/Err with `ClassifyError` identically (`driver.go:148-160`, `sqldb.go:176-181`).

### D7. Tiny AST node types and boilerplate (Low, S)

- `updPKEq` (`update.go:95-106`) is exactly `binOp{qcol{t,c}, " = ", argExpr{v}}`; used by `update.go:115,158` and `delete.go:88`.
- 17 predicate node types each declare `func (T) resultType() bool { return false }` (expr.go x11, relpred x2, allowlist, in_tuple, softdelete, update). Embed a `boolExpr struct{}` mixin.
- `b.WriteString(", ")` inside `if i > 0` loops appears 40+ times (assoc 6, load 8, select 7, insert 7, in_tuple 4, ...). Add `b.commaList(n, func(i int))` / `appendList` already exists for `[]Expr` only (`select.go:622`), so extend it for idents and use it.
- `indexFrom` (`raw_args.go:227`) re-implements `strings.Index(s[from:], sub)`.

### D8. Test helpers duplicated across root test files (Medium, S)

- `unitDB()` (`insert_test.go:49`), `typedDB(name)` (`typed_expr_test.go:75`), `stateDB(d)` (`write_states_unit_test.go:31`) are identical `momo.New(fakedriver.New(...))`; 41 test files also construct it inline.
- Compile helpers: `compileFor` (`conflict_where_test.go:66`), `renderPred` (`in_tuple_internal_test.go:12`), `renderForTest/renderErr` (`vector_expr_test.go:14-19`), `fieldDB` (`fields_unit_test.go:19`), each wrapping `Compiled` in a slightly different way.
- Refactor: one `testhelpers_test.go` (`newUnitDB(t, name)`, `sqlOf(t, q)`, `wantSQL(t, got, want)`). 2-3 hours.

---

## 3. Dead, half-used and stale code

### X1. Features with no producer or consumer (Medium, S per item)

- `gen/plugin.go` (Plugin, VersionedPlugin, `PluginAPIVersion`, `validatePlugin`) plus `cmd/momo/cmd.go:55` `plugins()` returns `[]gen.Plugin{}` and `cmd/momo/generate.go:222 selectPlugins` resolves names against an empty list. A complete plugin API with zero plugins. Keep only if documented as extension point; otherwise delete (-100 lines plus tests).
- `driver/pgxdriver/sqldb.go:60` `_ = opts` in `FromConnConfig(cfg, opts ...Option)`: options are accepted and silently ignored ("signature parity"). Either drop the parameter or apply them (a user passing `WithExecMode` gets nothing).
- `dialect.Dialect` flags `Merge`, `WindowFunctions`, `IsolationLevels` are only read by `internal/suite/caps.go`; `Flavor`/`MySQLFlavor` are used by no non-test code outside `dialect`; `TransactionalDDL` has one real reader (`migrate/automigrate.go:106`). `RowValues`, `RecursiveCTE`, `ILike` have one reader each.
- Exported but test-only/never-called outside root: `Allowlist/NewAllowlist` (0 docs, 0 examples), `WithTableName`, `HasFrom`, `ContainsLiteral`, `SubstrFrom`, `BatcherOf/CopierOf` (only used inside `pipeline.go`/`insert` paths). They are fine API but undocumented in README/ai-guide, so an agent or user cannot discover them.
- `Direction` (`relpred.go:7`) has 0 test references.
- `internal/testdb` exposes `MySQL84URL/MySQL97URL/MariaDBURL/SQLitePath` and `internal/suite.Caps` has `Merge/WindowFunctions/NonDefaultIsolation` flags that no Postgres test branch uses.
- `ParseHeader` (named in the brief): no references remain anywhere (code, tests, docs). Already clean.

### X2. Stale comments and docs-in-code (Medium, S)

- `CONTRIBUTING.md:6` "A C toolchain (for the SQLite packages, which use cgo)"; no such packages exist.
- Comments citing spec/plan IDs that no reader can resolve: `dialect/dialect.go:108` "(S-59)", `internal/suite/transactions.go:31` "S-7", `internal/suite/errors.go:13` "S-8", `migrate/postgres/inspect.go:84` "momo-04 Task 18"; `.procoder` is where these live, so they rot when plans are archived.
- `dialect.go:11` comment on `Postgres/SQLite/MySQL` consts has no doc comment lines at all (exported without docs).
- Root package has **no package doc comment** (`grep '^// Package momo'` finds nothing, while dialect, migrate, gen, schema, types, vector and pgxdriver all have one). pkg.go.dev shows a blank overview for the main package. (High-visibility, S.)
- Only 4 TODO-like markers exist in the repo, and they are in archived stories; the codebase is clean in that respect.
- `.github/workflows/ci.yml:157` matrix comment "M2: add sqlite; M3: add mysql84..." and `ci.yml:6` branch globs `sprint-*`, `worktree-agent-*` are leftovers of the multi-agent process.

### X3. Committed fixtures that are noise for maintainers (Low-Medium, S)

- `internal/namingrec/ent` (about 10.5k generated lines, 50 files) is a separate Go module (own go.mod) so it is not compiled by `go test ./...`, not covered by `.github/dependabot.yml` (only `/` is listed) and not covered by gosec/staticcheck. It exists only to record what ent's naming looks like. Keep the recorded output (e.g. a small `naming_record.json`) instead of the generator project, or exclude the module explicitly in `.gitattributes` with `linguist-generated`.
- 344 files / 42.7k lines under `.procoder/` (plans for 13 epics, 294 story files). Fine for the owner, but it drowns `git grep` and review noise. Add `.procoder/ export-ignore` in `.gitattributes`, or archive finished sprints into one file per sprint.

---

## 4. API surface and consistency

### A1. Expr-suffix naming is inconsistent (Medium, M, breaking)

- Value vs expression operand is spelled `Eq(v T)` / `EqExpr(e Expression[T])` and `Add(e, v)` / `AddExpr(e, other)` (`expr_funcs.go:253-269`), `SetExpr`, but `Coalesce(e, fallback T)` vs `CoalesceExpr(a, b, more...)` returns a different type (`FuncExpr`) and has a different arity pattern; `Substr` vs `SubstrFrom` encodes arity not operand kind; `CountOf(e)` vs `CountAll()` vs `CountDistinct(e)`.
- `Column.As` is a **table** alias while the package-level `As` and `Agg.As` are **select-list** aliases (documented in `expr.go:~645` but still an easy trap).
- Refactor options: (a) pre-1.0, rename to one rule: `X(v)` for values, `XExpr(e)` for expressions, everywhere including functions; (b) at least rename `Column.As` to `Column.From`/`InTable`, since `As` now means two things.

### A2. Several functions return `any` or a too-wide type (Medium, S)

- `momo.In(values any) any` and `momo.Ident(name string) any` (`raw_args.go:46,53`): argument markers typed as `any`, so `momo.SQL("... ?", momo.In(x))` is unchecked and `In` also exists as `Column.In`, `exprOps.In`, so three unrelated `In`s. Return exported marker types (`InArg`, `IdentArg`) accepted by a `RawArg` interface.
- `momo.SQL` returns `Predicate` (`expr.go:612`) while `TypedSQL[T]` returns `Expression[T]`; `SQL` is therefore usable as a boolean everywhere even for non-boolean fragments, defeating the type safety the package advertises.
- `Union(all bool, ...)` boolean positional param; `ForUpdate(opt ...string)` accepts free strings checked at build time (`select.go:300-318`); constants `NoWait`/`SkipLocked` would turn a runtime ErrValidation into a compile error. `SelectQuery.Limit(int64)` vs `ErrLimitExceeded`: fine.
- Two different `Where` meanings: `ConflictQuery.Where` (index predicate) and `InsertQuery.Where` (DO UPDATE condition) (`insert.go:208`, `242`); `insert.go:242` can be called at any point and errors only at render time. Rename the second `DoUpdateWhere` or make `DoUpdate(...)` return a type with `Where`.

### A3. Exported generated-code plumbing (Low-Medium, M)

- 11 constructors `NewColumn`, `NewNullColumn`, `NewReadOnlyColumn`, `NewNullReadOnlyColumn`, `NewCreateOnlyColumn`, `NewImmutableColumn`, `New{,Null,ReadOnly,CreateOnly,Immutable}VectorColumn` are public API only because generated code lives in user packages. They fill godoc ("is used by generated code"). Move to a `momo/mgen` (or `internal`-visible via a generated alias file) and keep root clean. Combinatorial type explosion (access x null x vector = about 12 column types) comes with it (see D1).
- Exported methods without any doc comment: 112 in `permissions.go`, 24 in `expr.go`, 6 in `expr_funcs.go`, `DB.Dialect`, `Tx.Dialect` (`db.go:~125,~158`), `Relation...`, `errors.go` sentinel group `ErrUniqueViolation...ErrNotNullViolation` (`errors.go:23-26`). Most are one-liners, but godoc will show a bare method list for the main user-facing type.
- `Querier` is sealed (unexported `conn()`): users cannot wrap or mock `Querier`. Good for safety; it should be said in README and offered with a `momotest`-style fake (the architecture doc promises `momotest`, not built).

### A4. Godoc accuracy spot checks

- `Column.SetDefault` doc (`expr.go:~139`): "PostgreSQL supports it in INSERT and UPDATE; SQLite only by omitting..." describes a database nobody can use.
- `ConflictQuery.Where` doc (`insert.go:190-207`) is a 17-line paragraph about SQLite literal formatting; the PostgreSQL behaviour (the only runnable one) is one sentence.
- `Insert.ValuesRows` doc (`insert.go:126-135`) describes SQLite behaviour as a primary case.
- `Upsert` doc (`upsert.go:8-33`) is accurate and thorough; `Rows` doc in `insert.go:~718` correctly warns about Close. These are the model to follow.
- `Error.Is` (`errors.go:~100`) compares only `target == e.Kind`; `errors.Is(err, driverErr)` still works through `Unwrap`, good. But nothing documents that `*Error` is returned as `error` value of pointer type, so `errors.As(err, &momo.Error{})` (wrong) vs `var e *momo.Error` is up to the reader. Add an example.
- `README.md:12-13` and `llms.txt:3` still say "v0.1.0" (see Section 7).

---

## 5. Error handling

### E1. Mix of typed and bare errors (Medium, S)

- Bare `errors.New("momo: ...")` with no sentinel: `db.go:364` (Commit on savepoint scope), `db.go:382` (Rollback on savepoint scope), `insert.go:985` (`Scan without a current row`), `returning.go:128`, `pipeline.go:221`, `builder.go:199`, 5x "query has no Querier". These cannot be matched with `errors.Is` (no `Kind`), unlike their siblings. Give them `ErrValidation` or `ErrTxDone`/new `ErrMisuse`.
- `return ErrTxDone` (bare sentinel) at `db.go:~371,~389,~475` vs `&Error{Kind: ErrTxDone, ...}` elsewhere: both match `errors.Is`, but `errors.As(&*Error)` only works for the second. Pick one (typed with Op).
- `select.go:758,794` `fmt.Errorf("momo: select cancelled: %w", err)`: wraps ctx error but has no `Kind`.
- Tx rollback failures: `errors.Join(err, fmt.Errorf("momo: rollback failed: %w", rbErr))` (`db.go:414,460`, `insert.go`): good, but three slightly different copies (D1/S1).

### E2. Swallowed errors (Medium, S)

- `migrate/history.go:116` `r.AppliedAt, _ = time.Parse(timeLayout, at)`: a malformed timestamp in the history table silently yields the zero time in `status` output.
- `migrate/*.go` has 7 identical `defer func(){ _ = c.Release() }()` (`baseline.go:55`, `runner.go:285`, `automigrate.go:62`, `rehash.go:47`, `drift.go:409`, `engine.go:202,409,481,557,606`): a failed Release (which for pgxdriver can mean the connection is poisoned) is never reported or logged. Use a named-return helper `releaseInto(&err, c)`.
- `driver/pgxdriver/sqldb.go:60` `_ = opts` (X1).
- Root: `_ = rows.Close()` after an error is acceptable and consistent; `db.go:273`, `graph.go:220-270` repeated 5 times in one function, candidates for the helper in D2.

### E3. Panics reachable by users (Low, S)

- `options_maxparams.go:27` `WithMaxParams(n<=0)` panics at `momo.New` time (documented). `db.go:346` `AdoptTx(nil)` and `pgxdriver/adopt.go:22,47` panic (documented as programming errors; consistent with "nil Driver in New").
- Re-panics (`db.go:409,452`, `insert.go:725`) lose the original stack when recovered and re-raised with `panic(p)`; wrap with `panic(fmt.Errorf(...))` is not appropriate, but `runtime/debug.Stack()` could be logged to hooks. Minor.
- Generator/migrate internal panics: `gen/emit_meta.go:276` (unsupported kind in schema literal, reachable only if schema types gain a kind), `migrate/drift.go:102` `mustJSON`. Fine as invariants but the CLI should convert them to exit 1 with a message via a top-level recover in `cmd/momo/main.go`.
- No panics on the query hot path. Good.

---

## 6. Tests

### T1. Per-review file accretion in the root package (High, M)

- Names that record _when_ a test was written, not _what_ it covers: `bulk_review_test.go`, `load_review_test.go`, `relpred_review_test.go`, `timestamps_review_test.go`, `upsert_review_test.go`, `graph_polish_test.go`, `graph_polish_integration_test.go`, `joinload_polish_integration_test.go`, `load_nested_more_test.go`, `typed_expr_more_test.go`, `typed_expr_more_integration_test.go`, `expr_fixes_test.go`, `select_repair_test.go`, `raw_args_hardening_internal_test.go`, `in_tuple_edges_internal_test.go`, `relpred_shapes_test.go`.
- Related duplicates by subject: `load*` has 14 files (`load_test`, `load_internal`, `load_composite_internal`, `load_finish_internal`, `load_nullkey_internal`, `load_m2m_internal`, `load_m2m_integration`, `load_combined`, `load_combined_integration`, `load_topn`, `load_topn_integration`, `load_integration`, `load_softdelete_integration`, `load_nested(_more)`, `load_review`); `insert` / `bulk` / `upsert` / `insert_select` / `conflict_where` / `write_states` / `returning` are 18 files for one INSERT pipeline; `typed_expr*`, `expr_*` 6 files.
- Proposal: group by production file, one `X_test.go` plus `X_internal_test.go` plus `X_integration_test.go` at most; move review-driven cases into table tests inside the right file. Mechanical (git mv + merge), about 2 days, no behaviour change. Add a repocheck test that bans `*_review_test.go`, `*_polish_*`, `*_more_*`.

### T2. Three integration-test conventions (High, S)

- Build tag `//go:build integration` + `_integration_test.go` suffix (36 files, one — `integration_joins_test.go` — has the name but not the tag).
- Tagged but not named (`bulk_test.go`, `update_test.go`).
- Untagged, run-time skip via `testopen.Each` / `testdb` (`load_test.go:433`, `integration_aggregates_test.go`, `integration_locking_test.go`, ...; 41 files use `testopen.Each`).
- Consequence: `make test` on macOS/Windows jobs (`go test ./...`) executes the untagged ones and silently skips; they report green with zero DB coverage. Contributors cannot tell by filename or tag whether a test needs Postgres.
- Fix: decide on tag-only (preferred, as `vet` already runs both modes). Rename `integration_*_test.go` to `*_integration_test.go`, tag them, and add a repocheck assertion (file imports `testdb`/`testopen` => has the tag).

### T3. SQL-string assertions duplicate renderer logic (Medium, M)

- `select_golden_test.go` is named "golden" but holds inline strings; each case repeats the 5-column select list verbatim (lines ~28-45). `internal/golden` exists and is used only by `gen`. About 539 `.SQL` references in root tests and 141 literal-SQL lines; 27 in `raw_args_internal_test.go` alone, 17 in `raw_args_hardening_internal_test.go`.
- Any change to quoting, column order or whitespace in `Builder` fails dozens of tests by hand-editing strings.
- Refactor: move SQL expectations to testdata golden files via `internal/golden` with `-update`, keyed by `dialect/name`. Keep a handful of inline asserts for semantic points (placeholders, arg order). The compilefail directory (`testdata/compilefail`) is a good example of the right idiom.

### T4. Sleep-based synchronisation (Medium, S-M)

- 17 `time.Sleep` calls in tests: `driver/pgxdriver/pin_test.go:279,370,444,483`, `tracer_test.go:75,198,200`, `sqldb_pin_test.go:43,170,249`, `bulk_mode_test.go:106`, `bulk_test.go:186`, `iter_test.go:198`, `migrate/helpers_test.go:124`, `migrate/postgres/lock_test.go:232`, `cmd/momo/migrate_cli_integration_test.go:122` (`700ms` release delay vs `--lock-wait 20s`).
- `tracer_test.go:75` and `sqldb_pin_test.go:170` sleep 100 ms to "let something happen"; under `-race` on a loaded CI runner these are the likely flakes. Replace with channels/`sync.WaitGroup`, or `Eventually(poll until condition, timeout)` helper that already half-exists in `migrate/helpers_test.go:124`.

### T5. Fuzz targets not wired into CI (Medium, S)

- `Makefile:5-15` `FUZZ_TARGETS` is a hand-maintained list of 10; the repo has 15 `Fuzz*` functions. Missing: `FuzzUUIDScan`, `FuzzTimeOfDayScan`, `FuzzDateScan`, `FuzzJSONScan` (`types/codec_fuzz_test.go`), `FuzzParse` (`gen/tags/hostile_test.go:85`). They compile and run as seed-only tests but never fuzz.
- `internal/repocheck/makefile_test.go` only dry-runs the target names; extend it to compare `go test -list Fuzz` output (or a `grep`) against the Makefile list.

### T6. Other test organisation notes (Low)

- `internal/repocheck` (636 lines) tests CI YAML, release config and Makefile contents (`ci_test.go`, `release_test.go`, `workflows_test.go`). Reasonable guardrail, but `TestImportGraph` shells out to `go list` and these tests only run when the whole module is tested; keep them out of the default `unit` job if runtime matters.
- 26 `t.Skip` in non-namingrec tests; most are intentional (integration URL), but sweep to make sure none are stale.
- `joinload_bench_test.go` is the only root benchmark and `bench-smoke` runs `-benchtime=1x` everywhere: it proves the benchmarks compile, not that performance did not regress.
- Test:src line ratio is 1.64 in root, 2.3 in pgxdriver: healthy, but pgxdriver `pin_test.go` 494 lines and `pgbouncer_test.go` 448 lines exercise the same pin/Release states as `sqldb_pin_test.go` (see D6).

---

## 7. Docs

### Doc1. Version strings are stale and not maintained by release-please (High, S)

- v0.3.0 is released (`.release-please-manifest.json`, CHANGELOG) but `README.md:12,21,22,178`, `llms.txt:3,7`, `docs/ai-guide.md:3,4,27,42,43,441,470,1304,1306`, `docs/capabilities.md:3,62`, `docs/supported-versions.md:3,12,13,52,93`, `docs/README.md:29`, `examples/README.md` all say v0.1.0. `ai-guide.md:3-4` ("Every identifier below was checked against v0.1.0") is false; the guide describes `Row1..Row10`, `FromSelect`, `AdoptTx` which arrived in 0.2/0.3.
- Install commands in the README pin `@v0.1.0`, so a user following the README gets an old release with none of the documented API.
- Fix: add `extra-files` to `release-please-config.json` with `<!-- x-release-please-version -->` markers (or generic updater regexes) for the version tokens, and add a repocheck test that fails if any doc mentions a version other than the manifest's.

### Doc2. `docs/ARCHITECTURE.md` documents a design, not the code (High, M)

- Line 3-4 calls itself "the binding contract for every implementation plan under `.procoder/plans/`". It lists APIs with zero references in the repo: `WithInterceptor`, `Interceptor`, `Operation`, `WithMaxInList`, `MaxRows`, `WithReplicas`, `DB.Reader`, `NewUnitOfWork`, `AllowStaleReads`, `Raw[T,PT]`/`RawQuery`, `WritableModeler`/`MomoWritable`, `GoMigration`/`WithGoMigrations`, `momotest`, `momolog`, `momovet` + `cmd/momovet`, `importer/{gorm,bun,ent}`, `gen/{graphql,grpc,openapi,viz}`, `driver/{sqlitedriver,sqlitepure,mysqldriver}`, `migrate/importers/*`, `.down.sql` rollback; `otelmomo/` and `bench/` modules do not exist.
- A reader or an AI agent that follows `llms.txt` -> `ARCHITECTURE.md` will plan against ~12 nonexistent APIs.
- Fix: split into `ARCHITECTURE.md` (what exists, 1-2 pages, tested by repocheck: every named identifier resolves via `go doc`) and move the plan-contract content to `.procoder/` where it belongs.

### Doc3. Same text maintained in three places (Medium, S)

- The SQLite-literal ON CONFLICT paragraph: `README.md:299`, `docs/ai-guide.md:~819`, `docs/capabilities.md:106` (a 700-char table cell), plus `insert.go:190-207` godoc. Four copies of a feature for a dialect with no driver.
- The "Known issues" and migrations sections repeat between `docs/migrations.md` (642 lines) and `docs/ai-guide.md:470-520`.
- `ai-guide.md` is 1360 lines: a single file for agents that probably exceeds useful context. Split by task (`ai-guide/queries.md`, `.../migrations.md`) and keep `llms.txt` as the index.
- Rule: one canonical home per fact, other places link.

### Doc4. Docs not covered by drift checks (Medium, M)

- Only `internal/repocheck/examples_test.go` verifies examples compile and sums match. Nothing checks that identifiers named in ai-guide/capabilities exist. A small test that extracts backticked `momo.X` / `X.Y` tokens and resolves them with `go/doc` would have caught Doc1/Doc2.
- `docs/capabilities.md` columns for SQLite and MySQL describe behaviour that is "not usable" and unverified; label them "design target, untested" or collapse into one line.

---

## 8. Build and CI hygiene

### B1. CI redundancy and structure (Medium, S)

- Full unit suite runs in: `unit` (x2 Go versions), `race` (x2), `windows`, `macos`, and again in `integration` (`-tags integration -race ./...` x2 Go x2 PG = 4 runs). That is about 10 runs of the unit tests per push; `compile-fail` (x2) re-runs a subset already inside `unit`.
- `lint` job runs `make gosec`, while a separate `gosec.yml` workflow runs the same gosec (different mode, SARIF). `govulncheck` runs through an action in CI but `make govulncheck` pins `v1.9.0` locally (two pinned sources of truth, `Makefile:16` vs `ci.yml:111`). `codeql.yml` as well. Three security scanners is fine; the duplication of the _pinning_ is the issue.
- Checkout + setup-go boilerplate appears 9 times in `ci.yml`. A composite action (`.github/actions/setup-go`) or a reusable workflow cuts ~60 lines.
- `ci.yml:6` `branches: [main, "sprint-*", "worktree-agent-*"]` plus `pull_request: {}`: pushes to a PR branch named `sprint-*` run CI twice. Remove the multi-agent branch globs now that sprints are over.
- `ci.yml:157` matrix `dialect: [postgres] # M2: add sqlite...` is a one-element matrix used only to compose a make target name.

### B2. Makefile (Low-Medium, S)

- `FUZZ_TARGETS` incomplete (T5). `test-integration` is an alias of `test-integration-postgres` (fine). No `make lint` aggregate or `make ci`, so local parity with the CI `lint` job requires typing five targets.
- No `.golangci.yml`; the project relies on `go vet`, `staticcheck` and `gosec` only. There is no complexity or function-length gate, which is how 100-160 line functions grew (list in Section 9).
- `make test-compilefail` runs `-run '^TestFieldPermissions$' ./gen` (a regex inside a recipe); fine but fragile if the test is renamed (repocheck would not catch it).
- `go.mod` says `go 1.26.0` and CI also builds with 1.27; there is no Go-version policy line in CONTRIBUTING (supported-versions.md has one).

### B3. Release config (Low, S)

- `release-please-config.json` is minimal and correct for `go` type, but see Doc1 (no `extra-files`), and `CHANGELOG.md` entries contain a hand-written italic preamble per release that release-please will not regenerate; document the manual step in CONTRIBUTING.
- `dependabot.yml` has only `/` for gomod, so `internal/namingrec` (separate module) is unmonitored (3 findings class: stale ent/gorm/bun deps in a committed go.sum).

---

## 9. Functions over about 70 lines (measured), deepest nesting called out

| Lines | Location                                                                                                   | Note                                                                                                                                                                   |
| ----- | ---------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 163   | `gen/tags/tags.go:68` `Tags.set`                                                                           | flat switch, 6 closures; table-driven map would be 60 lines (Low)                                                                                                      |
| 160   | `select.go:424` `renderBody`                                                                               | S2                                                                                                                                                                     |
| 156   | `insert.go:287` `renderStatement`                                                                          | S1                                                                                                                                                                     |
| 143   | `raw_args.go:83` `scanRaw`                                                                                 | hand lexer, nesting 5, dialect branches inline (pg/my/sqlite); split into a `rawScanner` struct with `scanQuoted/scanComment/scanDollar/scanParam` methods (Medium, M) |
| 138   | `migrate/postgres/render.go:47` `renderChange`                                                             | S6                                                                                                                                                                     |
| 126   | `migrate/diff.go:242` `differ.tables`                                                                      | D5                                                                                                                                                                     |
| 116   | `gen/emit_descriptors.go:197` `EmitDescriptors`                                                            |                                                                                                                                                                        |
| 102   | `migrate/chain.go:28` `loadChain`                                                                          |                                                                                                                                                                        |
| 99    | `cmd/momo/generate.go:45` `runGenerate`                                                                    | flag parsing + generation + reporting                                                                                                                                  |
| 92    | `gen/load.go:27` `Load`                                                                                    |                                                                                                                                                                        |
| 88    | `insert.go:450` `valueRows`; `migrate/diff_split.go:35` `splitNoTx`; `migrate/split.go:176` `splitter.run` |                                                                                                                                                                        |
| 87    | `gen/emit_descriptors.go:457` `emitRelation`                                                               |                                                                                                                                                                        |
| 85    | `gen/model.go:250` `discovery.build`                                                                       |                                                                                                                                                                        |
| 82    | `driver/pgxdriver/vector_codec.go:474` `decodeBinary`                                                      | type switch, acceptable                                                                                                                                                |
| 81    | `load.go:351` `topNQuery`; `migrate/postgres/render.go:489` `renderAlterColumn`                            |                                                                                                                                                                        |
| 80    | `migrate/diff.go:466` `differ.columns`                                                                     |                                                                                                                                                                        |
| 79    | `migrate/order.go:335` `sortTables`                                                                        |                                                                                                                                                                        |
| 74    | `update.go:127` `plan`                                                                                     |                                                                                                                                                                        |

No god functions above 165 lines. The count above 70 lines is 36 across non-generated code, 19 above 80.

---

## 10. Ranked top 15

Ranking weighs expected maintenance cost per week against effort.

| #   | Finding                                                                                                                                                                                    | Priority | Effort | Where  |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------- | ------ | ------ |
| 1   | Version strings and docs not tied to releases: README/llms/ai-guide say v0.1.0 at v0.3.0; install commands pin the old tag                                                                 | H        | S      | Doc1   |
| 2   | Per-review test accretion (16 `_review/_polish/_more/_fixes/_repair/_hardening` files, 14 `load*` files); consolidate by production file and ban the suffixes                              | H        | M      | T1     |
| 3   | Three inconsistent integration-test conventions; untagged integration tests silently skip on macOS/Windows jobs                                                                            | H        | S      | T2     |
| 4   | `ARCHITECTURE.md` presents about 12 unbuilt APIs and packages as binding contract                                                                                                          | H        | M      | Doc2   |
| 5   | Comparison method set written 7 times (Column, Agg, exprOps, 4 permission wrappers); `Agg` already inconsistent                                                                            | H        | M      | D1     |
| 6   | `InsertQuery` five-mode god type; `insert.go` 1072 lines mixing build/render/chunk/tx scope; shared tx-scope helper missing                                                                | H        | M-L    | S1     |
| 7   | Error construction scattered: 10 helpers plus 28 inline literals; bare `errors.New` without sentinel (not `errors.Is`-able)                                                                | H        | S      | D4, E1 |
| 8   | `SelectQuery.renderBody` 160 lines and a second partial renderer in `ExprSelect`; `onConflictFollows` SQLite flag threaded through signatures                                              | H        | M      | S2     |
| 9   | `migrate/diff.go` four copy-pasted object diffs (views, functions, triggers, policies); generic helper already exists                                                                      | H        | S      | D5     |
| 10  | Dialect handled by both capability flags and 19 `Name ==` checks; SQLite/MySQL branches and `sqliteLiteral` (60 lines) exist with no driver and no real-DB test                            | M-H      | M      | S3, S4 |
| 11  | Root package has no package doc; 112+ undocumented exported methods in `permissions.go`/`expr.go`                                                                                          | M        | S      | A3, X2 |
| 12  | Fuzz targets: 5 of 15 never fuzzed in CI (hand-maintained `FUZZ_TARGETS`); no check ties list to code                                                                                      | M        | S      | T5     |
| 13  | Swallowed errors: `time.Parse` in `migrate/history.go:116`, 7x `_ = c.Release()`, `_ = opts` in `FromConnConfig`; pinned-connection logic duplicated in pgxdriver (`pin.go` vs `sqldb.go`) | M        | S-M    | E2, D6 |
| 14  | SQL-string assertions in about 140 root test lines and a mislabelled "golden" test; move to `internal/golden` files                                                                        | M        | M      | T3     |
| 15  | `sleep`-based sync in 17 test sites (flake risk under `-race`); CI runs unit tests about 10 times per push                                                                                 | M        | S      | T4, B1 |

Honourable mentions just below the line: `scanRaw` hand lexer split (M/M); `cmd/momo/migrate.go` atomic file helpers to `internal/atomicfs` (L/S); plugin API with zero plugins (L/S); `internal/namingrec` 10.5k generated lines unmonitored by dependabot (L/S); `.procoder/` 42k lines in tree (L/S); `Column.As` vs `As()` naming (M/M, breaking).

## 11. Overall assessment

The code is better than the speed of growth suggests: the import graph is acyclic and enforced by a test, immutability and copy-on-write conventions are uniform, error kinds are a proper sentinel plus `*Error` design with `errors.Is`/`errors.As` support, panics are rare and documented, TODO markers are absent, and most public functions carry accurate, detailed godoc. There is no function above 165 lines and no file that cannot be split mechanically.

The maintenance risk is concentrated in four places that all trace back to parallel, per-story work: (1) test files named after review rounds and three different integration conventions, (2) documentation that describes planned or past states (stale version strings, an architecture contract for unbuilt packages, one fact stated in four places), (3) duplication that the reviewers each tolerated locally (seven comparison method sets, ten error helpers, four diff blocks, two pinned-connection implementations), and (4) forward-looking SQLite/MySQL code in the core builder that ships and costs docs and tests but has no driver. None of these block development today; all of them multiply the cost of the next feature in `insert.go`, `select.go` or `migrate/diff.go`. A two-week cleanup pass on items 1-9 would remove most of it with low behavioural risk; items 5-8 are the only ones that touch public types and should land before 1.0.
