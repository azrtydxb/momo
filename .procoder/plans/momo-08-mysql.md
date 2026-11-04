# momo-08-mysql — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

Milestone: M3 (MySQL and MariaDB), after M1 and M2.

## Goal

Add a MySQL and MariaDB dialect and adapter with typed queries, writes, locking, transactions, structured error classification, a migration engine that treats DDL as non-transactional with statement-level recovery, and vector support where the server provides it, verified against pinned official server images.

## Architecture

The `dialect` package gains a `MySQL` name, `Flavor` values (`FlavorMySQL`, `FlavorMariaDB`) and a MySQL capability set that differs by flavor: backtick identifiers, `?` placeholders, `ON DUPLICATE KEY UPDATE` upserts, `LAST_INSERT_ID` generated keys, and `RETURNING` for inserts on MariaDB only. The `driver/mysqldriver` adapter wraps `database/sql` on `github.com/go-sql-driver/mysql` and classifies errors by MySQL error number. The `migrate/mysql` package (behind the `migrate.Dialect` interface from momo-04) inspects `information_schema`, renders DDL, serializes runs with `GET_LOCK` on the pinned connection, and records progress per statement because implicit commits make every MySQL migration non-transactional. Vector support is flavor-specific: MariaDB 11.8 LTS has `VECTOR(N)`, `VEC_DISTANCE_*` and `VECTOR INDEX`; MySQL has `VECTOR` storage from 9.0 onward but its distance function is available only on HeatWave and MySQL AI, so distance queries on MySQL return `momo.ErrUnsupported`.

## Constraints

- MySQL/MariaDB: github.com/go-sql-driver/mysql (pinned); supported servers MySQL 8.4 LTS, MySQL 9.7 LTS and MariaDB 11.8 LTS (no innovation releases). Vector capability: MariaDB 11.8 full search with VECTOR INDEX; MySQL 9.7 VECTOR storage only, distance queries return ErrUnsupported (DISTANCE() is HeatWave/MySQL AI only); MySQL 8.4 has no vector columns; CI uses official MySQL and MariaDB service containers.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners.
- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state.
- Documented Go-to-SQL type mapping for both dialects, with timestamp vs timestamptz and SQLite time storage (spec S-31).
- Failure modes (spec): Migration fails mid-way (non-transactional): history row stays `failed` with statements_applied; status shows dirty; apply refuses until `migrate resolve` after manual repair (documented procedure). Lock held by another process: apply waits up to a timeout then fails with a lock error naming the holder; stale SQLite lock cleared only by explicit `--force-unlock`. Extension missing (pgvector / sqlite-vec): clear unsupported-capability error at migration or first vector query, not a silent fallback. Unsupported dialect feature requested: ErrUnsupported at build time, never silently dropped.
- Spec edge list: Concurrent `migrate apply` from two processes.
- Test packages: unit tests that call unexported helpers stay in the package under test. A test that opens a `*momo.DB` through `testopen` or through a driver uses an external package (`momo_test`, `sqlitedriver_test`, `mysqldriver_test`, `pgxdriver_test`), because `testopen` imports the drivers and the drivers import `momo`.

## Version record (verified 2026-11-04)

| Item                            | Verified value                                                                                                                                                                  | Source                                                    |
| ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| MySQL LTS                       | 8.4.13 (2026-10-20), EOL 2032 (third-party summary; Oracle page not confirmed)                                                                                                  | dev.mysql.com release notes 8.4                           |
| MySQL 9.7 LTS (supported)       | 9.7.4 (2026-10-20), latest patch on Docker Hub (`mysql:9.7.4`, verified 2026-11-04)                                                                                             | dev.mysql.com release notes 9.7; Oracle EOL notice        |
| MySQL innovation releases       | not supported (decision 2026-11-04: supported set is 8.4 LTS, 9.7 LTS and MariaDB 11.8 LTS)                                                                                     | user decision                                             |
| MySQL 8.4 VECTOR type           | not present (8.4 manual data types chapter has no VECTOR)                                                                                                                       | dev.mysql.com/doc/refman/8.4                              |
| MySQL VECTOR type               | added in 9.0.0: `VECTOR(N)`, default 2048, maximum 16383, float32 elements                                                                                                      | dev.mysql.com news-9-0-0                                  |
| MySQL DISTANCE()                | `DISTANCE(v1, v2, COSINE or DOT or EUCLIDEAN)` (synonym `VECTOR_DISTANCE`) documented as available only on HeatWave on OCI and MySQL AI, not Community                          | dev.mysql.com/doc/refman/9.7 vector-functions             |
| MySQL vector index              | none found in Community documentation (UNVERIFIED for every release)                                                                                                            | dev.mysql.com/doc/refman/9.7 vector                       |
| MariaDB LTS                     | 11.8.9 (2026-10-27), community EOL 4 June 2028                                                                                                                                  | mariadb.org maintenance policy; Docker official images    |
| MariaDB VECTOR                  | introduced 11.7 (rolling), GA in 11.8 LTS; `VECTOR(n)` float32, maximum 16383                                                                                                   | mariadb.com/docs vectors                                  |
| MariaDB distance functions      | `VEC_DISTANCE_EUCLIDEAN`, `VEC_DISTANCE_COSINE`, generic `VEC_DISTANCE` (follows the index metric); `VEC_FromText`; no dot-product function                                     | mariadb.com/docs vectors                                  |
| MariaDB VECTOR INDEX            | `VECTOR INDEX (col) M=8 DISTANCE=cosine or euclidean`, M 3..200, modified HNSW (approximate); used only for `ORDER BY VEC_DISTANCE_*(col, v) LIMIT k` with a bare distance call | mariadb.com/docs vectors                                  |
| MariaDB INSERT RETURNING        | 10.5.0                                                                                                                                                                          | mariadb.com/kb insertreturning                            |
| MariaDB UPDATE/DELETE RETURNING | not in 11.8 (UPDATE RETURNING arrives in 13.0). DELETE RETURNING from 10.0.5 is UNVERIFIED                                                                                      | mariadb.com release notes 13.0.2                          |
| go-sql-driver/mysql             | v1.10.1 (2026-09-02), requires Go 1.25 or newer, supports MySQL 8.0+ and MariaDB 10.11+                                                                                         | proxy.golang.org; github.com/go-sql-driver/mysql          |
| Docker tags                     | `mysql:8.4.13`, `mysql:9.7.4`, `mariadb:11.8.9` (verified on Docker Hub 2026-11-04)                                                                                             | hub.docker.com official images                            |
| MariaDB locking                 | `GET_LOCK` with a negative timeout returns NULL (no wait forever); `SKIP LOCKED` from 10.6 (InnoDB); `NOWAIT` and `WAIT n` from 10.3.0                                          | mariadb.com/kb get_lock, wait-and-nowait                  |
| MySQL locking                   | `NOWAIT` and `SKIP LOCKED` on `FOR UPDATE` and `FOR SHARE` in 8.4; `GET_LOCK` waits forever with a negative timeout                                                             | dev.mysql.com/doc/refman/8.4 locking functions            |
| Online DDL (MySQL 8.4)          | `ALGORITHM` DEFAULT/INSTANT/INPLACE/COPY; `LOCK` DEFAULT/NONE/SHARED/EXCLUSIVE; ADD/DROP/RENAME COLUMN, SET/DROP DEFAULT, rename table are INSTANT; data-type change needs COPY | dev.mysql.com/doc/refman/8.4 innodb-online-ddl-operations |

Consequences recorded in this plan: MySQL 8.4 has no vector columns, so declaring one fails at `migrate diff` with a dialect diagnostic wrapping `momo.ErrUnsupported`, and vector queries return `ErrUnsupported`; MySQL 9.7 stores VECTOR and rejects distance queries with `ErrUnsupported` (HeatWave/MySQL AI `DISTANCE()` is neither supported nor detected); MariaDB 11.8 is the only flavor with executable vector search, including VECTOR INDEX.

## Task 1: MySQL and MariaDB dialect flavors

Covers: S-59 (dialect with flavors mysql/mariadb, backtick quoting), S-31 (type mapping inputs). Acceptance tests named by later tasks: `TestMySQLIntegration` (Task 8).

Files:

- Modify `dialect/dialect.go`: add `MySQL` to the `Name` constants, `Flavor` type, `FlavorMySQL`, `FlavorMariaDB`, the `Flavor` field and the new capability fields; add `MySQLFlavor`; branch `AppendPlaceholder` and `AppendIdent` on `MySQL`.
- Create `dialect/mysql.go`: MySQL flavor literals.
- Create `dialect/mysql_test.go`: `TestMySQLQuoting`, `TestMySQLFlavorCapabilities`.

Interfaces (exact):

```go
// dialect/dialect.go
const MySQL Name = "mysql"

type Flavor string

const (
	FlavorMySQL   Flavor = "mysql"
	FlavorMariaDB Flavor = "mariadb"
)

// Dialect gains these fields (added to the struct in the spike):
//   Flavor            Flavor // set on MySQL dialects only
//   OnDuplicateKey    bool    // INSERT ... ON DUPLICATE KEY UPDATE
//   InsertReturning   bool    // INSERT ... RETURNING (MariaDB >= 10.5); Returning stays false on MySQL
//   WindowFunctions   bool
//   Merge             bool
//   ILike             bool    // false on MySQL: LIKE with LOWER() is rendered instead, see plan momo-05
//   Vector            VectorCaps  (from plan momo-07, dialect/vector.go)

func For(n Name) *Dialect            // For(MySQL) returns the FlavorMySQL dialect
func MySQLFlavor(f Flavor) *Dialect  // unknown flavor returns nil
```

Capability literal for both flavors (`dialect/mysql.go`):

```go
var mysqlBase = Dialect{
	Name: MySQL, MaxParams: 65535,
	OnDuplicateKey: true, Locking: true, DefaultInUpdate: true, DefaultInValues: true,
	RecursiveCTE: true, RowValues: true, Savepoints: true, IsolationLevels: true,
	WindowFunctions: true,
	// Returning false, OnConflict false, NullsOrdering false (emulated with an IS NULL sort key), ILike false,
	// TransactionalDDL false, Merge false.
}
// MySQL flavor: Vector = VectorCaps{Storage: true, Metrics: nil, Indexes: nil}
// MariaDB flavor: InsertReturning = true; Vector = VectorCaps{Storage: true,
//   Metrics: []Metric{MetricL2, MetricCosine}, Indexes: []string{"vector"}}
```

`Vector.Storage` is true on both flavors; the server-version gate in `migrate/mysql` rejects VECTOR on MySQL 8.4 at `migrate diff` time (Task 7).

- [ ] Step 1. Write `dialect/mysql_test.go`:

```go
package dialect

import "testing"

func TestMySQLQuoting(t *testing.T) {
	d := MySQLFlavor(FlavorMySQL)
	if got := d.QuoteIdent("order"); got != "`order`" {
		t.Fatalf("QuoteIdent(order) = %s, want `order`", got)
	}
	if got := d.QuoteIdent("a`b"); got != "`a``b`" {
		t.Fatalf("embedded backtick = %s, want `a``b`", got)
	}
	if got := string(d.AppendPlaceholder(nil, 3)); got != "?" {
		t.Fatalf("placeholder = %s, want ?", got)
	}
}

func TestMySQLFlavorCapabilities(t *testing.T) {
	my := MySQLFlavor(FlavorMySQL)
	ma := MySQLFlavor(FlavorMariaDB)
	if my.Returning || my.InsertReturning {
		t.Fatal("MySQL must not advertise RETURNING")
	}
	if ma.Returning || !ma.InsertReturning {
		t.Fatal("MariaDB: InsertReturning true, Returning (update/delete) false in 11.8")
	}
	if len(my.Vector.Metrics) != 0 {
		t.Fatalf("MySQL Community has no distance functions, got %v", my.Vector.Metrics)
	}
	if len(ma.Vector.Metrics) != 2 || len(ma.Vector.Indexes) != 1 {
		t.Fatalf("MariaDB vector caps = %+v", ma.Vector)
	}
	if !my.OnDuplicateKey || !ma.OnDuplicateKey {
		t.Fatal("both flavors must support ON DUPLICATE KEY UPDATE")
	}
}
```

Expect FAIL with `undefined: MySQLFlavor`.

- [ ] Step 2. Implement `dialect/mysql.go` and the `dialect.go` changes. `AppendIdent` for MySQL writes a backtick and doubles embedded backticks; the `postgres` and `sqlite` cases keep double quotes. Run `go test ./dialect -count=1`; expect `ok`.
- [ ] Step 3. Commit: `feat(dialect): add MySQL and MariaDB flavors`.

## Task 2: go-sql-driver adapter, error classification and redaction

Covers: S-59 (adapter via github.com/go-sql-driver/mysql, error classification), S-8 (structured errors, both dialects: this task adds the MySQL half), S-41 (error redaction for MySQL messages). Acceptance tests: `TestErrorClassification` (MySQL cases, this task), `TestErrorRedaction` (MySQL case, this task), `TestEmptyResultNotNotFound` (MySQL case, this task).

Files:

- Modify `go.mod`: `go get github.com/go-sql-driver/mysql@v1.10.1` (requires Go 1.25 or newer; module minimum stays 1.26).
- Create `driver/mysqldriver/driver.go`: `New`, driver and transaction types.
- Create `driver/mysqldriver/errors.go`: classifier.
- Create `driver/mysqldriver/errors_test.go`: `TestMySQLErrorClassification` (unit, mapping table), `TestErrorRedaction` (integration).
- Create `driver/mysqldriver/driver_integration_test.go` (`//go:build integration`): `TestErrorClassification`, `TestEmptyResultNotNotFound`.

Interfaces (exact):

```go
// driver/mysqldriver (package mysqldriver)
// New wraps an opened *sql.DB. The DSN must set parseTime=true and loc=UTC;
// multiStatements stays false. The adapter never opens connections itself.
func New(db *sql.DB, flavor dialect.Flavor) momo.Driver

// Classifier maps MySQL server error numbers to momo sentinels.
// Implements momo.ErrorClassifier.
type classifier struct{}
func (classifier) ClassifyError(err error) error
```

Error mapping (MySQL and MariaDB use the same numbers; verified against the MySQL server error reference, re-checked by the integration test that provokes each one):

| Number | Name                         | momo kind              |
| ------ | ---------------------------- | ---------------------- |
| 1062   | ER_DUP_ENTRY                 | ErrUniqueViolation     |
| 1451   | ER_ROW_IS_REFERENCED_2       | ErrForeignKeyViolation |
| 1452   | ER_NO_REFERENCED_ROW_2       | ErrForeignKeyViolation |
| 1048   | ER_BAD_NULL_ERROR            | ErrNotNullViolation    |
| 3819   | ER_CHECK_CONSTRAINT_VIOLATED | ErrCheckViolation      |
| 1213   | ER_LOCK_DEADLOCK             | ErrSerialization       |
| 1205   | ER_LOCK_WAIT_TIMEOUT         | ErrBusy                |

`Constraint` is parsed from the server message: for 1062 the text after `for key '` (for example `users.email`); for 1451 and 1452 the backtick-quoted name after `CONSTRAINT `. The `Err` field keeps the `*mysql.MySQLError`; `Error()` omits the message text (contract: `Detail()` returns it), so duplicate values never appear in `Error()`.

- [ ] Step 1. Write `driver/mysqldriver/errors_test.go` with the mapping table:

```go
package mysqldriver

import (
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/azrtydxb/momo"
)

func TestMySQLErrorClassification(t *testing.T) {
	cases := []struct {
		num  uint16
		msg  string
		kind error
		name string
	}{
		{1062, "Duplicate entry 'a@example.com' for key 'users.email'", momo.ErrUniqueViolation, "users.email"},
		{1452, "Cannot add or update a child row: a foreign key constraint fails (`app`.`orders`, CONSTRAINT `fk_orders_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`))", momo.ErrForeignKeyViolation, "fk_orders_user"},
		{1048, "Column 'name' cannot be null", momo.ErrNotNullViolation, ""},
		{3819, "Check constraint 'positive' is violated.", momo.ErrCheckViolation, "positive"},
		{1213, "Deadlock found when trying to get lock", momo.ErrSerialization, ""},
		{1205, "Lock wait timeout exceeded", momo.ErrBusy, ""},
	}
	for _, c := range cases {
		err := classifier{}.ClassifyError(&mysql.MySQLError{Number: c.num, Message: c.msg})
		if !errors.Is(err, c.kind) {
			t.Errorf("%d: errors.Is(%v) false", c.num, err)
		}
		var me *mysql.MySQLError
		if !errors.As(err, &me) || me.Number != c.num {
			t.Errorf("%d: driver error not reachable with errors.As", c.num)
		}
		var me2 *momo.Error
		if !errors.As(err, &me2) {
			t.Errorf("%d: not a *momo.Error", c.num)
		}
		if c.name != "" && me2.Constraint != c.name {
			t.Errorf("%d: constraint = %q, want %q", c.num, me2.Constraint, c.name)
		}
		if err != nil && containsValue(err.Error(), "a@example.com") {
			t.Errorf("%d: Error() leaked the offending value", c.num)
		}
	}
	if got := (classifier{}).ClassifyError(errors.New("boom")); got.Error() != "boom" {
		t.Errorf("unknown errors must pass through unchanged, got %v", got)
	}
}
```

`containsValue` is a one-line `strings.Contains` helper in the same file. Expect FAIL with `undefined: classifier`.

- [ ] Step 2. Implement `errors.go` (a `map[uint16]error` with the table above, a regular expression `for key '([^']+)'` and the backtick-quoted name after the word CONSTRAINT) and `driver.go` (`New` returns a `momo.Driver` whose `Begin` maps `TxOptions.Isolation` to `sql.IsolationLevel` through `db.BeginTx` and passes `ReadOnly`; `Query`/`Exec` call `db.QueryContext`/`ExecContext` and return the classified error). Run `go test ./driver/mysqldriver -run TestMySQLErrorClassification -count=1`; expect `ok`.
- [ ] Step 3. Write `driver/mysqldriver/driver_integration_test.go` (`//go:build integration`, package `mysqldriver_test` because it opens a `*momo.DB` through `testopen`). It provokes each mapped server error and checks the sentinel, checks the empty-result rule, and checks that a duplicate value never reaches `Error()`:

```go
//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// mustExecMySQL fails the test on any error from momo.Exec. The same helper is used by the other files in this package.
func mustExecMySQL(t *testing.T, q momo.Querier, sql string, args ...any) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestErrorClassification(t *testing.T) {
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE users (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(255) UNIQUE)")
	mustExecMySQL(t, db, "CREATE TABLE orders (id bigint PRIMARY KEY AUTO_INCREMENT, user_id bigint NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id))")
	mustExecMySQL(t, db, "INSERT INTO users (email) VALUES ('a@example.com')")
	mustExecMySQL(t, db, "INSERT INTO orders (user_id) VALUES (1)")
	cases := []struct {
		name string
		sql  string
		kind error
	}{
		{"unique (1062)", "INSERT INTO users (email) VALUES ('a@example.com')", momo.ErrUniqueViolation},
		{"foreign key insert (1452)", "INSERT INTO orders (user_id) VALUES (999)", momo.ErrForeignKeyViolation},
		{"foreign key delete (1451)", "DELETE FROM users WHERE id = 1", momo.ErrForeignKeyViolation},
		{"not null (1048)", "INSERT INTO orders (user_id) VALUES (NULL)", momo.ErrNotNullViolation},
	}
	for _, c := range cases {
		_, err := momo.Exec(context.Background(), db, c.sql)
		if !errors.Is(err, c.kind) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.kind)
		}
	}
}

type emptyRow struct{ ID int64 }

func (r *emptyRow) MomoMeta() *momo.Meta[emptyRow] {
	return &momo.Meta[emptyRow]{
		Table:   "empties",
		PK:      []int{0},
		Columns: []momo.ColumnMeta{{Name: "id"}},
		Ptr:     func(r *emptyRow, i int) any { return &r.ID },
		Value:   func(r *emptyRow, i int) any { return r.ID },
	}
}

func TestEmptyResultNotNotFound(t *testing.T) {
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE empties (id bigint PRIMARY KEY)")
	rows, err := momo.Select[emptyRow, *emptyRow](db).All(context.Background())
	if err != nil {
		t.Fatalf("All on zero rows: err = %v, want nil", err)
	}
	if len(rows) != 0 {
		t.Fatalf("All on zero rows returned %d rows", len(rows))
	}
}

func TestErrorRedaction(t *testing.T) {
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE accounts (id bigint PRIMARY KEY AUTO_INCREMENT, email varchar(255) UNIQUE)")
	mustExecMySQL(t, db, "INSERT INTO accounts (email) VALUES ('secret@example.com')")
	_, err := momo.Exec(context.Background(), db, "INSERT INTO accounts (email) VALUES ('secret@example.com')")
	var me *momo.Error
	if !errors.As(err, &me) {
		t.Fatalf("err = %v, want a *momo.Error", err)
	}
	if strings.Contains(err.Error(), "secret@example.com") {
		t.Fatal("Error() leaked the offending value")
	}
	if !strings.Contains(me.Detail(), "secret@example.com") {
		t.Fatal("Detail() lost the driver message")
	}
}
```

Expect FAIL with `undefined: testopen.MySQL84` before Task 8 Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 go test -tags integration ./driver/mysqldriver -run 'TestErrorClassification|TestEmptyResultNotNotFound|TestErrorRedaction' -count=1`; expect `ok`.

- [ ] Step 4. Commit: `feat(mysqldriver): add go-sql-driver adapter with error classification`.

## Task 3: Writes on MySQL: generated keys, upsert, conflict targets

Covers: S-59 (LAST_INSERT_ID where RETURNING is unavailable, RETURNING on MariaDB where supported, ON DUPLICATE KEY UPDATE), S-5 (write semantics on MySQL), S-84 (FindOrCreate and Upsert rely on this). Explicit rejections are asserted: `TestMySQLGeneratedKeys` (a multi-row insert that needs generated keys returns `ErrUnsupported`) and `TestMySQLConflictTargetRule` (an `OnConflict` target that is not the sole unique key returns `ErrUnsupported`). Acceptance: `TestWriteStates` (MySQL case), `TestMySQLIntegration` (Task 8).

Files:

- Create `mysql_render.go` (root package `momo`): `renderMySQLUpsert`, `checkMySQLConflictTarget`, `mysqlReturning`.
- Create `mysql_render_test.go`: `TestMySQLUpsertRendering`, `TestMySQLConflictTargetRule`.
- Modify the plan momo-05 insert renderer at the single branch point where it renders `ON CONFLICT`: when `b.d.Name == dialect.MySQL` it calls `renderMySQLUpsert`. This plan does not change momo-05's other code paths.
- Create `driver/mysqldriver/generated_keys.go`: the driver-side generated key path for `Model(&m)` inserts.

Interfaces (exact):

```go
// mysql_render.go (package momo)

// renderMySQLUpsert renders INSERT ... VALUES (...) AS momo_new ON DUPLICATE KEY UPDATE ...
// for MySQL flavors with server support for row aliases (8.0.19+, including MySQL 9.7 LTS);
// it renders VALUES(col) on MariaDB, where the alias form is not verified (see the version record).
// DoNothing renders "pk = pk" assignments on the first primary key column.
func renderMySQLUpsert(b *Builder, flavor dialect.Flavor, table string, cols []string, target []string, sets []Assignment, doNothing bool)

// checkMySQLConflictTarget returns ErrUnsupported unless target equals the primary key or the
// only unique key of ts, because MySQL's ON DUPLICATE KEY UPDATE fires for any unique key.
func checkMySQLConflictTarget(ts *schema.Table, target []string) error

// mysqlReturning reports whether an INSERT may use RETURNING on this flavor.
func mysqlReturning(d *dialect.Dialect) bool { return d.InsertReturning }
```

Generated-key rule: a single-row insert on MySQL and MariaDB without RETURNING reads `Result.LastInsertId()` and assigns it to the auto-increment primary key. Multi-row inserts that need generated keys return `ErrUnsupported` on MySQL and MariaDB (contract: ids for multi-row statements depend on `innodb_autoinc_lock_mode` and are not contiguous under the default lock mode). Multi-row inserts that do not need generated keys are allowed.

- [ ] Step 1. Write `mysql_render_test.go`:

```go
package momo

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func TestMySQLUpsertRendering(t *testing.T) {
	b := NewBuilder(dialect.MySQLFlavor(dialect.FlavorMySQL))
	renderMySQLUpsert(b, dialect.FlavorMySQL, "users", []string{"id", "email"}, []string{"email"},
		[]Assignment{assignForTest("email")}, false)
	want := "INSERT INTO `users` (`id`, `email`) VALUES (?, ?) AS `momo_new` ON DUPLICATE KEY UPDATE `email` = `momo_new`.`email`"
	if b.String() != want {
		t.Fatalf("MySQL upsert:\n got %s\nwant %s", b.String(), want)
	}
	m := NewBuilder(dialect.MySQLFlavor(dialect.FlavorMariaDB))
	renderMySQLUpsert(m, dialect.FlavorMariaDB, "users", []string{"id", "email"}, []string{"email"},
		[]Assignment{assignForTest("email")}, false)
	if want := "ON DUPLICATE KEY UPDATE `email` = VALUES(`email`)"; !contains(m.String(), want) {
		t.Fatalf("MariaDB upsert = %s, want suffix %s", m.String(), want)
	}
}

func TestMySQLConflictTargetRule(t *testing.T) {
	ts := &schema.Table{Name: "users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "users_email", Columns: []string{"email"}}}}
	if err := checkMySQLConflictTarget(ts, []string{"email"}); err != nil {
		t.Fatalf("sole unique target rejected: %v", err)
	}
	ts.Uniques = append(ts.Uniques, schema.Unique{Name: "users_handle", Columns: []string{"handle"}})
	if err := checkMySQLConflictTarget(ts, []string{"email"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("second unique key: err = %v, want ErrUnsupported", err)
	}
}
```

`assignForTest` builds an `Assignment` for a column named by its argument with value `"x"`; `contains` is `strings.Contains`. Expect FAIL with `undefined: renderMySQLUpsert`.

- [ ] Step 2. Implement `mysql_render.go`. For `doNothing`, render `ON DUPLICATE KEY UPDATE <pk> = <pk>` (quoted through the builder). `checkMySQLConflictTarget` compares sorted column sets. Run `go test . -run 'TestMySQLUpsertRendering|TestMySQLConflictTargetRule' -count=1`; expect `ok`.
- [ ] Step 3. Write the integration test `TestMySQLGeneratedKeys` (`//go:build integration`, package `mysqldriver_test`, file `generated_keys_integration_test.go`). It checks the single-row generated key, the multi-row rejection, and multi-row inserts with explicit keys:

```go
//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

type keyRow struct {
	ID   int64
	Name string
}

func (r *keyRow) MomoMeta() *momo.Meta[keyRow] {
	return &momo.Meta[keyRow]{
		Table:    "keys_t",
		PK:       []int{0},
		Identity: 0,
		Columns:  []momo.ColumnMeta{{Name: "id"}, {Name: "name"}},
		Ptr: func(r *keyRow, i int) any {
			return [...]any{&r.ID, &r.Name}[i]
		},
		Value: func(r *keyRow, i int) any {
			return [...]any{r.ID, r.Name}[i]
		},
	}
}

func TestMySQLGeneratedKeys(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE keys_t (id bigint PRIMARY KEY AUTO_INCREMENT, name varchar(40) NOT NULL)")

	// Single row: LAST_INSERT_ID is written back to the model.
	one := keyRow{Name: "one"}
	if _, err := momo.Insert[keyRow, *keyRow](db).Model(&one).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if one.ID == 0 {
		t.Fatal("generated id was not written back to the model")
	}

	// Multi-row insert that needs generated keys: rejected before any statement runs.
	many := []keyRow{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	if _, err := momo.Insert[keyRow, *keyRow](db).Models(many).Exec(ctx); !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("multi-row generated keys: err = %v, want momo.ErrUnsupported", err)
	}

	// Multi-row insert with explicit primary keys is allowed.
	explicit := []keyRow{{ID: 100, Name: "x"}, {ID: 101, Name: "y"}, {ID: 102, Name: "z"}}
	if _, err := momo.Insert[keyRow, *keyRow](db).Models(explicit).Exec(ctx); err != nil {
		t.Fatalf("explicit keys: %v", err)
	}
}
```

Expect FAIL with `undefined: keyRow`-independent symbol error (`Model` or `Models` unimplemented) before Step 4. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 go test -tags integration ./driver/mysqldriver -run TestMySQLGeneratedKeys -count=1`; expect `ok`.

- [ ] Step 4. Implement `driver/mysqldriver/generated_keys.go`: after a single-row insert, `res.LastInsertId()` is returned to the root builder through `Result`; the root maps it to the auto-increment column recorded in `Meta[T].Identity`. On MariaDB with `InsertReturning`, the root uses `RETURNING` instead. Re-run the Step 3 command; expect `ok`.
- [ ] Step 5. Commit: `feat(momo): render MySQL upserts and generated keys`.

## Task 4: Locking, savepoints, isolation and read-only transactions on MySQL

Covers: S-59 (locking), S-7 (transactions on MySQL), S-4 (locking clauses: NOWAIT, SKIP LOCKED). Acceptance: `TestTransactions` (MySQL case, this task), `TestMutationGuardrails` (MySQL case is covered by momo-05's suite through Task 8).

Files:

- Create `mysql_lock_render.go` (root): `renderMySQLLock`.
- Create `mysql_lock_render_test.go`: `TestMySQLLockRendering`.
- Create `driver/mysqldriver/tx_integration_test.go` (`//go:build integration`): `TestTransactions`, `TestMySQLLockSyntax`.

Interfaces (exact):

```go
// mysql_lock_render.go (package momo)
// renderMySQLLock appends the locking clause for a SELECT.
//   FOR UPDATE            -> FOR UPDATE                (MySQL 8.0+, MariaDB)
//   FOR SHARE, MySQL      -> FOR SHARE                 (MySQL 8.0.1+)
//   FOR SHARE, MariaDB    -> LOCK IN SHARE MODE        (see version record)
//   NOWAIT                -> NOWAIT                    (MySQL 8.0+, MariaDB 10.3+)
//   SKIP LOCKED           -> SKIP LOCKED               (MySQL 8.0+, MariaDB 10.6+ InnoDB only)
func renderMySQLLock(b *Builder, flavor dialect.Flavor, share bool, opts ...string)
```

The lock clauses come from plan momo-05's `ForUpdate(opt ...string)` and `ForShare(opt ...string)`, with the string options `"NOWAIT"` and `"SKIP LOCKED"`; this plan consumes them unchanged.

- [ ] Step 1. Write `mysql_lock_render_test.go` (package `momo`, internal, because it calls `renderMySQLLock`):

```go
package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestMySQLLockRendering(t *testing.T) {
	cases := []struct {
		name   string
		flavor dialect.Flavor
		share  bool
		opts   []string
		want   string
	}{
		{"update", dialect.FlavorMySQL, false, nil, " FOR UPDATE"},
		{"share on mysql", dialect.FlavorMySQL, true, nil, " FOR SHARE"},
		{"share on mariadb", dialect.FlavorMariaDB, true, nil, " LOCK IN SHARE MODE"},
		{"update nowait", dialect.FlavorMySQL, false, []string{"NOWAIT"}, " FOR UPDATE NOWAIT"},
		{"update skip locked", dialect.FlavorMySQL, false, []string{"SKIP LOCKED"}, " FOR UPDATE SKIP LOCKED"},
	}
	for _, c := range cases {
		b := NewBuilder(dialect.MySQLFlavor(c.flavor))
		renderMySQLLock(b, c.flavor, c.share, c.opts...)
		if got := b.String(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
```

The options are the string options of plan momo-05's `ForUpdate(opt ...string)` and `ForShare(opt ...string)`: `"NOWAIT"` and `"SKIP LOCKED"`. This step consumes them unchanged. Expect FAIL with `undefined: renderMySQLLock` before Step 2. Run `go test . -run TestMySQLLockRendering -count=1`; expect `ok`.

- [ ] Step 2. Implement `renderMySQLLock`. Run `go test . -run TestMySQLLockRendering -count=1`; expect `ok`.
- [ ] Step 3. Verify MariaDB `FOR SHARE` on the pinned image before relying on the decision: run the integration test `TestMySQLLockSyntax` (Step 4) against `mariadb:11.8.9`. If `SELECT ... FOR SHARE` succeeds there, change the MariaDB branch to `FOR SHARE` and update the version record row. Record the result in the commit message body.
- [ ] Step 4. Write `driver/mysqldriver/tx_integration_test.go` (`//go:build integration`, package `mysqldriver_test`). `TestTransactions` covers commit, rollback, panic re-raise, savepoint rollback and read-only; `TestMySQLLockSyntax` checks that a `NOWAIT` lock on a row held by another transaction fails:

```go
//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func countRows(t *testing.T, q momo.Querier, table string) int64 {
	t.Helper()
	n, err := momo.Exec(context.Background(), q, "INSERT INTO tx_sink SELECT id FROM "+table)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTransactions(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE t (id bigint PRIMARY KEY)")
	mustExecMySQL(t, db, "CREATE TABLE tx_sink (id bigint)")

	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		_, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (1)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, "t"); got != 1 {
		t.Fatalf("commit: rows = %d, want 1", got)
	}

	wantErr := errors.New("boom")
	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (2)"); err != nil {
			return err
		}
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("rollback: err = %v, want boom", err)
	}
	if got := countRows(t, db, "t"); got != 1 {
		t.Fatalf("rollback: rows = %d, want 1", got)
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("panic was swallowed")
			}
		}()
		err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
			if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (3)"); err != nil {
				t.Fatal(err)
			}
			panic("callback panic")
		})
		t.Fatalf("Tx returned %v after a panic; the panic must be re-raised", err)
	}()
	if got := countRows(t, db, "t"); got != 1 {
		t.Fatalf("panic: rows = %d, want 1", got)
	}

	if err := db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (4)"); err != nil {
			return err
		}
		if err := tx.Savepoint(ctx, "sp"); err != nil {
			return err
		}
		if _, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (5)"); err != nil {
			return err
		}
		return tx.RollbackTo(ctx, "sp")
	}); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, "t"); got != 2 {
		t.Fatalf("savepoint: rows = %d, want 2 (ids 1 and 4)", got)
	}

	if err := db.Tx(ctx, momo.TxOptions{ReadOnly: true}, func(tx *momo.Tx) error {
		_, err := momo.Exec(ctx, tx, "INSERT INTO t VALUES (6)")
		return err
	}); err == nil {
		t.Fatal("INSERT in a read-only transaction succeeded")
	}
}

func TestMySQLLockSyntax(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExecMySQL(t, db, "CREATE TABLE locks_t (id bigint PRIMARY KEY)")
	mustExecMySQL(t, db, "INSERT INTO locks_t VALUES (1)")
	if err := db.Tx(ctx, momo.TxOptions{}, func(holder *momo.Tx) error {
		if _, err := momo.Exec(ctx, holder, "SELECT id FROM locks_t WHERE id = 1 FOR UPDATE"); err != nil {
			return err
		}
		return db.Tx(ctx, momo.TxOptions{}, func(other *momo.Tx) error {
			_, err := momo.Exec(ctx, other, "SELECT id FROM locks_t WHERE id = 1 FOR UPDATE NOWAIT")
			if err == nil {
				t.Error("NOWAIT on a locked row succeeded")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
}
```

Expect FAIL with `undefined: TxOptions` or a missing savepoint method before implementation. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 MOMO_TEST_MARIADB_URL=$MA go test -tags integration ./driver/mysqldriver -run 'TestTransactions|TestMySQLLockSyntax' -count=1`; expect `ok` on both servers. The `TestMySQLLockSyntax` run on MariaDB decides the `FOR SHARE` branch of Step 3.

- [ ] Step 5. Commit: `feat(momo): render MySQL locking clauses and verify transactions`.

## Task 5: migrate/mysql inspection and rendering

Covers: S-60 (information_schema inspection, diff and SQL generation), S-31 (type mapping inspection round trip). Acceptance: `TestMySQLMigrationRoundTrip` (this task creates it; the apply half is Task 6).

Files:

- Create `migrate/mysql/inspect.go`: `Dialect.Inspect`.
- Create `migrate/mysql/render.go`: `Dialect.Render`.
- Create `migrate/mysql/normalize_test.go`: `TestMySQLNormalizeType`.
- Create `migrate/mysql/roundtrip_integration_test.go` (`//go:build integration`): `TestMySQLMigrationRoundTrip`.

Interfaces (exact, from the contract): `migrate/mysql` type `Dialect` implements `migrate.Dialect{Inspect(ctx, momo.PinnedConn, migrate.InspectOptions) (*schema.Schema, error); Render(plan []migrate.Change) ([]migrate.Statement, error); Lock(ctx, momo.PinnedConn) (unlock func() error, err error)}`. Lock is Task 6.

Inspection queries (all with the current `DATABASE()` schema and bound parameters):

- tables: `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE = 'BASE TABLE' AND TABLE_NAME <> 'momo_migrations'`
- columns: `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_DEFAULT, EXTRA, GENERATION_EXPRESSION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME, ORDINAL_POSITION`
- indexes: `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, COLUMN_NAME, SUB_PART, COLLATION, INDEX_TYPE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`
- foreign keys: `information_schema.KEY_COLUMN_USAGE` joined with `information_schema.REFERENTIAL_CONSTRAINTS` for ON DELETE/ON UPDATE
- checks: `information_schema.CHECK_CONSTRAINTS` (MySQL 8.0.16+ and MariaDB 10.2.1+)

Rendering rules: identifiers via the MySQL dialect (backticks); `CREATE TABLE` with `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`; `ON DELETE`/`ON UPDATE` clauses; `CHECK` expressions as stored; `VECTOR(N)` for vector columns on MySQL 9.0+ and MariaDB (`vector(N)` in schema; the server-version check is in Task 7).

- [ ] Step 1. Write `migrate/mysql/normalize_test.go` (package `mysql`, internal):

```go
package mysql

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/schema"
)

func TestMySQLNormalizeType(t *testing.T) {
	cases := map[string]string{
		"int(11)":            "int",
		"bigint(20) unsigned": "bigint unsigned",
		"varchar(255)":       "varchar(255)",
		"tinyint(1)":         "tinyint(1)",
		"VECTOR(1536)":       "vector(1536)",
		"longtext":           "longtext",
	}
	for in, want := range cases {
		if got := schema.NormalizeType(dialect.MySQL, in); got != want {
			t.Errorf("NormalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}
```

Expect FAIL with a mismatch on the first case (`int(11)` is still `int(11)`) before Step 2. Run `go test ./schema ./migrate/mysql -run TestMySQLNormalizeType -count=1`; expect `ok`.

- [ ] Step 2. Implement `NormalizeType` branch for MySQL in `schema/normalize.go` (momo-01 file): drop display widths on integer types except `tinyint(1)`, lower-case, collapse spaces. Run `go test ./schema ./migrate/mysql -run TestMySQLNormalizeType -count=1`; expect `ok`.
- [ ] Step 3. Write `TestMySQLMigrationRoundTrip` (integration, package `mysql_test`, file `roundtrip_integration_test.go`). It applies the example schema on MySQL 8.4, MySQL 9.7 and MariaDB 11.8, asserts no drift, and asserts a second diff with no statements. The example covers an auto-increment key, a unique key, a composite foreign key with `ON DELETE CASCADE`, a generated column, a `CHECK` and an index on the generated column:

```go
//go:build integration

package mysql_test

import (
	"context"
	"os"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
)

// writeMigration writes the files that a Diff returned into dir.
func writeMigration(t *testing.T, dir string, m *migrate.Migration) {
	t.Helper()
	files := make(map[string]string, len(m.Files))
	for name, body := range m.Files {
		files[name] = string(body)
	}
	migratetest.WriteFiles(t, dir, files)
}

func exampleSchema() *schema.Schema {
	return &schema.Schema{
		Dialect: "mysql",
		Tables: []schema.Table{
			{
				Name:       "parents",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "a", Type: "int"},
					{Name: "b", Type: "int"},
				},
				Uniques: []schema.Unique{{Name: "parents_ab", Columns: []string{"a", "b"}}},
			},
			{
				Name:       "children",
				PrimaryKey: []string{"id"},
				Columns: []schema.Column{
					{Name: "id", Type: "bigint", Identity: "by_default"},
					{Name: "pa", Type: "int"},
					{Name: "pb", Type: "int"},
					{Name: "total", Type: "int", Generated: "pa + pb"},
				},
				ForeignKeys: []schema.ForeignKey{{
					Name:       "children_parent",
					Columns:    []string{"pa", "pb"},
					RefTable:   "parents",
					RefColumns: []string{"a", "b"},
					OnDelete:   "CASCADE",
				}},
				Checks:  []schema.Check{{Name: "pa_nonnegative", Expr: "pa >= 0"}},
				Indexes: []schema.Index{{Name: "children_total", Parts: []schema.IndexPart{{Column: "total"}}}},
			},
		},
	}
}

func TestMySQLMigrationRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, target := range []struct {
		name string
		open func(*testing.T) *momo.DB
	}{
		{"mysql-8.4", testopen.MySQL84},
		{"mysql-9.7", testopen.MySQL97},
		{"mariadb-11.8", testopen.MariaDB},
	} {
		t.Run(target.name, func(t *testing.T) {
			db := target.open(t)
			dir := t.TempDir()
			eng := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
			m, err := eng.Diff(ctx, exampleSchema(), "example", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			writeMigration(t, dir, m)
			if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
				t.Fatal(err)
			}
			drift, err := eng.Drift(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if !drift.Clean() {
				t.Fatalf("drift after apply: %+v", drift)
			}
			again, err := eng.Diff(ctx, exampleSchema(), "noop", migrate.DiffOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if again != nil && len(again.Changes) != 0 {
				t.Fatalf("second diff produced changes: %+v", again.Changes)
			}
		})
	}
}
```

`db.Driver()` is the accessor that hands the engine its `momo.Driver`; plan momo-01 publishes it. Expect FAIL before Tasks 5 and 6 with a non-empty diff or a missing symbol. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 MOMO_TEST_MYSQL97_URL=$MY97 MOMO_TEST_MARIADB_URL=$MA go test -tags integration ./migrate/mysql -run TestMySQLMigrationRoundTrip -count=1`; expect `ok`.

- [ ] Step 4. Implement `inspect.go` with the queries above and `render.go` with the rules above. Re-run Step 3; expect `ok`.
- [ ] Step 5. Commit: `feat(migrate/mysql): inspect and render MySQL schemas`.

## Task 6: migrate/mysql execution, GET_LOCK, statement-level recovery and online DDL

Covers: S-60 (GET_LOCK-based concurrency lock, explicit handling of implicit-commit DDL with statement-level recovery, ALGORITHM/LOCK options), S-11 (partial-failure recovery on a non-transactional path), S-10 (apply under lock). Acceptance: `TestMySQLPartialFailure`, `TestConcurrentApply` (MySQL case).

Files:

- Create `migrate/mysql/lock.go`: `Dialect.Lock`.
- Create `migrate/mysql/exec.go`: statement-level execution hook used by the engine for MySQL.
- Create `migrate/mysql/lock_test.go` (`//go:build integration`): `TestMySQLLockHolder`.
- Create `migrate/mysql/partial_integration_test.go` (`//go:build integration`): `TestMySQLPartialFailure`, `TestMySQLOnlineDDL`.
- Modify `migrate/*.go` (momo-04's engine) only at the points listed below, through the contract's `Dialect.TransactionalDDL` capability.

Interfaces (exact):

```go
// migrate/mysql
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, key string) (unlock func() error, holder func(context.Context) (string, error), err error)

// lockName returns the GET_LOCK name for an engine key: the key truncated to MySQL's 64-character limit.
func lockName(key string) string
```

Engine rules for MySQL (implemented in the engine files, asserted here):

- `Dialect.TransactionalDDL` is false for MySQL, so the engine treats every migration as non-transactional, with or without `-- momo:no-transaction`.
- Before each statement the history row has `status = 'running'`; after each successful statement `statements_applied` increments in the same connection; on error the row becomes `failed` with `statements_applied` set to the count so far and `error` set to the server message.
- Statements are executed one at a time; the migration file is split with the splitter from momo-04 (`migrate.SplitStatements`), so the go-sql-driver `multiStatements` option stays false.

Online DDL directive: `-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE` on the line before an `ALTER TABLE` statement appends that clause to the statement. This is an addition to the directive table of plan momo-04, which already lists `-- momo:no-transaction`, `-- momo:destructive`, `-- momo:split` and `-- momo:sqlite-foreign-keys-off`; the parser entry is added there, and the Render clause append is implemented in `migrate/mysql`. Without the directive no ALGORITHM or LOCK clause is added.

- [ ] Step 1. Write `TestMySQLPartialFailure` in `migrate/mysql/partial_integration_test.go` (`//go:build integration`, package `mysql_test`). The migration file has two `CREATE TABLE` statements and one failing `INSERT`. Non-transactional execution shows as table `a` existing after the failure, and the history row shows two applied statements:

```go
//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

// appliedStatements reads statements_applied for a version from the history table through the driver.
func appliedStatements(t *testing.T, db *momo.DB, version string) int64 {
	t.Helper()
	rows, err := db.Driver().Query(context.Background(), "SELECT statements_applied FROM momo_migrations WHERE version = ?", []any{version})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	if !rows.Next() {
		t.Fatalf("no history row for %s: %v", version, rows.Err())
	}
	var n int64
	if err := rows.Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestMySQLPartialFailure(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	const version = "20260101000000"
	sql := "CREATE TABLE a (id bigint PRIMARY KEY);\nCREATE TABLE b (id bigint PRIMARY KEY);\nINSERT INTO missing_table VALUES (1);\n"
	if err := os.WriteFile(filepath.Join(dir, version+"_partial.up.sql"), []byte(sql), 0o644); err != nil {
		t.Fatal(err)
	}
	eng := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err == nil {
		t.Fatal("apply of a failing migration succeeded")
	}
	if _, err := momo.Exec(ctx, db, "SELECT 1 FROM a LIMIT 1"); err != nil {
		t.Fatalf("table a is missing; a non-transactional migration keeps earlier DDL: %v", err)
	}
	states, err := eng.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var state string
	for _, st := range states {
		if st.Version == version {
			state = string(st.State)
		}
	}
	if state != "dirty" {
		t.Fatalf("status of %s = %q, want dirty", version, state)
	}
	if got := appliedStatements(t, db, version); got != 2 {
		t.Fatalf("statements_applied = %d, want 2", got)
	}
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); !errors.Is(err, migrate.ErrDirty) {
		t.Fatalf("second apply: err = %v, want migrate.ErrDirty", err)
	}
	if err := eng.Resolve(ctx, version, migrate.ResolveRolledBack); err != nil {
		t.Fatal(err)
	}
}
```

`Status`, `Version` and the `Status`/`StatementsApplied` fields are the names plan momo-04 publishes for the history view. Expect FAIL before Task 6 Step 3 with a history row showing `applied`, or a missing symbol. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 go test -tags integration ./migrate/mysql -run TestMySQLPartialFailure -count=1`; expect `ok`.

- [ ] Step 2. Write `TestMySQLLockHolder` (integration, package `mysql_test`, file `lock_integration_test.go`). A first migration holds the migration lock for three seconds through one engine; a contender engine with a one-second lock wait must fail with `migrate.ErrLocked`:

```go
//go:build integration

package mysql_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

func TestMySQLLockHolder(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	dir := t.TempDir()
	// The first migration holds the migration lock for three seconds.
	if err := os.WriteFile(filepath.Join(dir, "20260101000000_hold.up.sql"), []byte("SELECT SLEEP(3);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	holder := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
	contender := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))

	done := make(chan error, 1)
	go func() {
		_, err := holder.Apply(ctx, migrate.ApplyOptions{})
		done <- err
	}()
	time.Sleep(500 * time.Millisecond) // let the holder take the lock first

	_, err := contender.Apply(ctx, migrate.ApplyOptions{LockWait: time.Second})
	if !errors.Is(err, migrate.ErrLocked) {
		t.Fatalf("contender apply: err = %v, want migrate.ErrLocked", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("holder apply: %v", err)
	}
}
```

The second engine contends for the lock while the first holds it, so the test does not depend on the lock name. Expect FAIL before Task 6 Step 3 with a missing `Lock` symbol. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 go test -tags integration ./migrate/mysql -run TestMySQLLockHolder -count=1`; expect `ok`.

- [ ] Step 3. Implement `lock.go` on the pinned connection:

```go
// Lock implements migrate.Dialect. It never blocks: GET_LOCK waits zero seconds, and the engine's
// wait loop retries until ApplyOptions.LockWait expires. A busy lock returns an error wrapping
// migrate.ErrLocked that names the holder; holder reports IS_USED_LOCK.
func (Dialect) Lock(ctx context.Context, c momo.PinnedConn, key string) (func() error, func(context.Context) (string, error), error) {
	name := lockName(key)
	got, err := scalar(ctx, c, "SELECT GET_LOCK(?, 0)", name)
	if err != nil {
		return nil, nil, err
	}
	holder := func(ctx context.Context) (string, error) {
		id, err := scalar(ctx, c, "SELECT IS_USED_LOCK(?)", name)
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(id, 10), nil
	}
	if got != 1 {
		h, err := holder(ctx)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("%w: %s held by connection %s", migrate.ErrLocked, name, h)
	}
	return func() error {
		_, err := scalar(ctx, c, "SELECT RELEASE_LOCK(?)", name)
		return err
	}, holder, nil
}

// lockName truncates the engine key to MySQL's 64-character lock-name limit.
func lockName(key string) string {
	if len(key) > 64 {
		return key[:64]
	}
	return key
}

// scalar returns the first column of the first row as an integer; a NULL value is 0.
func scalar(ctx context.Context, c momo.PinnedConn, query string, args ...any) (v int64, err error) {
	rows, err := c.Query(ctx, query, args)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := rows.Close(); err == nil {
			err = cerr
		}
	}()
	var n sql.NullInt64
	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return n.Int64, nil
}
```

The lock is taken with `GET_LOCK(name, 0)`. A zero timeout is used on purpose: a negative timeout waits forever on MySQL and returns NULL on MariaDB, so the engine owns the wait through `ApplyOptions.LockWait`.

- [ ] Step 4. Write `TestMySQLOnlineDDL` (integration, package `mysql_test`, file `online_integration_test.go`). A table with 100000 rows gets a column with `ALGORITHM=INSTANT, LOCK=NONE`, which succeeds. A second migration asks for an unsupported combination, which the server rejects; its history records zero statements applied:

```go
//go:build integration

package mysql_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
)

func TestMySQLOnlineDDL(t *testing.T) {
	ctx := context.Background()
	db := testopen.MySQL84(t)
	mustExec(t, db, "CREATE TABLE big (id bigint PRIMARY KEY)")
	mustExec(t, db, "INSERT INTO big (id) SELECT seq FROM (SELECT @r := @r + 1 AS seq FROM information_schema.columns a, information_schema.columns b, (SELECT @r := 0) init LIMIT 100000) s")

	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("20260101000000_add_note.up.sql",
		"-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE\nALTER TABLE big ADD COLUMN note varchar(40) NULL;\n")
	write("20260101000001_bad_alter.up.sql",
		"-- momo:ddl ALGORITHM=INSTANT\nALTER TABLE big MODIFY COLUMN id bigint unsigned;\n")

	eng := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
	if _, err := eng.Apply(ctx, migrate.ApplyOptions{To: "20260101000000"}); err != nil {
		t.Fatalf("instant add column: %v", err)
	}
	if _, err := momo.Exec(ctx, db, "SELECT note FROM big LIMIT 1"); err != nil {
		t.Fatalf("column note missing after the directive migration: %v", err)
	}

	_, err = eng.Apply(ctx, migrate.ApplyOptions{})
	if err == nil || !strings.Contains(err.Error(), "ALGORITHM=INSTANT is not supported") {
		t.Fatalf("unsupported instant change: err = %v", err)
	}
	if got := appliedStatements(t, db, "20260101000001"); got != 0 {
		t.Fatalf("statements_applied for the rejected migration = %d, want 0", got)
	}
}

func mustExec(t *testing.T, q momo.Querier, sql string) {
	t.Helper()
	if _, err := momo.Exec(context.Background(), q, sql); err != nil {
		t.Fatal(err)
	}
}
```

The directive table entry `-- momo:ddl` is added by plan momo-04; `Render` appends the clause in `migrate/mysql`. Expect FAIL before the directive parser exists, with `-- momo:ddl` treated as a comment. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 go test -tags integration ./migrate/mysql -run TestMySQLOnlineDDL -count=1`; expect `ok`.

- [ ] Step 5. Commit: `feat(migrate/mysql): lock with GET_LOCK and recover from statement-level failures`.

## Task 7: MariaDB VECTOR and MySQL VECTOR storage, distance and index capability

Covers: S-61 (MariaDB VECTOR(N), VEC_DISTANCE functions, VECTOR INDEX; MySQL VECTOR storage with distance only where the server provides it), S-15 (vector parity on a second server). Acceptance: `TestMariaDBVectorParity`, `TestMySQLVectorCapability`.

Files:

- Create `vector_mysql.go` (root): the MySQL/MariaDB branch used by `distExpr` (plan momo-07 Task 3), `wrapMySQLVectorArg`, `mysqlVectorColumnRead`.
- Create `vector_mysql_test.go` (root): `TestMySQLVectorSQL`.
- Modify `vector/dialect.go` (plan momo-07 Task 2): `DialectValue` returns the text form `Value()` for `dialect.MySQL`.
- Modify `schema/vector.go` (plan momo-07 Task 4): `ValidateVectorIndex` accepts method `vector` on MariaDB only, with `DISTANCE` in {`euclidean`, `cosine`} and `M` in 3..200; rejects `hnsw`/`ivfflat` on MySQL and MariaDB.
- Create `migrate/mysql/vector.go`: VECTOR type rendering and the MySQL server-version gate.
- Create `driver/mysqldriver/vector_integration_test.go` (`//go:build integration`): `TestMariaDBVectorParity`, `TestMySQLVectorCapability`.

Interfaces (exact):

```go
// vector_mysql.go (package momo)
// Rendered forms (verified in the version record):
//   MariaDB: VEC_DISTANCE_EUCLIDEAN(col, VEC_FromText(?))  (L2Distance)
//            VEC_DISTANCE_COSINE(col, VEC_FromText(?))     (CosineDistance)
//   MySQL (any flavor): not rendered; the Metrics capability is empty, so distExpr returns ErrUnsupported.
// Storage: MySQL VECTOR(N) reads as VECTOR_TO_STRING(col) and writes as STRING_TO_VECTOR(?);
//          MariaDB VECTOR(N) reads as VEC_ToText(col) and writes as VEC_FromText(?).
func wrapMySQLVectorArg(b *Builder, flavor dialect.Flavor, arg any)
func mysqlVectorColumnRead(flavor dialect.Flavor, column string) string

// migrate/mysql/vector.go
// vectorSQLType returns "VECTOR(N)" for MySQL 9.7 and MariaDB 11.8+. For MySQL 8.4 it returns a dialect diagnostic wrapping ErrUnsupported that names the server version, and
// ErrUnsupported with the message "VECTOR requires MySQL 9.0 or MariaDB 11.7" otherwise.
func vectorSQLType(flavor dialect.Flavor, serverVersion string, dim int) (string, error)
```

- [ ] Step 1. Write `vector_mysql_test.go`:

```go
package momo

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/vector"
)

func TestMySQLVectorSQL(t *testing.T) {
	col := NewVectorColumn[vector.Vector]("docs", "emb", 3, 0)
	sql := renderForTest(t, dialect.MySQLFlavor(dialect.FlavorMariaDB), CosineDistance(col, vector.Vector{1, 2, 3}))
	if want := "VEC_DISTANCE_COSINE(`docs`.`emb`, VEC_FromText(?))"; sql != want {
		t.Fatalf("MariaDB cosine = %s, want %s", sql, want)
	}
	if _, err := renderErr(t, dialect.MySQLFlavor(dialect.FlavorMySQL), L2Distance(col, vector.Vector{1, 2, 3})); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("MySQL L2 = %v, want ErrUnsupported", err)
	}
	if _, err := vectorSQLType(dialect.FlavorMySQL, "8.4.13", 3); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("MySQL 8.4 VECTOR = %v, want ErrUnsupported", err)
	}
}
```

Add `errors` to the imports. `renderForTest` and `renderErr` are the helpers from plan momo-07 Task 3, which take a `*dialect.Dialect`; pass `dialect.MySQLFlavor(...)`. Expect FAIL with `undefined: vectorSQLType`.

- [ ] Step 2. Implement `vector_mysql.go`, `migrate/mysql/vector.go` (parse `major.minor` from `SELECT VERSION()`; `8.4.x` returns the dialect diagnostic wrapping ErrUnsupported; `9.7.x` returns `VECTOR(N)`; MariaDB `11.8+` returns `VECTOR(N)`), and the `schema` and `vector` changes listed above. Run `go test . ./migrate/mysql ./schema -run 'TestMySQLVectorSQL|TestVectorIndexRules' -count=1`; expect `ok`.
- [ ] Step 3. Write `TestMariaDBVectorParity` (integration, package `mysqldriver_test`, file `vector_integration_test.go`). Two constants declare the thresholds: `parityTopK` is 10 and `minIndexedOverlap` is 9. The exact path must match PostgreSQL pgvector exact search id for id; the indexed path must share at least `minIndexedOverlap` ids with the exact result:

```go
//go:build integration

package mysqldriver_test

import (
	"context"
	"database/sql"
	"math/rand"
	"slices"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/go-sql-driver/mysql"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/vector"
)

const (
	parityTopK        = 10
	minIndexedOverlap = 9
	parityVectors     = 300
	parityDim         = 8
)

type parityRow struct {
	ID  int64
	Emb vector.Vector
}

func (r *parityRow) MomoMeta() *momo.Meta[parityRow] {
	return &momo.Meta[parityRow]{Table: "docs", PK: []int{0}, Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}}}
}

// idsFrom runs query on a database/sql handle and returns the first column.
func idsFrom(t *testing.T, db *sql.DB, query string, args ...any) []int64 {
	t.Helper()
	rows, err := db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Error(err)
		}
	}()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestMariaDBVectorParity(t *testing.T) {
	ctx := context.Background()
	r := rand.New(rand.NewSource(5))
	vecs := make([]vector.Vector, parityVectors)
	for i := range vecs {
		vecs[i] = make(vector.Vector, parityDim)
		for j := range vecs[i] {
			vecs[i][j] = float32(r.Float64()*2 - 1)
		}
	}
	query := vecs[0]
	text, err := query.Value()
	if err != nil {
		t.Fatal(err)
	}

	// PostgreSQL reference: exact search.
	pg := testopen.Postgres(t)
	mustExecPar(t, pg, "CREATE TABLE docs (id bigint PRIMARY KEY, emb vector(8) NOT NULL)")
	for i, v := range vecs {
		mustExecPar(t, pg, "INSERT INTO docs (id, emb) VALUES (?, ?)", int64(i+1), v)
	}
	exactPG := []int64{}
	if err := pg.WithSearch(ctx, momo.SearchSettings{ExactSearch: true}, func(tx *momo.Tx) error {
		_, err := momo.Exec(ctx, tx, "CREATE TABLE exact_top AS SELECT id, row_number() OVER (ORDER BY emb <=> ?, id) AS rnk FROM docs ORDER BY emb <=> ?, id LIMIT 10", query, query)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	pgRaw, err := sql.Open("pgx", testdb.PostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pgRaw.Close(); err != nil {
			t.Error(err)
		}
	})
	exactPG = idsFrom(t, pgRaw, "SELECT id FROM exact_top ORDER BY rnk")

	// MariaDB: the same vectors, a VECTOR INDEX, and the two query forms.
	ma := testopen.MariaDB(t)
	mustExecPar(t, ma, "CREATE TABLE docs (id bigint PRIMARY KEY, emb VECTOR(8) NOT NULL, VECTOR INDEX (emb) M=8 DISTANCE=cosine)")
	for i, v := range vecs {
		mustExecPar(t, ma, "INSERT INTO docs (id, emb) VALUES (?, VEC_FromText(?))", int64(i+1), mustText(t, v))
	}
	maRaw, err := sql.Open("mysql", testdb.MariaDBURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := maRaw.Close(); err != nil {
			t.Error(err)
		}
	})

	// Exact path: "+ 0" keeps the server from using the index, so the result is exact.
	exactMA := idsFrom(t, maRaw, "SELECT id FROM docs ORDER BY VEC_DISTANCE_COSINE(emb, VEC_FromText(?)) + 0, id LIMIT 10", text)
	if !slices.Equal(exactMA, exactPG) {
		t.Fatalf("exact path: mariadb %v, pgvector %v", exactMA, exactPG)
	}

	// Indexed path: the momo query, which the server serves from the VECTOR INDEX.
	col := momo.NewVectorColumn[vector.Vector]("docs", "emb", parityDim, 0)
	c, err := momo.Select[parityRow, *parityRow](ma).OrderBy(momo.CosineDistance(col, query).Asc()).Limit(parityTopK).SQL()
	if err != nil {
		t.Fatal(err)
	}
	indexed := idsFrom(t, maRaw, c.SQL, c.Args...)
	overlap := 0
	for _, id := range indexed {
		if slices.Contains(exactPG, id) {
			overlap++
		}
	}
	if overlap < minIndexedOverlap {
		t.Fatalf("indexed path shares %d of %d ids with the exact result, want at least %d", overlap, parityTopK, minIndexedOverlap)
	}
}
```

`mustExecPar` is `mustExecMySQL` generalized to any `momo.Querier`; define it in this file as a one-line wrapper around `momo.Exec` with `t.Fatal` on error, and `mustText(t, v)` calls `v.Value()` and returns the string, failing on error. Expect FAIL with `undefined: testopen.MariaDB` before Task 8 Step 2. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MARIADB_URL=$MA MOMO_TEST_POSTGRES_URL=$PG go test -tags integration ./driver/mysqldriver -run TestMariaDBVectorParity -count=1`; expect `ok`.

- [ ] Step 4. Write `TestMySQLVectorCapability` (integration, package `mysqldriver_test`, file `vector_capability_integration_test.go`) with one subtest per MySQL server. MySQL 8.4 must refuse a `vector(3)` declaration at diff time with a dialect diagnostic that wraps `momo.ErrUnsupported`. MySQL 9.7 must store the vector and return `ErrUnsupported` for distance queries:

```go
//go:build integration

package mysqldriver_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
	"github.com/azrtydxb/momo/migrate"
	"github.com/azrtydxb/momo/migrate/migratetest"
	mmysql "github.com/azrtydxb/momo/migrate/mysql"
	"github.com/azrtydxb/momo/schema"
	"github.com/azrtydxb/momo/vector"
)

// writeMigration writes the files that a Diff returned into dir.
func writeMigration(t *testing.T, dir string, m *migrate.Migration) {
	t.Helper()
	files := make(map[string]string, len(m.Files))
	for name, body := range m.Files {
		files[name] = string(body)
	}
	migratetest.WriteFiles(t, dir, files)
}

type capRow struct {
	ID  int64
	Emb vector.Vector
}

func (r *capRow) MomoMeta() *momo.Meta[capRow] {
	return &momo.Meta[capRow]{Table: "vtab", PK: []int{0}, Columns: []momo.ColumnMeta{{Name: "id"}, {Name: "emb"}}}
}

func vectorTableSchema() *schema.Schema {
	return &schema.Schema{Dialect: "mysql", Tables: []schema.Table{{
		Name:       "vtab",
		PrimaryKey: []string{"id"},
		Columns: []schema.Column{
			{Name: "id", Type: "bigint"},
			{Name: "emb", Type: "vector(3)"},
		},
	}}}
}

func TestMySQLVectorCapability(t *testing.T) {
	ctx := context.Background()
	t.Run("mysql-8.4 refuses vector columns", func(t *testing.T) {
		db := testopen.MySQL84(t)
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
		_, err := eng.Diff(ctx, vectorTableSchema(), "vectors", migrate.DiffOptions{})
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("diff: err = %v, want momo.ErrUnsupported", err)
		}
		if !strings.Contains(err.Error(), "8.4") {
			t.Fatalf("diagnostic %q does not name the server version", err)
		}
	})
	t.Run("mysql-9.7 stores and refuses distance", func(t *testing.T) {
		db := testopen.MySQL97(t)
		dir := t.TempDir()
		eng := migratetest.NewEngine(t, mmysql.Dialect{}, db.Driver(), os.DirFS(dir))
		m, err := eng.Diff(ctx, vectorTableSchema(), "vectors", migrate.DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		writeMigration(t, dir, m)
		if _, err := eng.Apply(ctx, migrate.ApplyOptions{}); err != nil {
			t.Fatal(err)
		}
		mustExecPar(t, db, "INSERT INTO vtab (id, emb) VALUES (?, STRING_TO_VECTOR(?))", int64(1), "[1,2,3]")
		col := momo.NewVectorColumn[vector.Vector]("vtab", "emb", 3, 0)
		_, err = momo.Select[capRow, *capRow](db).OrderBy(momo.L2Distance(col, vector.Vector{1, 2, 3}).Asc()).Limit(1).All(ctx)
		if !errors.Is(err, momo.ErrUnsupported) {
			t.Fatalf("distance query: err = %v, want momo.ErrUnsupported", err)
		}
	})
}
```

The HeatWave/MySQL AI `DISTANCE()` function is neither probed nor used. The query orders by the distance expression: `OrderBy` takes the `Orderable` that `L2Distance` returns, so no comparison method is needed. Expect FAIL before Task 7 Step 2 with `undefined: vectorSQLType` or a missing symbol. Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 MOMO_TEST_MYSQL97_URL=$MY97 go test -tags integration ./driver/mysqldriver -run TestMySQLVectorCapability -count=1`; expect `ok`.

- [ ] Step 5. Commit: `feat(momo): add MariaDB VECTOR and MySQL vector storage capability`.

## Task 8: Pinned MySQL and MariaDB services and the shared suites

Covers: S-59 (`TestMySQLIntegration`: the shared query, write, transaction and error suites run against MySQL and MariaDB containers), S-20 (CI integration against MySQL and MariaDB containers). Acceptance: `TestMySQLIntegration`.

Files:

- Modify `internal/testdb/compose.yaml` (momo-01): services `mysql` (`mysql:8.4.13`), `mysql-9.7` (`mysql:9.7.4`), `mariadb` (`mariadb:11.8.9`); each with a health check; root password from `MOMO_TEST_ROOT_PASSWORD` with a local default.
- Consume the URL helpers from `internal/testdb` (momo-01 owns them; this plan adds none): `MySQL84URL(t)`, `MySQL97URL(t)` and `MariaDBURL(t)`, reading `MOMO_TEST_MYSQL84_URL`, `MOMO_TEST_MYSQL97_URL` and `MOMO_TEST_MARIADB_URL`. The `*momo.DB` constructors are `testopen.MySQL84(t)`, `testopen.MySQL97(t)` and `testopen.MariaDB(t)` (momo-03).
- Create `driver/mysqldriver/integration_test.go` (`//go:build integration`, package `mysqldriver_test`): `TestMySQLIntegration`.

Interfaces (exact):

```go
// Test packages: TestMySQLIntegration is in package mysqldriver_test because testopen imports this driver.
// Consumed from plan momo-05 (this plan does not define them):
//   suite.RunAll(t *testing.T, db *momo.DB, caps <capability set>)
//   the capability set for a handle's dialect, constructed by plan momo-05 (called CapabilitiesOf there)
// Consumed from plan momo-03 (driver/mysqldriver):
//   mysqldriver.New(db *sql.DB, flavor dialect.Flavor) momo.Driver
```

The shared suites run the same assertions as PostgreSQL. A result that differs from PostgreSQL must be `momo.ErrUnsupported` and listed in `docs/mysql.md` (Task 9).

- [ ] Step 1. Write `driver/mysqldriver/integration_test.go`:

```go
//go:build integration

package mysqldriver_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/suite"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestMySQLIntegration(t *testing.T) {
	for _, target := range []struct {
		name string
		open func(*testing.T) *momo.DB
	}{
		{"mysql-8.4", testopen.MySQL84},
		{"mysql-9.7", testopen.MySQL97},
		{"mariadb-11.8", testopen.MariaDB},
	} {
		t.Run(target.name, func(t *testing.T) {
			db := target.open(t)
			suite.RunAll(t, db, suite.CapabilitiesOf(db.Dialect()))
		})
	}
}

```

Expect FAIL with `undefined: testopen.MySQL84` before Step 2.

- [ ] Step 2. Confirm the compose services for the three images and the URL helpers `MySQL84URL(t)`, `MySQL97URL(t)` and `MariaDBURL(t)` from plan momo-01. For each image run `docker manifest inspect mysql:8.4.13`, `docker manifest inspect mysql:9.7.4` and `docker manifest inspect mariadb:11.8.9` (all three verified on Docker Hub on 2026-11-04). Run `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_MYSQL84_URL=$MY84 MOMO_TEST_MYSQL97_URL=$MY97 MOMO_TEST_MARIADB_URL=$MA go test -tags integration ./driver/mysqldriver -run TestMySQLIntegration -count=1`; expect `ok` on all three subtests.
- [ ] Step 3. Commit: `test(mysql): run the shared suites on MySQL 8.4, MySQL 9.7 and MariaDB 11.8`.

## Task 9: MySQL documentation and capability matrix

Covers: S-21 (MySQL capability matrix and migration recovery section), S-60 (the online DDL directive and the implicit-commit recovery procedure documented), S-59 (capability differences documented). Acceptance: `TestCapabilityMatrixMatchesDialect`.

Files:

- Create `docs/mysql.md`.
- Modify `docs/README.md`: add one row for `docs/mysql.md` to the page index.
- Create `dialect/matrix_test.go`: `TestCapabilityMatrixMatchesDialect`.

Required content of `docs/mysql.md`, in this order:

1. Supported servers: MySQL 8.4 LTS (`mysql:8.4.13`), MySQL 9.7 LTS (`mysql:9.7.4`) and MariaDB 11.8 LTS (`mariadb:11.8.9`). No innovation releases are supported. The supported-versions record in `docs/README.md` mirrors this list.
2. DSN requirements: `parseTime=true&loc=UTC&charset=utf8mb4`; `multiStatements` stays false.
3. Capability table with one row per capability and columns `mysql` and `mariadb`, generated from `dialect.MySQLFlavor` and checked by the test. Rows include `Returning`, `InsertReturning`, `OnConflict`, `OnDuplicateKey`, `NullsOrdering`, `TransactionalDDL`, `Merge`, `ILike` and `Vector.Metrics`.
4. Writes: `LAST_INSERT_ID` for single-row generated keys; multi-row inserts that need generated keys return `ErrUnsupported`; `OnConflict` targets are accepted only when they are the sole unique key of the table (`ErrUnsupported` otherwise); MariaDB uses `VALUES()` in upserts.
5. Vectors: MySQL 8.4 has no vector columns: declaring one fails at `migrate diff` with a dialect diagnostic wrapping `momo.ErrUnsupported`, and vector queries return `ErrUnsupported`. MySQL 9.7 stores VECTOR, and every distance query returns `ErrUnsupported`; HeatWave/MySQL AI `DISTANCE()` is neither supported nor detected. MariaDB 11.8 supports full search, including VECTOR INDEX, which is approximate; its exact-search path is documented in `docs/vectors.md`.
6. Migrations and recovery: DDL implies a commit on MySQL, so every migration is non-transactional and history records `statements_applied` after each statement; a failed migration leaves `status = failed`; `momo migrate status` reports it as dirty; the recovery procedure is manual repair, then `momo migrate resolve --version N --applied` or `--rolled-back`. Apply refuses while dirty.
7. Online DDL directive: `-- momo:ddl ALGORITHM=INSTANT, LOCK=NONE` before an `ALTER TABLE` statement appends those clauses; the server rejects unsupported combinations and the migration then stops with `statements_applied` recorded.
8. Locking: `GET_LOCK` serializes apply with a 30-second default wait; the holder is named in the error.

Test:

```go
package dialect

import (
	"os"
	"strings"
	"testing"
)

func TestCapabilityMatrixMatchesDialect(t *testing.T) {
	raw, err := os.ReadFile("../docs/mysql.md")
	if err != nil {
		t.Fatal(err)
	}
	my := MySQLFlavor(FlavorMySQL)
	ma := MySQLFlavor(FlavorMariaDB)
	rows := map[string][2]bool{
		"Returning":       {my.Returning, ma.Returning},
		"InsertReturning": {my.InsertReturning, ma.InsertReturning},
		"OnDuplicateKey":  {my.OnDuplicateKey, ma.OnDuplicateKey},
		"TransactionalDDL": {my.TransactionalDDL, ma.TransactionalDDL},
	}
	for name, v := range rows {
		want := "| " + name + " | " + yesNo(v[0]) + " | " + yesNo(v[1]) + " |"
		if !strings.Contains(string(raw), want) {
			t.Errorf("docs/mysql.md lacks matrix row %q", want)
		}
	}
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
```

Expect FAIL with `open ../docs/mysql.md: no such file or directory` before Step 2.

- [ ] Step 2. Write `docs/mysql.md` with the required content above (the capability rows must match the test's `| name | mysql | mariadb |` format with `yes` or `no`), and add the row to `docs/README.md`. Run `go test ./dialect -run TestCapabilityMatrixMatchesDialect -count=1`; expect `ok`.
- [ ] Step 3. Commit: `docs(mysql): document capabilities, migration recovery and the online DDL directive`.

## Verification of this plan

- [ ] `go test ./... -count=1` is green on Go 1.26 without any database URL set (integration tests skip).
- [ ] `MOMO_REQUIRE_INTEGRATION=1 go test -tags integration ./... -count=1` is green against `mysql:8.4.13`, `mysql:9.7.4`, `mariadb:11.8.9` and the PostgreSQL and SQLite services.
- [ ] `go list -deps ./dialect ./schema ./vector ./momo` shows no `github.com/go-sql-driver/mysql` and no `migrate` package.
- [ ] `go test -race ./driver/mysqldriver ./migrate/mysql -count=1` is green.
