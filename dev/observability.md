# Observability hooks

Momo calls the hooks you register with `momo.WithHook` around every statement
(`momo.Hook`) and around every relation load (`momo.RelationHook`, for a hook
that also implements it). The interfaces live in the root package because `DB`
calls them. Ready-made implementations:

| Hook       | Constructor                                | What it does                                                                            |
| ---------- | ------------------------------------------ | --------------------------------------------------------------------------------------- |
| `Recorder` | `&obs.Recorder{}` (package `obs`)          | Keeps every finished statement and relation load for assertions in tests.               |
| `NPlusOne` | `obs.NewNPlusOne(n, report)` (`obs`)       | Reports a relation that was loaded more than `n` times inside one `obs.WithScope`.      |
| slog hook  | `momolog.New(logger, opts...)` (`momolog`) | Logs one `log/slog` record per statement and one per relation load (see [slog](#slog)). |

`obs` imports only `momo` and the standard library. All of these hooks are safe
for concurrent use.

## Arguments and sensitive columns

A hook sees bound argument values only when the DB was opened with
`momo.WithArgLogging`. A value bound from a column tagged `sensitive` reaches
every hook as `momo.RedactedArg` (`[redacted]`), never as the real value, and
the hooks here cannot recover it. `Recorder` stores a copy of the arguments and
hands out copies. The slog hook logs arguments only when you also pass
`momolog.Values(true)`.

## Recorder

```text
rec := &obs.Recorder{}
db := momo.New(drv, momo.WithHook(rec))
// run the code under test
rec.SQLs()      // the SQL text of each statement, in order
rec.Events()    // the full momo.QueryEvent of each, plus CtxSQL
rec.Relations() // the momo.RelationEvent of each relation load, with its Err
rec.Reset()
```

`Event.CtxSQL` is the value `Recorder` put into the context in `BeforeQuery`; it
equals `SQL` when the context reached `AfterQuery` intact, which makes it a
check that the hook chain passes the context through.

## N+1 detection

Two detectors exist, and they count different things:

| Detector               | Counts                                        | Scope                      | Use it for                                           |
| ---------------------- | --------------------------------------------- | -------------------------- | ---------------------------------------------------- |
| `obs.NewNPlusOne`      | relation loads (`Load`, `LoadAll`, `Preload`) | `obs.WithScope(ctx)`       | a relation `Load` inside a loop over the parents     |
| `momo.NewNPlusOneHook` | repeated statement shapes                     | `momo.WithQueryScope(ctx)` | any repeated statement, such as a `Select` in a loop |

Use either or both. Each reads only its own scope, so a context needs
`obs.WithScope` for the first and `momo.WithQueryScope` for the second; a
context with only one of them is not tracked by the other detector.

```text
det := obs.NewNPlusOne(3, func(f obs.Finding) {
    log.Printf("N+1: %s loaded %d times", f.Relation, f.Count)
})
db := momo.New(drv, momo.WithHook(det))

ctx := obs.WithScope(r.Context()) // one request, one job
```

`obs.NPlusOne` counts relation loads per relation within a scope. It reports
once per relation and scope, when the count first exceeds the threshold. One
batched `Load` is one load however many parents it covers, so it never triggers
a finding; a `Load` inside a loop over the parents does. `Finding.Parents` is
the parent count of the load that crossed the threshold; 1 is the usual sign of
a loop. A context without a scope is not tracked, a scope tracks at most 1024
relations, and a panic in `report` is recovered and dropped. `report` runs on
the loading goroutine and must not query the database. Create the detector with
`obs.NewNPlusOne`; its zero value must not be used.

## slog

The slog hook is `momolog.New`; there is one slog hook, not two.

```text
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
db := momo.New(drv, momo.WithHook(momolog.New(logger,
    momolog.Level(slog.LevelInfo),
    momolog.SlowOver(200*time.Millisecond), // slower statements log at WARN
)))
```

A statement record (`momo.query`) has `op`, `table`, `relation` (when set),
`sql`, `duration`, `rows`, `err` (when set) and, with `momolog.Values(true)` and
`momo.WithArgLogging`, `args`. A relation record (`momo.relation`) has `parent`,
`name`, `kind`, `parents`, `children`, `queries`, `duration` and `err` (when
set). Errors are logged through `Error()` only.

## Tenants

Tenant isolation stays row-level security through `WithTxSetup`; `obs` adds no
tenant helper.
