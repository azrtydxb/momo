// SPDX-License-Identifier: Apache-2.0

package gendocs

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/gen/tags"
	"github.com/azrtydxb/momo/internal/typemap"
)

// family is one column of the dialect tables: a dialect value and the name
// shown for it. MySQL and MariaDB share the MySQL dialect name and its driver.
type family struct {
	Title string
	Name  dialect.Name
	D     *dialect.Dialect
}

func families() []family {
	return []family{
		{"PostgreSQL", dialect.Postgres, dialect.For(dialect.Postgres)},
		{"SQLite", dialect.SQLite, dialect.For(dialect.SQLite)},
		{"MySQL", dialect.MySQL, dialect.MySQLFlavor(dialect.FlavorMySQL)},
		{"MariaDB", dialect.MySQL, dialect.MySQLFlavor(dialect.FlavorMariaDB)},
	}
}

// statusBlock is the one text that says which databases are usable. README,
// llms.txt and four docs pages carry the same block.
func statusBlock(root string) (string, error) {
	drivers, err := scanDrivers(root)
	if err != nil {
		return "", err
	}
	cli, err := cliDialects(root)
	if err != nil {
		return "", err
	}
	groups := []struct {
		Title string
		Name  dialect.Name
	}{
		{"PostgreSQL", dialect.Postgres},
		{"SQLite", dialect.SQLite},
		{"MySQL and MariaDB", dialect.MySQL},
	}
	lines := []string{"A database is usable when a driver package for it exists under `driver/`:", ""}
	for _, g := range groups {
		ds := driversFor(drivers, g.Name)
		var s strings.Builder
		s.WriteString("- " + g.Title + ": ")
		if len(ds) == 0 {
			s.WriteString("not usable, no driver package exists for it under `driver/`.")
		} else {
			parts := make([]string, len(ds))
			for i, d := range ds {
				var notes []string
				if d.NeedsCGO {
					notes = append(notes, "needs cgo")
				}
				if !d.Windows {
					notes = append(notes, "not on Windows")
				}
				switch d.Vectors {
				case "yes":
					notes = append(notes, "vector builders: yes")
				case "no":
					notes = append(notes, "vector builders: no")
				default:
					notes = append(notes, "vector storage and builders depend on the server version")
				}
				parts[i] = code(d.Dir) + " (" + strings.Join(notes, ", ") + ")"
			}
			s.WriteString("usable through " + strings.Join(parts, " and ") + ".")
		}
		name := string(g.Name)
		switch {
		case cli[name]:
			s.WriteString(" Migrations: " + code("migrate/"+name) + ", run by `momo migrate`.")
		case hasMigrateDialect(root, name):
			s.WriteString(" Migrations: " + code("migrate/"+name) + ", library only (`momo migrate` does not select it).")
		default:
			s.WriteString(" Migrations: no migration dialect.")
		}
		lines = append(lines, s.String())
	}
	return strings.Join(lines, "\n"), nil
}

// nativeOnly labels the capability flags that mean native syntax, not
// availability: MySQL and SQLite emulate ILike, and MySQL emulates
// NullsOrdering, so the feature works where the flag says no.
var nativeOnly = map[string]string{
	"ILike":         "(native ILIKE; emulated elsewhere)",
	"NullsOrdering": "(native NULLS FIRST or LAST; emulated on MySQL)",
}

// capabilitiesBlock is the dialect status table: usable, driver packages,
// migration support, MaxParams and every boolean capability of dialect.Dialect
// plus the vector capabilities, one column per dialect.
func capabilitiesBlock(root string) (string, error) {
	drivers, err := scanDrivers(root)
	if err != nil {
		return "", err
	}
	cli, err := cliDialects(root)
	if err != nil {
		return "", err
	}
	fams := families()
	header := []string{"Item"}
	for _, f := range fams {
		header = append(header, f.Title)
	}
	row := func(label string, cell func(f family) string) []string {
		r := []string{label}
		for _, f := range fams {
			r = append(r, cell(f))
		}
		return r
	}
	rows := [][]string{
		row("Usable (a driver package exists)", func(f family) string {
			return yesNo(len(driversFor(drivers, f.Name)) > 0)
		}),
		row("Driver package", func(f family) string {
			var dirs []string
			for _, d := range driversFor(drivers, f.Name) {
				dirs = append(dirs, d.Dir)
			}
			if len(dirs) == 0 {
				return "none"
			}
			return codes(dirs)
		}),
		row("Migration dialect", func(f family) string {
			if !hasMigrateDialect(root, string(f.Name)) {
				return "none"
			}
			return code("migrate/" + string(f.Name))
		}),
		row("`momo migrate` runs it", func(f family) string { return yesNo(cli[string(f.Name)]) }),
		row("`MaxParams`", func(f family) string { return strconv.Itoa(f.D.MaxParams) }),
	}
	rt := reflect.TypeFor[dialect.Dialect]()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if sf.Type.Kind() != reflect.Bool {
			continue
		}
		label := code(sf.Name)
		if note, ok := nativeOnly[sf.Name]; ok {
			label += " " + note
		}
		rows = append(rows, row(label, func(f family) string {
			return yesNo(reflect.ValueOf(*f.D).Field(i).Bool())
		}))
	}
	rows = append(rows,
		row("`Vector.Storage`", func(f family) string { return yesNo(f.D.Vector.Storage) }),
		row("`Vector.Metrics`", func(f family) string {
			if len(f.D.Vector.Metrics) == 0 {
				return "none"
			}
			ms := make([]string, len(f.D.Vector.Metrics))
			for i, m := range f.D.Vector.Metrics {
				ms[i] = string(m)
			}
			return codes(ms)
		}),
		row("`Vector.Indexes`", func(f family) string {
			if len(f.D.Vector.Indexes) == 0 {
				return "none"
			}
			return codes(f.D.Vector.Indexes)
		}),
	)
	return table(header, rows), nil
}

// feature is one row of the feature support table. The per-dialect cells are
// the measured behavior; a dialect without a driver package shows as not
// usable whatever its cell says. API lists the identifiers the row is about,
// as package.Name, and the generator fails when one is missing from the code.
type feature struct {
	Name   string
	API    []string
	PG     string
	SQLite string
	MySQL  string
}

var features = []feature{
	{
		Name:   "Advisory session locks",
		API:    []string{"advisory.Acquire", "advisory.Release", "advisory.Try", "advisory.Shared", "advisory.Xact", "advisory.WithHash64"},
		PG:     "yes: `pg_advisory_lock`, `pg_try_advisory_lock` and `pg_advisory_unlock` on a pinned connection (`db.OnConn`); the shared and the transaction-scoped forms are also `yes`",
		SQLite: "`ErrUnsupported`",
		MySQL: "`GET_LOCK`, `GET_LOCK(name, 0)` and `RELEASE_LOCK` on a pinned connection (`db.OnConn`) on MySQL 8.4, MySQL 9.7 and MariaDB 11.8; the lock name is `momo:advisory:<db>:<key>`, scoped to the current database as on PostgreSQL, with an `int64` key in decimal and a text key as `s:` plus 32 hex digits of its sha256, and `advisory.WithHash64` has no effect; " +
			"`advisory.Xact` and `advisory.Shared` are `ErrUnsupported`, because a `GET_LOCK` lock belongs to the session, a commit does not release it and there is no shared mode",
	},
	{
		Name: "JSON operators on `types.JSON[T]`",
		API:  []string{"jsonop.Text", "jsonop.Path", "jsonop.Root", "jsonop.Contains", "jsonop.HasKey", "jsonop.DeleteKey", "jsonop.Concat", "jsonop.Set", "jsonop.TypeOf"},
		PG:   "yes",
		SQLite: "`jsonop.Text`, `jsonop.Path`, `jsonop.Root`, `jsonop.DeleteKey`, `jsonop.HasKey` and `jsonop.TypeOf` run on both SQLite drivers; a JSON boolean extracts as `1` or `0`; " +
			"`jsonop.Contains`, `jsonop.Concat` and `jsonop.Set` are `ErrUnsupported`",
		MySQL: "`jsonop.Text`, `jsonop.Path`, `jsonop.Root`, `jsonop.Contains`, `jsonop.HasKey`, `jsonop.DeleteKey` and `jsonop.TypeOf` run on MySQL 8.4, MySQL 9.7 and MariaDB 11.8; " +
			"a JSON boolean extracts as `true` or `false` and a JSON null member as the text `null`; `jsonop.Concat` and `jsonop.Set` are `ErrUnsupported`",
	},
	{
		Name: "Date and time expressions",
		API: []string{
			"timeop.Extract", "timeop.AtTimeZone", "timeop.Trunc", "timeop.Now", "timeop.CurrentTimestamp",
			"timeop.IntervalOf", "timeop.IntervalOfExpr", "timeop.IntervalDuration", "timeop.AddInterval", "timeop.SubInterval",
			"timeop.AddDuration", "timeop.SubDuration", "timeop.Sub", "timeop.Age", "timeop.EpochSeconds",
			"timeop.AddDays", "timeop.AddDaysExpr", "timeop.DaysBetween",
			"timeop.CurrentDate", "timeop.Infinity", "timeop.NegInfinity",
		},
		PG: "yes",
		SQLite: "every `timeop.ExtractField`, `timeop.TruncUnit`, interval and `timeop.Now` run on both SQLite drivers; month arithmetic overflows into the next month " +
			"(Jan 31 plus a month is Mar 2 or 3, where PostgreSQL and MySQL clamp); `timeop.Now` is millisecond text; `timeop.AtTimeZone` is PostgreSQL only; " +
			"`timeop.IntervalMillisecond` and `timeop.IntervalMicrosecond` keep milliseconds; `timeop.EpochSeconds` of `timeop.Sub` runs through `julianday` (about 0.1 ms), `timeop.Age` is PostgreSQL only; " +
			"`timeop.AddDays`, `timeop.AddDaysExpr` and `timeop.DaysBetween` read a `types.Date` stored as `YYYY-MM-DD` text (`date()` and `julianday`), and a bound infinite date fails the build with `ErrValidation`",
		MySQL: "every `timeop.ExtractField`, `timeop.TruncUnit`, interval (including milliseconds and microseconds), `timeop.Now`, `timeop.EpochSeconds` of `timeop.Sub`, `timeop.AddDays`, " +
			"`timeop.AddDaysExpr` and `timeop.DaysBetween` run on MySQL 8.4, MySQL 9.7 and MariaDB 11.8; month arithmetic clamps to the last day of the month; " +
			"`timeop.AtTimeZone` is PostgreSQL only; `timeop.Age` and `timeop.StatementTimestamp` are `ErrUnsupported` (golden-tested); a bound infinite `types.Date` fails the build with `ErrValidation`",
	},
	{
		Name:   "Deferrable constraints",
		API:    []string{"schema.Unique", "schema.ForeignKey", "schema.Table"},
		PG:     "yes: `DEFERRABLE` and `DEFERRABLE INITIALLY DEFERRED` on a unique constraint, a primary key and a foreign key are declared, read from `pg_constraint`, written by `generate --from-db`, rendered, compared by `diff`, `Baseline` and `drift`, and kept by a snapshot; a change of deferrability drops and adds the constraint again",
		SQLite: "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because `PRAGMA foreign_key_list` does not report the deferrability of a foreign key and a primary key or unique constraint cannot be deferred",
		MySQL:  "`ErrUnsupported`: MySQL and MariaDB check each constraint at the end of the statement and have no `DEFERRABLE`; a declaration is a generator error and the renderer refuses it",
	},
	{
		Name:   "Audit history tables",
		API:    []string{"schema.History", "schema.HistoryFor"},
		PG:     "yes: `Config.History` declares `<table>_history`, a trigger function and an `AFTER INSERT OR UPDATE OR DELETE` row trigger as ordinary schema objects, so `diff`, `Baseline`, `drift` and `generate --from-db` handle them and a column change or rename of the model reaches the history table in the same plan, and a dropped column stays there as nullable; the actor is the transaction-local setting `momo.actor`, set by `momo.SetActor`; `momo.HistoryOf` reads the versions of a row and `momo.HistoryAsOf` its state at a time",
		SQLite: "`ErrUnsupported`: a declaration is a generator error, and `momo.SetActor`, `momo.HistoryOf` and `momo.HistoryAsOf` return `ErrUnsupported`, because SQLite has no transaction-local setting to carry the actor",
		MySQL:  "`ErrUnsupported`: a declaration is a generator error, and `momo.SetActor`, `momo.HistoryOf` and `momo.HistoryAsOf` return `ErrUnsupported`, because MySQL and MariaDB have no transaction-local setting to carry the actor",
	},
	{
		Name:   "Transactional outbox",
		API:    []string{"outbox.New", "outbox.Outbox", "outbox.Relay", "outbox.Publisher", "outbox.WithListen"},
		PG:     "yes: `outbox.Outbox.Add` writes the event in the caller's transaction and sends `NOTIFY`; `outbox.Relay` claims batches with `FOR UPDATE SKIP LOCKED` and a lease, publishes with no lock held, retries with backoff up to a maximum of attempts, keeps the order of each key, and wakes on `NOTIFY` through `outbox.WithListen` with polling as the fallback; several relays in several processes share one table",
		SQLite: "yes: the same table, `outbox.Outbox.Add` and `outbox.Relay`, polling only; the single writer serializes the claims, and `outbox.WithListen` is `ErrUnsupported`",
		MySQL:  "yes on MySQL 8.4, MySQL 9.7 and MariaDB 11.8: the claim is one short `READ COMMITTED` transaction that selects with `FOR UPDATE SKIP LOCKED`, leases the rows and commits before anything is published; polling only, `outbox.WithListen` is `ErrUnsupported`; a server before MySQL 8.0 or MariaDB 10.6 makes `outbox.Relay.Run` return `ErrUnsupported`",
	},
	{
		Name:   "Embeddings kept in sync with their source columns (`embed_from`)",
		API:    []string{"vector.Embedder", "vector.SourceHash"},
		PG:     "yes: the generator adds a `varchar(64)` hash column beside the `*vector.Vector` field, `momo.StaleEmbeddings` finds the rows whose source hash or NULL vector says they are out of date, and `momo.SyncEmbeddings` embeds them in batches with a compare-and-set write, no transaction held during the `vector.Embedder` call",
		SQLite: "yes: the same hash column and the same two functions on a sqlite-vec vector column; concurrent runners are limited by SQLite's single writer",
		MySQL:  "`ErrUnsupported`: `embed_from` is a generator error and `momo.SyncEmbeddings` and `momo.StaleEmbeddings` refuse the dialect",
	},
	{
		Name:   "Range types on `types.Range[T]`",
		API:    []string{"types.Range", "types.Bound", "types.Timestamp", "rangeop.Contains", "rangeop.ContainsRange", "rangeop.ContainedBy", "rangeop.Overlaps", "rangeop.Adjacent", "rangeop.StrictlyLeft", "rangeop.StrictlyRight", "rangeop.Lower", "rangeop.Upper", "rangeop.IsEmpty"},
		PG:     "yes: `int4range`, `int8range`, `numrange`, `daterange`, `tsrange` and `tstzrange` are typed columns with empty, infinite, inclusive and exclusive bounds, written by `generate --from-db`, and queried with `rangeop` predicates for containment, overlap, adjacency, position, the bounds and emptiness; multiranges are not mapped",
		SQLite: "`ErrUnsupported`: a `types.Range` field is a generator error, because SQLite has no range type, and each `rangeop` builder fails the build",
		MySQL:  "`ErrUnsupported`: a `types.Range` field is a generator error, because MySQL and MariaDB have no range type, and each `rangeop` builder fails the build",
	},
	{
		Name:   "Exclusion constraints",
		API:    []string{"schema.Exclude", "schema.ExcludeElement", "schema.Table"},
		PG:     "yes: `EXCLUDE USING gist (room WITH =, during WITH &&)` over plain columns is declared with `Config.Exclude`, read from `pg_constraint`, written by `generate --from-db`, rendered, compared by `diff`, `Baseline` and `drift`, and kept by a snapshot; a GiST constraint over a scalar column adds the `btree_gist` extension; a constraint with a `WHERE` predicate, an expression or an operator class is refused by the inspector",
		SQLite: "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because SQLite has no exclusion constraints",
		MySQL:  "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because MySQL and MariaDB have no exclusion constraints",
	},
	{
		Name:   "Domains",
		API:    []string{"schema.Domain", "schema.DomainCheck", "schema.Schema"},
		PG:     "yes: `CREATE DOMAIN` with a base type, `NOT NULL`, a default and named `CHECK` constraints is declared with `Config.Domain`, read from `pg_type` and `pg_constraint`, written by `generate --from-db`, rendered, compared by `diff`, `Baseline` and `drift`, and kept by a snapshot; a column of a domain takes the Go type of the base type and names the domain in its `type` tag; a change of the default, `NOT NULL` or a constraint is an `ALTER DOMAIN`, and a change of the base type recreates the domain and rewrites the columns that use it, which `diff` treats as destructive",
		SQLite: "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because SQLite has no domains",
		MySQL:  "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because MySQL and MariaDB have no domains",
	},
	{
		Name:   "Composite types on `types.Composite[T]`",
		API:    []string{"types.Composite", "schema.Composite", "schema.CompositeAttr", "schema.Schema"},
		PG:     "yes: `CREATE TYPE ... AS (...)` is declared with `Config.Composite`, read from `pg_type` and `pg_attribute`, written by `generate --from-db` together with its Go struct, rendered, compared by `diff` (add, drop and retype an attribute with `ALTER TYPE`), `Baseline` and `drift`, and kept by a snapshot; a column is a `types.Composite` of the struct and names the type in its `type` tag, with the record text of PostgreSQL read and written through `Value` and `Scan`; a nested composite type and an array of composite types are `ErrUnsupported`",
		SQLite: "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because SQLite has no composite types",
		MySQL:  "`ErrUnsupported`: a declaration is a generator error and the renderer refuses it, because MySQL and MariaDB have no composite types",
	},
}

// featuresBlock is the feature support table built from the features data and
// the driver scan.
func featuresBlock(root string) (string, error) {
	drivers, err := scanDrivers(root)
	if err != nil {
		return "", err
	}
	if err := checkFeatureAPI(root); err != nil {
		return "", err
	}
	cell := func(n dialect.Name, text string) string {
		if len(driversFor(drivers, n)) == 0 {
			return "not usable (no driver package)"
		}
		return text
	}
	var rows [][]string
	for _, f := range features {
		rows = append(rows, []string{
			f.Name, codes(f.API),
			cell(dialect.Postgres, f.PG), cell(dialect.SQLite, f.SQLite), cell(dialect.MySQL, f.MySQL),
		})
	}
	return table([]string{"Feature", "API", "PostgreSQL", "SQLite", "MySQL / MariaDB"}, rows), nil
}

// typeRow is one Go type of the type table: the label shown, the type string
// and tags handed to typemap.Map.
type typeRow struct {
	Label string
	Go    string
	Tags  tags.Tags
}

// sentinel stands in for the size and dim tags, so a mapped type with a length
// can be shown with N. It is a small value that is valid for every length and
// dimension, so a future range check in typemap.Map cannot reject it.
const sentinel = 7

var typeRows = []typeRow{
	{"int", "int", tags.Tags{}},
	{"int8", "int8", tags.Tags{}},
	{"int16", "int16", tags.Tags{}},
	{"int32", "int32", tags.Tags{}},
	{"int64", "int64", tags.Tags{}},
	{"float32", "float32", tags.Tags{}},
	{"float64", "float64", tags.Tags{}},
	{"bool", "bool", tags.Tags{}},
	{"string", "string", tags.Tags{}},
	{"string with size:N", "string", tags.Tags{Size: sentinel}},
	{"[]byte", "[]byte", tags.Tags{}},
	{"time.Time", "time.Time", tags.Tags{}},
	{"types.UUID", "types.UUID", tags.Tags{}},
	{"uuid.UUID (github.com/google/uuid)", "uuid.UUID", tags.Tags{}},
	{"types.Date", "types.Date", tags.Tags{}},
	{"types.Numeric", "types.Numeric", tags.Tags{}},
	{"types.JSON[T]", "types.JSON[T]", tags.Tags{}},
	{"types.Range[int32]", "types.Range[int32]", tags.Tags{}},
	{"types.Range[int64]", "types.Range[int64]", tags.Tags{}},
	{"types.Range[types.Numeric]", "types.Range[types.Numeric]", tags.Tags{}},
	{"types.Range[types.Date]", "types.Range[types.Date]", tags.Tags{}},
	{"types.Range[types.Timestamp]", "types.Range[types.Timestamp]", tags.Tags{}},
	{"types.Range[time.Time]", "types.Range[time.Time]", tags.Tags{}},
	{"vector.Vector", "vector.Vector", tags.Tags{Dim: sentinel}},
	{"vector.HalfVector", "vector.HalfVector", tags.Tags{Dim: sentinel}},
	{"vector.SparseVector", "vector.SparseVector", tags.Tags{Dim: sentinel}},
	{"vector.Bits", "vector.Bits", tags.Tags{Dim: sentinel}},
	{"vector.Vector (no dim)", "vector.Vector", tags.Tags{}},
	{"vector.HalfVector (no dim)", "vector.HalfVector", tags.Tags{}},
	{"[]string", "[]string", tags.Tags{}},
	{"[]int16", "[]int16", tags.Tags{}},
	{"[]int32", "[]int32", tags.Tags{}},
	{"[]int64", "[]int64", tags.Tags{}},
	{"[]bool", "[]bool", tags.Tags{}},
	{"[]float32", "[]float32", tags.Tags{}},
	{"[]float64", "[]float64", tags.Tags{}},
	{"[]types.UUID", "[]types.UUID", tags.Tags{}},
	{"[]uuid.UUID (github.com/google/uuid)", "[]uuid.UUID", tags.Tags{}},
}

// typemapBlock is the Go-to-column-type table, one column per dialect. A
// dialect typemap.Map has no mapping for shows unsupported.
func typemapBlock() (string, error) {
	dialects := []struct {
		Title string
		Name  dialect.Name
	}{
		{"PostgreSQL", dialect.Postgres},
		{"SQLite", dialect.SQLite},
		{"MySQL", dialect.MySQL},
	}
	header := []string{"Go type"}
	for _, d := range dialects {
		header = append(header, d.Title)
	}
	var rows [][]string
	for _, tr := range typeRows {
		r := []string{code(tr.Label)}
		for _, d := range dialects {
			sql, err := typemap.Map(tr.Go, d.Name, tr.Tags)
			switch {
			case errors.Is(err, momo.ErrUnsupported):
				sql = "unsupported"
			case err != nil:
				return "", fmt.Errorf("typemap.Map(%q, %s): %w", tr.Go, d.Name, err)
			}
			r = append(r, strings.ReplaceAll(sql, fmt.Sprintf("(%d)", sentinel), "(N)"))
		}
		rows = append(rows, r)
	}
	return table(header, rows), nil
}
