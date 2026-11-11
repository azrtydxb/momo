# momovet: static check for raw SQL

Momo binds every value as a parameter, but the raw-SQL functions take the statement text as a Go string. A string built with `+` or a format call from user input is an injection point. The `momovet` analyzer reports it.

## What it reports

A call to one of these functions of `github.com/azrtydxb/momo` whose SQL argument is not a constant:

| Function           | SQL argument                              |
| ------------------ | ----------------------------------------- |
| `momo.Exec`        | third (after the context and the querier) |
| `momo.Scalars`     | third (after the context and the querier) |
| `momo.MapRows`     | third (after the context and the querier) |
| `momo.Query`       | third (after the context and the querier) |
| `momo.QueryRows`   | third (after the context and the querier) |
| `momo.RawQuery`    | second (after the querier)                |
| `momo.SQL`         | first                                     |
| `momo.TypedSQL`    | first                                     |
| `momo.RawSubquery` | first                                     |

A constant is a string literal, a `const`, or an expression made only of constants (`"UPDATE t SET " + "x = 1"`). Anything else is reported as `non-constant SQL string passed to momo.<Name>`. Pass values as arguments after the SQL text instead of concatenating them.

## Marking a vetted call

When the text is built only from constants and allowlisted identifiers, mark the call with a line directive and say why:

```text
momo.Exec(ctx, db, "UPDATE "+table+" SET x = 1") //momovet:ok table comes from the schema allowlist

//momovet:ok column is checked against allowedColumns
momo.Query[int](ctx, db, "SELECT "+column+" FROM t")
```

The directive goes as a trailing comment on the line of the call (or of its SQL argument), or on the line directly above the call. It suppresses the report for that one call only. A call without a directive is still reported.

The directive is itself reported when:

- it has no reason (`//momovet:ok` alone): `//momovet:ok needs a reason`;
- it suppresses nothing, for example because the SQL is already constant or no momo raw-SQL call is next to it: `unused //momovet:ok directive`.

## Running it

```sh
go install github.com/azrtydxb/momo/cmd/momovet@latest
go vet -vettool="$(which momovet)" ./...
```

It also runs standalone: `momovet ./...`.

To embed it in your own multichecker, import `github.com/azrtydxb/momo/momovet` and use its `Analyzer` variable.

## Limits

The analyzer does not follow assignments. A `var` that holds a literal is reported, even a package-level one that is never reassigned: this is a false positive by design, so declare it as a `const`. It checks the call site only, so SQL built in one function and passed through another is reported where it reaches a raw-SQL function.
