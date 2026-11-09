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
	"github.com/azrtydxb/momo/gen/typemap"
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
		rows = append(rows, row(code(sf.Name), func(f family) string {
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
		Name: "JSON operators on `types.JSON[T]`",
		API:  []string{"jsonop.Text", "jsonop.Path", "jsonop.Contains", "jsonop.HasKey"},
		PG:   "yes",
		SQLite: "`jsonop.Text`, `jsonop.Path` and `jsonop.HasKey` run on both SQLite drivers; a JSON boolean extracts as `1` or `0`; " +
			"`jsonop.Contains` is `ErrUnsupported`, since `json_each` cannot express the recursive `@>` rule",
		MySQL: "SQL is golden-tested only; no integration test runs it against a MySQL or MariaDB server",
	},
	{
		Name: "Date and time expressions",
		API: []string{
			"timeop.Extract", "timeop.AtTimeZone", "timeop.Trunc", "timeop.Now", "timeop.CurrentTimestamp",
			"timeop.IntervalOf", "timeop.IntervalExpr", "timeop.AddInterval", "timeop.SubInterval",
		},
		PG: "yes",
		SQLite: "every `ExtractField`, `TruncUnit`, interval and `Now` run on both SQLite drivers; month arithmetic overflows into the next month " +
			"(Jan 31 plus a month is Mar 2 or 3, where PostgreSQL and MySQL clamp); `Now` is millisecond text; `timeop.AtTimeZone` is PostgreSQL only",
		MySQL: "SQL is golden-tested only; no integration test runs it against a MySQL or MariaDB server; `timeop.AtTimeZone` is PostgreSQL only",
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
	{"types.Date", "types.Date", tags.Tags{}},
	{"types.JSON[T]", "types.JSON[T]", tags.Tags{}},
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
