# Upgrading

Momo is pre-v1 and makes no compatibility promise between releases; see the
[stability policy](supported-versions.md#stability-policy). Read this page and
the `CHANGELOG.md` entry of a release before you upgrade, and expect that
later releases can break more.

## Upgrading to 0.8.0

### `ToSnake` keeps a plural initialism together: `RoleIDs` is `role_ids`

`naming.ToSnake`, and with it the column name of the default `momo` naming
preset and the column of a plain struct that `momo.Query` and `momo.QueryRows`
scan, used to split an initialism followed by a plural `s`: `RestrictRoleIDs`
gave `restrict_role_i_ds`. The rule now is exact. It applies to an upper-case
run followed by a lone lower-case `s` that ends the name or is followed by
anything other than a lower-case letter, when the run ends in a word of the
list. The `s` stays with the last word, and the name breaks between the words
of the run:

1. If the whole run is a sequence of list words, it is cut into them. When
   there are several such cuts, the last word is the longest one that leaves a
   prefix that can be cut as well, and the prefix follows the same rule. So
   `HTTPIDs` is HTTP and ID (`http_ids`, not HTT and PID), `XMLHTTPIDs` is XML,
   HTTP and ID (`xml_http_ids`), `APIIDs` is API and ID (`api_ids`), and `PIDs`,
   `OIDs`, `SSIDs`, `GUIDs` and `GUIs` are one word each (`pids`, `oids`,
   `ssids`, `guids`, `guis`).
2. Otherwise the run is cut once, before the longest list word it ends with,
   and the letters before it stay together: `ABCIDs` is `abc_ids`.

A name where the run does not end in a list word (`ABs`), or where the `s`
starts a lower-case word (`IDsomething`), is unchanged.

The word list is ACL, API, ASCII, CPU, CSS, DNS, EOF, GUI, GUID, HTML, HTTP,
HTTPS, ID, IP, JSON, LHS, OID, PID, QPS, RAM, RHS, RPC, SLA, SMTP, SQL, SSH,
SSID, TCP, TLS, TTL, UDP, UI, UID, URI, URL, UUID, VM, XML, XMPP, XSRF and XSS.
Examples of columns that change (any Go field name that matches the rule does):

| Go field  | Column before | Column now |
| --------- | ------------- | ---------- |
| `RoleIDs` | `role_i_ds`   | `role_ids` |
| `IDs`     | `i_ds`        | `ids`      |
| `URLs`    | `ur_ls`       | `urls`     |
| `UUIDs`   | `uui_ds`      | `uuids`    |
| `APIs`    | `ap_is`       | `apis`     |
| `UserIPs` | `user_i_ps`   | `user_ips` |
| `PIDs`    | `pi_ds`       | `pids`     |
| `OIDs`    | `oi_ds`       | `oids`     |
| `SSIDs`   | `ssi_ds`      | `ssids`    |
| `GUIs`    | `gu_is`       | `guis`     |

Names without a plural initialism (`UserID`, `APIKeys`, `HTTPServer`) are
unchanged, and so is every other name: a test compares the old and the new
algorithm on every name built from a set of parts and requires them to agree
wherever the rule does not apply.

This is a breaking change, and the migration is not a rename. `momo migrate
diff` never guesses a rename (see [migrations](migrations.md)): for a field
whose column name changes it sees the old column go and a new one appear, so it
writes a `DROP COLUMN` and an `ADD COLUMN`, and it prints a `hint: possible
rename` line. Applied with `--allow-destructive`, that drops the data of the old
column. Do one of these before you run `diff`:

- Pin the old name on the field and nothing changes in the database:
  ``RoleIDs []string `momo:"column:role_i_ds"` ``. A plain struct scanned by
  `Query` needs the same tag.
- Or accept the new name and ask for a rename:
  `momo migrate diff <name> --rename-column table.role_i_ds=role_ids` (the flag
  repeats, one per column).

The same rule decides every other name that momo derives from a Go name, so
check them too: a table name derived from a model name (`Plural(ToSnake(model))`,
for a model such as `APIs` or `UserIDs`) changes the same way (use
`--rename-table old=new`, or the `table:` tag), and so do the names of indexes,
unique constraints and foreign keys that are built from the column
(`<table>_<column>_idx`, `_key`, `_fkey`). Models under the `gorm`, `bun`, `ent`
and `prisma` presets are not affected, they have their own rules.

### `--from-db` writes initialisms in capitals in more Go field names

This is a second, separate breaking change: it renames Go fields, not columns.
`momo generate --from-db` writes a column word that is an initialism in
capitals (`user_id` is `UserID`). It now reads the same word list as `ToSnake`
above, which is longer than the list it had (it had `id`, `url`, `uri`, `uuid`,
`api`, `http`, `https`, `ip`, `json`, `sql`, `html`, `xml`, `tls`, `ssh` and
`cpu`), and a plural `s` after a word of the list stays with it. The words
added are `acl`, `ascii`, `css`, `dns`, `eof`, `guid`, `lhs`, `qps`, `ram`,
`rhs`, `rpc`, `sla`, `smtp`, `tcp`, `ttl`, `udp`, `ui`, `uid`, `vm`, `xmpp`,
`xsrf`, `xss`, `gui`, `oid`, `pid` and `ssid`. Examples of field names that change when you regenerate:

| Column     | Field before | Field now |
| ---------- | ------------ | --------- |
| `team_ids` | `TeamIds`    | `TeamIDs` |
| `role_ids` | `RoleIds`    | `RoleIDs` |
| `urls`     | `Urls`       | `URLs`    |
| `api_ids`  | `APIIds`     | `APIIDs`  |
| `ttl`      | `Ttl`        | `TTL`     |
| `dns_name` | `DnsName`    | `DNSName` |
| `tcp_port` | `TcpPort`    | `TCPPort` |

The column name does not change, and the generated `momo` tag still maps the
field to it, so the database and the migrations are untouched. Code that uses
the old Go field name stops compiling: `m.TeamIds` must become `m.TeamIDs`.
In the `--from-db` goldens of this repository the only name that changed is
`TeamIds` to `TeamIDs`, in `gen/fromdb/googleuuid_test.go`.

To keep an old field name, pin it in the `introspect.columns` rules of
`momo.yaml` (see "The introspect section" in [migrations](migrations.md)): a
rule has a `match` glob over `table.column` and a `name` for the Go field, and
a later rule wins.

```yaml
introspect:
  columns:
    - match: "members.team_ids"
      name: TeamIds
```

There is no per-table `columns` map: the key is the list `introspect.columns`,
with `match` and `name`.

### `--from-db` keeps singular table names that end in `-ss`, `-us` and a few `-is`, `-as` and `-ns` words

`momo generate --from-db` named the table `class` the model `Clas` (and
`assessment_class` `AssessmentClas`), the same family as the `-ses` words fixed
in 0.7.0. The rules for a table name that is already singular are now:

- A word in `-ss` is kept: `class`, `address`, `access`, `process`, `glass`,
  `boss` give `Class`, `Address`, `Access`, `Process`, `Glass`, `Boss`.
- A word in `-us` is kept, except the plurals of words in `-u`: `status`,
  `virus`, `bonus`, `census`, `corpus`, `nucleus`, `stimulus`, `campus` and
  `bus` give `Status`, `Virus` and so on (not `Statu`), while `menus`, `emus`,
  `gnus`, `haikus`, `tutus` and `kudus` give `Menu`, `Emu`, `Gnu`, `Haiku`,
  `Tutu` and `Kudu`.
- These singular words keep their `s`: `analysis`, `axis`, `crisis`, `basis`,
  `thesis`, `iris`, `synopsis` and `diagnosis`; `alias`, `canvas`, `atlas`,
  `bias` and `gas`; `news` and `lens`. They are listed, not matched by suffix,
  so `areas` still gives `Area` and `ideas` gives `Idea`.

A plural such as `classes`, `statuses` or `aliases` is singularized as before.
After you regenerate, a model that had the wrong name changes name and its
column package changes with it (`clas` to `class`); pin
`introspect.tables.<table>.name` to keep the old name.

### `momo.WithNaming`: a plain-struct `Query[T]` follows the naming preset

`momo.Query` and `momo.QueryRows` into a plain struct named every untagged
field's column as the snake_case of the field name, so under `naming: prisma`
(camelCase columns such as `endpointId`) each field needed a `column:` tag.
The new option `momo.WithNaming(naming.Prisma{})` sets the preset on the DB, and
an untagged field then reads the column the preset gives its name (`EndpointID`
is `endpointId`). A `column:` tag still wins. Without the option nothing
changes: snake_case, as `naming.ToSnake` gives it. Any preset of `gen/naming`
(or any value with a `ColumnName(goField string) string` method, comparable or
not) fits, and the option applies to handles derived from the DB (transactions,
`OnConn`, replicas). A DB with a preset keeps its own plans, built once per type
from the field names, so the preset must give the same answer every time. Known
limit: the preset is not read from `momo.yaml` or from a generated registration
at run time; pass the same one you set as `naming:` there.

### `--from-db` no longer writes the objects of tables it does not model

`momo generate --from-db` used to put the policies and triggers of a table left
out by `introspect.skip` or `introspect.only` on the alphabetically first model.
An object that belongs to a table with no model now gets no output: it is not
written to any model and it is not in the init snapshot. The functions that only
those objects use are dropped too, unless `introspect.functions: all` is set,
which still writes every function. After you regenerate, the first model loses
its `c.Policy`, `c.Trigger` and `c.Function` lines for those tables, and it no
longer changes when a migration adds row level security to a skipped table. If
the database keeps those objects of a table left out by `introspect.skip`, add
the table to `migrate.exclude` so drift does not report them as unmanaged. With
`introspect.only` nothing is needed: `migrate.only` already hides them.

### Indexes attached to a parent partitioned index are no longer inspected

The PostgreSQL inspector used to report an index on a partition that is
attached to a parent partitioned index as a standalone index of that partition.
`momo generate --from-db` then emitted it as `c.Index(...)` on the partition's
model, a fresh install created it twice, and `drift --strict` reported the
PostgreSQL-created index as `not in snapshot`. Inspection now skips every index
that has a `pg_inherits` parent; only the parent index is modeled. An index
created directly on a partition, and a unique index created directly on one, is
not attached and is still modeled on that partition.

A snapshot written by 0.7.0 from such a database (the init snapshot of
`generate --from-db`, or a snapshot taken by `baseline`) lists the attached
indexes on the partitions. After the upgrade the live side no longer reports
them, so `migrate drift` reports each one as `missing: index
<partition>.<name>` (present in the snapshot, absent live) and exits non-zero,
and `baseline` reports the database as differing from the snapshot. `migrate
diff` between your models and the snapshot is still empty, because both carry
the index.

What to do, per project that has a partitioned table with a parent index:

1. Re-run `momo generate --from-db` with 0.8.0 against a database whose
   partitions were created by PostgreSQL, so the models and the snapshot both
   lose the attached partition indexes, or edit both by hand: delete the
   `c.Index(...)` lines the partitions carry for attached indexes and the same
   entries from the snapshot. Do not change only the models. The diff would
   then plan `DROP INDEX` on an attached partition index, which PostgreSQL
   refuses.
2. A database that was built from the 0.7.0 init migration has the duplicate
   partition indexes (the one PostgreSQL attached and the one momo declared).
   Drop the extra, unattached one by hand; `migrate` does not do it.
3. A database that was only ever baselined needs nothing else; drift is clean
   once the snapshot is regenerated.

### `pgxdriver.Listen` and `pgxdriver.Notify` for LISTEN/NOTIFY

New API, nothing to change in existing code. `pgxdriver.Listen` opens a
dedicated connection, subscribes to channels and reconnects with backoff;
`pgxdriver.Notify` sends with `pg_notify`. If you kept a raw pgx
`WaitForNotification` loop beside momo, you can move it onto the `Listener`.
See [LISTEN and NOTIFY](listen-notify.md).

### `dbdefault` on a pointer field writes false and 0

A `dbdefault` field on a NOT NULL column can now be a pointer. The column stays
NOT NULL. A nil pointer means "use the database default": `Model` and `Models`
leave the column out of the INSERT (or write `DEFAULT` in a multi-row VALUES
list) and `Exec` reads the stored value back into the pointer with `RETURNING`.
A non-nil pointer is always written, so `false` and `0` reach the database:

```text
type Account struct {
	momo.Model `momo:"table:accounts"`
	ID         int64  `momo:"pk,identity"`
	Active     *bool  `momo:"default:true,dbdefault"`
	Level      *int64 `momo:"default:3,dbdefault"`
}

a := Account{Active: ptr(false)} // stores false; Level is nil, so 3 is read back
```

`generate` used to refuse `dbdefault` on a pointer. It still refuses it on a
nullable column (a `sql.Null*` type or the `nullable` tag), because nil would
stop meaning NULL there. Non-pointer `dbdefault` fields behave as before. The
read-back rules, the dialects without `RETURNING` (plain MySQL fails with
`ErrUnsupported` for a nil pointer; an explicit value is still written) and the
mixed-row limits of SQLite are the same as for any other `dbdefault` field; see
"Database defaults" in the [AI guide](ai-guide.md).

`Update(...).Model(&m)` treats a nil `dbdefault` pointer as "not set": the
column is left out of `SET` and keeps its stored value, because writing NULL
would fail on the NOT NULL column. A pointer to `false` or `0` is written. If
no column is left to set, the update fails with the usual `ErrValidation` ("no
columns to update"). Before this change a pointer `dbdefault` field could not be
generated, so no generated model sees a different result. `Upsert` takes its
`DO UPDATE` values from `EXCLUDED` and already leaves out a `dbdefault` column
that any row leaves to the database, so it needs no change.

`momo generate` now warns (on standard error, exit code unchanged) about a
non-pointer `dbdefault` field of a `bool` or number type whose default is a
non-zero constant, because the zero value means "use the default" there and an
explicit `false` or `0` is replaced by it. Make the field a pointer to write
them.

### `--from-db` generates a pointer for a non-zero constant default

With `introspect.dbdefault: all`, `momo generate --from-db` gives a NOT NULL
`bool` or number column whose constant default is `true` or non-zero
(`DEFAULT true`, `DEFAULT 1` and above) a pointer type with `dbdefault`, for
example `Active *bool` with `` `momo:"default:true,dbdefault"` ``. A plain field
would silently replace an explicit `false` or `0` with the default. A column
with a zero default (`DEFAULT false`, `DEFAULT 0`), a text default or a computed
default keeps its plain type. After you regenerate with this option, fields of
those columns change from `bool`/`int64` to `*bool`/`*int64`, and code that
reads them dereferences the pointer. Without `introspect.dbdefault: all` nothing
changes, because a constant default is not tagged.

The warning and the pointer type cover only plain `bool` and number types
(`bool`, `int`..`int64`, `uint`..`uint64`, `float32`, `float64`). A named type,
a type override from `introspect.types` or `introspect.columns`, a decimal or a
string keeps its field as it is and gets neither.

### New: `RawRows` returns the hooked cursor of a raw statement

`momo.RawRows(ctx, q, sql, args...)` runs a hand-written statement and returns
an open cursor with `Next`, `Scan(dest ...any)`, `Columns`, `Err` and `Close`,
for a result whose shape is known only at run time (a column count that depends
on the input). `Query` and `QueryRows` need a type for the row and `MapRows`
buffers every row into a map; before this, a `Scan(dest...)` cursor meant
`db.Driver().Query`, which skips hooks, `WithTxSetup`, error classification and
redaction. `RawRows` takes the raw path of `Query`: the same placeholders, hooks
(op `raw`), interceptors, `WithTxSetup` and `WithImplicitTx`, error
classification and argument redaction, on a `*DB`, a `*Tx` or an `AdoptTx`
handle.

```text
rows, err := momo.RawRows(ctx, db, `SELECT id, title FROM tasks WHERE tenant_id = $1`, tenant)
if err != nil {
	return err
}
defer rows.Close()
for rows.Next() {
	if err := rows.Scan(&id, &title); err != nil {
		return err
	}
}
return rows.Err()
```

The return type is the new interface `momo.ColumnRows`, a `Rows` that also has
`Columns`, because `Rows` itself has no column names. Close the cursor, always:
one that is never closed holds its connection, and on a `*DB` with
`WithImplicitTx` the transaction behind it. When the context is canceled,
`Next` returns false, the cursor is closed and `Err` reports `ErrCanceled`; a
context canceled after the last row was read can still report `ErrCanceled`,
as in `QueryRows`. Scan errors go through the error classifier and are
recorded on the hook event.

Under `WithImplicitTx`, momo treats raw SQL as a write, because raw SQL may
write. The transaction commits when the rows are read to the end. It rolls back,
and `Close` returns `ErrValidation`, when `Close` comes before the end of the
rows (including a `Close` with no `Next`), after a Scan or driver error, or
after a canceled context. Without `WithImplicitTx` an early `Close` is just a
`Close`. `Columns` returns the names as the driver reports them: an empty list
means the statement returns no columns (an `UPDATE` without `RETURNING`) or the
driver cannot name them, and `ErrUnsupported` means the cursor cannot report
columns at all. This is additive; nothing to change.

### The history importer replays migrations in transactions and writes `-- momo:imported`

`history.Baseline` used to replay every statement of a converted migration in
autocommit. It now runs a migration that the source did not mark as
non-transactional between `BEGIN` and `COMMIT` (rolled back when a statement
or the `COMMIT` fails; this holds on PostgreSQL and SQLite, while MySQL
commits DDL implicitly), as goose, golang-migrate, Atlas, Bun and `momo migrate apply` run it.
A migration that used `CREATE TEMP TABLE ... ON COMMIT DROP`, `SET LOCAL` or
`set_config(..., true)` across statements now replays; one that relied on a
statement being committed before the next now sees it in the same transaction.
A non-transactional migration (`NoTx`) runs as before.

`history.WriteImport` now writes a new directive, `-- momo:imported`, as the
first line of every imported up file (before `-- momo:no-transaction` when
there is one), so `momo migrate validate` accepts the directory. Before,
validate reported each imported migration as a custom migration whose snapshot
differs from its parent's. `validate` skips that check for a file with the
directive, and `momo migrate rebase` treats the file as hand-written and refuses
to regenerate it. `migrate.Directives` has a new `Imported` field. A directory
you imported with an earlier release still fails validate: add
`-- momo:imported` as the first line of each imported up file and run
`momo migrate hash`.

### `UnnestColumn` and `Distinct` on aggregates

These are additions; no existing query changes.

`momo.UnnestColumn(col).As(alias, name)` expands an array column (or any
array expression) of an earlier source into rows, as a `LATERAL` source of a
join or FROM item. `CrossJoin(momo.UnnestColumn(q2Flags).As("fl", "flag"))`
renders `CROSS JOIN LATERAL unnest("q2"."flags") AS "fl" ("flag")`, and
`Column()` of the returned source reads the element. A NULL array yields no
rows. It is PostgreSQL only and fails the build with `ErrUnsupported` on SQLite
and MySQL. It reads a column of an earlier source, so it must come after the
table that holds the array: as the first FROM item it fails the build with
`ErrValidation`. `CrossJoin` drops the rows whose array is NULL or empty;
`LeftJoin(src, momo.SQL("true"))` keeps them with a NULL element, rendering
`LEFT JOIN LATERAL unnest(...) AS "fl" ("flag") ON true`. The element column
works in `Where` like any column. `momo.UnnestValues` still binds Go slices; use `UnnestColumn` when
the array comes from a column.

`agg.Call.Distinct()` renders `DISTINCT` inside the call: `Distinct()` on an
`agg.Array` of a column gives `array_agg(DISTINCT col)`, and it works on
`agg.String`, `agg.JSONB` and `agg.Func` (a `count` function gives
`count(DISTINCT col)`). It combines with `OrderBy` and
`Filter`. It takes exactly one argument, cannot be used with `Over`, and on
PostgreSQL every `OrderBy` expression must be the argument itself (as
PostgreSQL requires); each of these fails the build with `ErrValidation`. On `agg.PercentileCont` and `agg.PercentileDisc` it fails with
`ErrValidation`. SQLite supports it on single-argument aggregates, so
`agg.String(...).Distinct()` gives `ErrUnsupported` there; MySQL gives
`ErrUnsupported` for `agg.JSONB(...).Distinct()`.

### `--from-db` can write the inverse has_many and has_one

`momo generate --from-db` wrote a `belongs_to` on the child and nothing on the
parent, so `Has`, `HasNo`, `LoadCounts` and eager loading of children needed a
hand-written relation on every parent. Set `introspect.inverse: true` and it
also writes the inverse on the referenced model: `has_one` when the foreign key
column is unique (the primary key, a unique constraint, or a plain unique
index), `has_many` otherwise. It is off by default, so nothing changes until you
opt in. A hand-edited model file is never replaced, so a project that already
declared these relations by hand turns the option on and gets a `.new` file to
merge, or keeps the hand-written relations and leaves it off.

The name is the plural of the child model for `has_many` (`SubscriptionItems`)
and the child model name for `has_one`. A child with two foreign keys to the same
parent, and a table that references itself, get `By` and the foreign key field
without `ID` instead (`MessagesBySender`, `MessagesByRecipient`). Rename or skip
one with `introspect.relations`, keyed by model and derived name (User.MessagesBySender):

```yaml
introspect:
  inverse: true
  relations:
    "Subscription.SubscriptionItems": { name: Items }
    "User.MessagesBySender": { skip: true }
```

The key uses the name derived before the rule and before any collision suffix,
and `relations` has no effect unless `inverse` is `true`. A rule key that matches
no inverse is printed as a warning, and so is an inverse whose name collides with
another field of the parent and gets a number suffix (Orders2); the warning
names the key that renames it.

No inverse is written for a composite foreign key, for a foreign key that is left
out of the models, or from a table with no model (`only` and `skip`).

### `SelectExprs` takes joins, `GroupBy`, `Having` and `OrderBy`

`momo.SelectExprs(...)` had only `From` and `Where`. It now also has `Join`,
`LeftJoin`, `CrossJoin`, `GroupBy`, `Having` and `OrderBy`, rendered as in
`Select[T]`, so a grouped `INSERT ... SELECT` source can have any number of
columns. A join, `GroupBy` or `Having` without `From` fails with
`ErrValidation`, and so does an `As(...)` alias in `GroupBy`. There is no
`Limit` or `Offset` on it yet.

### Row-value comparison, `sqlfn.Div`, `sqlfn.CurrentDate`, infinity and `momo.Lit`

These are additions; nothing changes for existing code.

- `momo.Row(a, b).Lt(momo.Row(x, y))` (also `Le`, `Gt`, `Ge`) renders
  `(a, b) < (x, y)`, the position check of keyset pagination. Both rows need at
  least two elements and the same count, or the build fails with
  `ErrValidation`.
- `sqlfn.Div` and `sqlfn.DivExpr` divide, and `sqlfn.NullIf` and
  `sqlfn.NullIfExpr` build `NULLIF`, so `sqlfn.DivExpr(final, sqlfn.NullIf(max, 0))` avoids
  a division by zero. Integer operands divide as integers on PostgreSQL and
  SQLite, but `/` on integers returns a DECIMAL on MySQL. Division by zero is
  an error on PostgreSQL and, in strict mode, on MySQL: guard the divisor with
  `sqlfn.NullIf` on every dialect.
- `sqlfn.CurrentDate()` is `CURRENT_DATE`. `sqlfn.Infinity()` and
  `sqlfn.NegInfinity()` are `'infinity'::timestamptz` and
  `'-infinity'::timestamptz`; they are PostgreSQL only and fail with
  `ErrUnsupported` elsewhere.
- `momo.Lit(v)` writes a value into the SQL text instead of binding it, so the
  same expression is identical text in `SELECT` and `GROUP BY` (a bound value
  gets a new `$n` each time and PostgreSQL rejects the grouping with 42803),
  for example `COALESCE(col, 'none')`. It accepts integers, floats, `bool`,
  strings and `time.Time`. A string renders as a standard SQL string literal
  with each `'` doubled; a string containing a backslash or a NUL byte fails
  the build with `ErrValidation` on every dialect (MySQL and PostgreSQL with
  `standard_conforming_strings` off read a backslash as an escape). A
  `time.Time` renders as `DATE 'YYYY-MM-DD'` and must be exactly 00:00:00 UTC;
  a value with a time of day fails with `ErrValidation` instead of losing the
  time. A NaN or infinite float fails the build with `ErrValidation`.

### Union members keep their ORDER BY and LIMIT, and a union runs at the top level

`Union` used to render every member bare, so a
member's own `ORDER BY`, `LIMIT`, `OFFSET` or row lock could not be expressed.
A member that has any of those clauses is now grouped, so the clause binds to
that member and not to the whole union: in parentheses on PostgreSQL and MySQL,
and as `SELECT * FROM (member)` on SQLite, which rejects parentheses around a
compound member. A member without those clauses still renders bare, so existing
queries keep their SQL. A nested `Union` as a member is grouped the same way.

A union can now run directly. `momo.SelectUnion` (type parameter `T`, then the
querier, `all` and the members) returns a `UnionQuery[T]` with `OrderBy`, `Limit`, `Offset`, `SQL`, `All` and
`Rows`; `T` is a model, a projection, a tuple row or a plain struct, matched to
the result columns as for `momo.Query`. The outer `OrderBy` orders the whole
union and may name result columns only; build them with
`momo.ColumnOf` and an empty source:

```text
teachersTop := momo.Select[Hit](db).From(teachers).Columns(kind, id, name).
	OrderBy(name.Asc()).Limit(5)
studentsTop := momo.Select[Hit](db).From(students).Columns(kind, id, name).
	OrderBy(name.Asc()).Limit(5)
byName := momo.ColumnOf[string]("", "full_name")
rows, err := momo.SelectUnion[Hit](db, true, teachersTop, studentsTop).
	OrderBy(byName.Asc()).Limit(8).All(ctx)
```

A row lock inside a member returns `ErrUnsupported` on SQLite, as on a plain
select, and on PostgreSQL, which rejects `FOR UPDATE` inside a set operation;
MySQL accepts it in a parenthesised member. A member with its own `WITH`
clause is grouped like one with `ORDER BY`. An outer `OrderBy` that names a
qualified column or an expression fails at build time with `ErrValidation`.
Hooks and interceptors see op `select` with an empty table name.

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
