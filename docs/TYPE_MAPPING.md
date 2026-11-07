# Go to SQL type mapping

This page lists which SQL column type Momo uses for each Go field type, what
survives a round trip, and where the edges are. The tables are checked by
tests: `TestTypeMappingDoc` fails when a row here disagrees with the generator's
type map or a codec's `SQLType`, and `TestTypeMapping` creates a column of each
documented PostgreSQL type on a live server and round-trips edge values through
it.

## How a type is chosen

1. A field whose type implements the codec contract (`types.Codec`: `Scan`,
   `Value` and `SQLType`) uses its own `SQLType`. This is how `types.TimeOfDay`
   and `types.Hstore` get their columns.
2. Otherwise the generator maps the Go type with its built-in table
   (package `gen/typemap`). A pointer and the `sql.Null*` wrappers map like the
   type they wrap: nullability is a column property, not a type property.
3. A Go type with no mapping (for example `uint64` or `map`) makes
   `typemap.Map` return an error wrapping `momo.ErrUnsupported`, which the
   generator reports at the field. Give the type a codec or declare the column
   yourself.

## Generated mappings

`N` is the `size` tag for `string` and the `dim` tag for vector types. A
`vector.Vector` or `vector.HalfVector` field without `dim` is supported: it maps
to the bare `vector` or `halfvec` type, a dimensionless column that holds
embeddings of any size (see [vectors.md](vectors.md#dimensionless-columns)).
`vector.SparseVector` and `vector.Bits` require `dim`. SQLite has no vector
types.

| Go type                      | PostgreSQL                 | SQLite        |
| ---------------------------- | -------------------------- | ------------- |
| `int`                        | `bigint`                   | `INTEGER`     |
| `int8`                       | `smallint`                 | `INTEGER`     |
| `int16`                      | `smallint`                 | `INTEGER`     |
| `int32`                      | `integer`                  | `INTEGER`     |
| `int64`                      | `bigint`                   | `INTEGER`     |
| `float32`                    | `real`                     | `REAL`        |
| `float64`                    | `double precision`         | `REAL`        |
| `bool`                       | `boolean`                  | `INTEGER`     |
| `string`                     | `text`                     | `TEXT`        |
| `string with size:N`         | `character varying(N)`     | `TEXT`        |
| `[]byte`                     | `bytea`                    | `BLOB`        |
| `time.Time`                  | `timestamp with time zone` | `TEXT`        |
| `types.UUID`                 | `uuid`                     | `TEXT`        |
| `types.Date`                 | `date`                     | `TEXT`        |
| `types.JSON[T]`              | `jsonb`                    | `TEXT`        |
| `vector.Vector`              | `vector(N)`                | `unsupported` |
| `vector.HalfVector`          | `halfvec(N)`               | `unsupported` |
| `vector.SparseVector`        | `sparsevec(N)`             | `unsupported` |
| `vector.Bits`                | `bit(N)`                   | `unsupported` |
| `vector.Vector (no dim)`     | `vector`                   | `unsupported` |
| `vector.HalfVector (no dim)` | `halfvec`                  | `unsupported` |

On SQLite, `bool` is stored as 0 or 1, and `time.Time`, `types.UUID`,
`types.Date` and `types.JSON[T]` are stored as text. The SQLite column is what
the generator emits; it is not yet exercised against a live SQLite database
because the SQLite driver is not part of the current build.

## Codec-provided mappings

These types are not in the generator's table. Their own `SQLType` method names
the column; the generator's table is the fallback for everything else.

| Go type           | PostgreSQL | SQLite |
| ----------------- | ---------- | ------ |
| `types.TimeOfDay` | `time`     | `TEXT` |
| `types.Hstore`    | `hstore`   | `TEXT` |
| `types.TSVector`  | `tsvector` | `TEXT` |
| `types.TSQuery`   | `tsquery`  | `TEXT` |

`types.Hstore` needs the `hstore` extension; the vector types need `vector`
(pgvector). Momo never runs `CREATE EXTENSION` for you.

## Mappings you declare yourself

The generator never emits these columns, but the Go types work against them
when you write the DDL (or a migration) by hand. There is no SQLite column to
document because nothing generates one.

| Go type                 | PostgreSQL  | SQLite |
| ----------------------- | ----------- | ------ |
| `time.Time (timestamp)` | `timestamp` | `n/a`  |
| `[]string`              | `text[]`    | `n/a`  |
| `string (decimal)`      | `numeric`   | `n/a`  |

## Edge values and behavior

Each statement below is asserted by a test against PostgreSQL.

- Integers: `math.MaxInt64` and `math.MinInt64` round-trip through `bigint`;
  `int` is 64-bit and maps to `bigint`. A value that does not fit the column
  (an `int64` above `math.MaxInt16` into `smallint`) is a database error, not a
  silent truncation. `uint64` has no mapping and is rejected when bound if it
  exceeds `math.MaxInt64`.
- Floats: `NaN`, `+Inf` and `-Inf` are legal `double precision` and `real`
  values and round-trip. Compare with `math.IsNaN`, since `NaN != NaN`.
- Strings: any valid UTF-8 round-trips, including quotes and backslashes (they
  are bound, never interpolated). PostgreSQL rejects U+0000 in `text`. A string
  longer than `size` is a database error on `character varying(N)`.
- `[]byte`: `nil` stores NULL; an empty non-nil slice stores an empty `bytea`.
- `time.Time` on `timestamptz`: the instant is preserved, the zone is not. A
  value written at `+05:30` reads back as the same instant in the server
  session's location, so compare with `Time.Equal`, not `==`. Precision is
  microseconds; finer digits are cut. Times before 1970 and year 9999 work.
- `time.Time` on `timestamp` (no zone): you store a wall clock. Pass
  `t.UTC()` and treat the column as UTC; a value in another zone is stored as
  its UTC wall clock.
- `types.Date`: stores the year, month and day of `Time` in its own location;
  it reads back at midnight UTC. Compare the date, not the zone.
- `types.UUID`: round-trips all 16 bytes, including the all-zero UUID.
- `types.JSON[T]`: encodes with `encoding/json`. An empty slice and a nil
  slice are different JSON (`[]` and `null`) and read back as such.
- `types.TimeOfDay`: nanoseconds since midnight, truncated to microseconds
  (PostgreSQL `time` precision); `86399999999000` is the last representable
  value, and a value outside one day is an error.
- `types.Hstore`: a nil `*string` value is an hstore NULL; keys and values
  with quotes and backslashes are escaped.
- `types.TSVector` and `types.TSQuery`: the server's text form, with the
  lexemes sorted and normalized, so write values in that canonical form to
  compare them with `==`. An empty tsvector reads back empty. Full-text
  expressions are PostgreSQL only (see docs/fulltext.md).
- `[]string` on `text[]`: elements containing commas, quotes, the text `NULL`,
  or empty strings round-trip; an empty slice reads back empty.
- Decimal: carry `numeric` as a string to avoid binary floating point. Values
  beyond `float64` precision round-trip exactly, and `numeric` also accepts
  `NaN`.

## Null handling

A column that can be NULL needs a Go type that can hold NULL: a pointer
(`*int64`), a `sql.Null*` wrapper, or a type whose `Scan` accepts nil.
Scanning NULL into a plain `int64` or into a codec from the `types` package is
an error, never a silent zero value.
