# momo-01-foundation — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Turn the uncommitted spike on branch `init` into a tested, CI-governed foundation: the normalized schema, the dialect capability set, the root query builder with immutable queries and unit-level SQL golden tests, structured error sentinels, the driver contract declarations, the integration-test harness with pinned service images, the CI, security, and release workflows, and the import-graph guard that every later plan depends on.

## Architecture

The stdlib-only core is the root `momo` package plus `dialect`, `schema`, `vector` and `migrate` (sentinels only). `TestImportGraph` enforces that core packages depend on nothing outside the module. Test-only helpers live under `internal/` (`fakedriver`, `testdb`, `repocheck`); `internal/repocheck` parses `.github/` and the Makefile in tests so workflow rules are checked the same way as code. Every spike file is repaired task by task against docs/ARCHITECTURE.md: a name the contract requires is added, a defect the spike has is fixed under a failing test, and nothing is deleted without a replacement. Build order follows docs/ARCHITECTURE.md (Delivery order): the PostgreSQL path (Tasks 18 and 19) lands before any SQLite or MySQL entry, and the SQLite and MySQL CI entries are added by their milestone plans.

## Constraints

Copied verbatim from `.procoder/specs/momo.md` (Constraints section) where they apply to this plan:

- "Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum."
- "SQLite driver: mattn/go-sqlite3 v1.14.52 (cgo) with sqlite-vec v0.1.6 statically linked via github.com/asg017/sqlite-vec-go-bindings/cgo; SQLite builds require cgo and a C toolchain. The database/sql adapter itself stays driver-agnostic."
- "PostgreSQL driver: jackc/pgx v5 (pinned) via pgxpool, plus pgx stdlib for *sql.DB adoption."
- "MySQL/MariaDB: github.com/go-sql-driver/mysql (pinned); supported servers MySQL 8.4 LTS, MySQL 9.7 LTS and MariaDB 11.8 LTS (no innovation releases). Vector capability: MariaDB 11.8 full search with VECTOR INDEX; MySQL 9.7 VECTOR storage only, distance queries return ErrUnsupported (DISTANCE() is HeatWave/MySQL AI only); MySQL 8.4 has no vector columns; CI uses official MySQL and MariaDB service containers."
- "PostgreSQL: supported versions to be recorded (tested in CI against real PostgreSQL with pgvector, image `pgvector/pgvector:pg17` locally available; plus one older major)."
- "SQLite: >= 3.35 (RETURNING); the adapter checks the library version on connect."
- "Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module."
- "Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled."
- "No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state."
- "License: Apache-2.0."
- "Module path: github.com/azrtydxb/momo; public repository https://github.com/azrtydxb/momo."
- "CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners. Benchmarks published as release evidence are run on a documented dedicated machine, because shared runners are too noisy for regression gating; CI only smoke-runs benchmarks."
- "Versioning: standard Go module SemVer. v0.x carries no compatibility promise (breaking changes bump the minor, fixes the patch); v1.0.0 is cut when the API is declared stable; after v1, minor = backward-compatible features, patch = fixes, and any breaking change requires v2.0.0 with a `/v2` module path suffix. The first tag, v0.1.0, is cut only when all delivery stages 1-9 are complete; intermediate milestones are untagged."
- "Release automation: release-please (GitHub Action, free runners) reads Conventional Commits on main, maintains a release PR with the version bump and CHANGELOG.md, and tags vX.Y.Z on merge; pre-1.0 it is configured so `feat!`/BREAKING CHANGE bumps the minor, not the major. Commits on main follow Conventional Commits (enforced on PR titles in CI)."
- "Persisted formats carry explicit format versions independent of the module version: schema snapshot JSON (`format_version`), the checksum file header, the `momo_migrations` history table (metadata row), and generated-file headers (generator version). A newer Momo reads every older format; an older Momo refuses a newer format with an actionable error instead of misreading it."

Contract rules from docs/ARCHITECTURE.md that this plan enforces:

- Core packages (`momo`, `dialect`, `schema`, `vector`, `types`) import only the standard library and each other.
- Generated code begins with `// Code generated by momo`, so the SPDX header check exempts generated files.
- Names are exactly those in docs/ARCHITECTURE.md. Where the contract is silent, this plan states the choice and the report lists it as a contract gap.

Pinned versions, verified 2026-11-04 (each published at least two weeks earlier):

| Item                                               | Pin                                                                                     |
| -------------------------------------------------- | --------------------------------------------------------------------------------------- |
| actions/checkout                                   | `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1`                    |
| actions/setup-go                                   | `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0`                    |
| actions/upload-artifact                            | `actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2`             |
| github/codeql-action (init, analyze, upload-sarif) | `github/codeql-action/<sub>@9b3254085494b55a0f7c4c2b3d2d52cc712ce370 # v4.39.1`         |
| securego/gosec                                     | `securego/gosec@deb54465fea23d19a77f037e11e6589021f8501d # v2.29.0`                     |
| golang/govulncheck-action                          | `golang/govulncheck-action@032d45514ae346b1db93c04b0c90b841c370344f # v1.1.0`           |
| ossf/scorecard-action                              | `ossf/scorecard-action@fa7117db83c52d787ae5a89f350999701505d78e # v2.5.0`               |
| googleapis/release-please-action                   | `googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7 # v5.0.0`    |
| amannn/action-semantic-pull-request                | `amannn/action-semantic-pull-request@48f256284bd46cdaab1048c3721360e808335d50 # v6.1.1` |
| govulncheck (Makefile)                             | `golang.org/x/vuln/cmd/govulncheck@v1.9.0`                                              |
| jackc/pgx/v5                                       | `v5.11.0` (v5.12.0 was published 2026-10-31 and is excluded)                            |
| go-sql-driver/mysql                                | `v1.10.1`                                                                               |
| go.yaml.in/yaml/v3                                 | `v3.0.6` (test-only, in internal/repocheck)                                             |
| PostgreSQL + pgvector                              | `pgvector/pgvector:0.8.7-pg17`, `pgvector/pgvector:0.8.7-pg16`                          |
| MySQL 8.4 LTS                                      | `mysql:8.4.13`                                                                          |
| MySQL 9.7 LTS                                      | `mysql:9.7.4`                                                                           |
| MariaDB 11.8 LTS                                   | `mariadb:11.8.9`                                                                        |
| PgBouncer                                          | `edoburu/pgbouncer:v1.26.1-p0`                                                          |

MySQL versions follow the answers recorded in `.procoder/ask/answers.md` (key `mysql-versions`): MySQL 8.4 LTS, MySQL 9.7 LTS and MariaDB 11.8 LTS, with no innovation releases.

## Task 1: Repository, licence and community files

Delivers: S-24 (`TestRepoFiles`). Also records the SPDX header rule every later Go file follows.

Files:

- Create: `internal/repocheck/repo_test.go` (test helper `repoRoot` and `TestRepoFiles`)
- Create: `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`
- Create: `.github/ISSUE_TEMPLATE/bug_report.yml`, `.github/ISSUE_TEMPLATE/feature_request.yml`, `.github/ISSUE_TEMPLATE/config.yml`, `.github/PULL_REQUEST_TEMPLATE.md`
- Create: `.gitignore`
- Modify: every existing `*.go` file in the repository root, `dialect/` and `schema/` (prepend the SPDX header, see step 5)

Interfaces:

- `func repoRoot(t *testing.T) string` in package `repocheck` returns the module root (two directories above the package directory).
- Every non-generated `.go` file must contain `SPDX-License-Identifier: Apache-2.0` within its first 600 bytes, or start with `// Code generated by momo`.

- [ ] **Step 1: Write the failing test.** Create `internal/repocheck/repo_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const spdxTag = "SPDX-License-Identifier: Apache-2.0"

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func TestRepoFiles(t *testing.T) {
	root := repoRoot(t)
	for _, p := range []string{
		"LICENSE", "SECURITY.md", "CONTRIBUTING.md", "CODE_OF_CONDUCT.md",
		".github/ISSUE_TEMPLATE/bug_report.yml",
		".github/ISSUE_TEMPLATE/feature_request.yml",
		".github/ISSUE_TEMPLATE/config.yml",
		".github/PULL_REQUEST_TEMPLATE.md",
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	lic, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(lic), "Apache License") || !strings.Contains(string(lic), "Version 2.0, January 2004") {
		t.Error("LICENSE is not the Apache-2.0 text")
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", ".procoder":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(string(b), "// Code generated by momo") {
			return nil
		}
		head := string(b)
		if len(head) > 600 {
			head = head[:600]
		}
		if !strings.Contains(head, spdxTag) {
			t.Errorf("%s: missing SPDX header", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run it and expect FAIL.** `go test ./internal/repocheck/ -run '^TestRepoFiles$' -count=1` fails with `missing LICENSE` (and the other missing community files) and with `missing SPDX header` for `builder.go`.

- [ ] **Step 3: Create the licence and community files.**
  - `LICENSE`: run `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE`, then `grep -c "Version 2.0, January 2004" LICENSE` prints `1`.
  - `CODE_OF_CONDUCT.md`: run `curl -fsSL https://www.contributor-covenant.org/version/2/1/code_of_conduct/code_of_conduct.md -o CODE_OF_CONDUCT.md`; then replace the enforcement contact placeholder with the GitHub private reporting link `https://github.com/azrtydxb/momo/security/advisories/new` (the project has no contact mailbox).
  - `SECURITY.md`: a section "Reporting a vulnerability" telling reporters to use GitHub private vulnerability reporting at `https://github.com/azrtydxb/momo/security/advisories/new`, not public issues; a supported-versions table listing only the latest minor; an acknowledgement target of 7 days.
  - `CONTRIBUTING.md`: prerequisites (Go 1.26 or later, a C toolchain for the SQLite packages, Docker for `make db-up`); the commands `make test`, `make test-race`, `make test-compilefail` and `make test-integration`; the rule that commit messages and PR titles use Conventional Commits (`feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`, `revert`), with `feat!:` or a `BREAKING CHANGE:` footer for breaking changes; the rule that every new `.go` file starts with the two-line SPDX header shown in step 5; the dialect rule from Task 2.
  - `.github/ISSUE_TEMPLATE/bug_report.yml`: an issue form named `Bug report` with body fields `version` (input, required), `dialect` (dropdown: PostgreSQL, SQLite, MySQL, MariaDB, Other), `what happened` (textarea, required) and `minimal reproduction` (textarea, required).
  - `.github/ISSUE_TEMPLATE/feature_request.yml`: named `Feature request`, body fields `problem` (textarea, required) and `proposed API` (textarea).
  - `.github/ISSUE_TEMPLATE/config.yml`: `blank_issues_enabled: false` and one `contact_links` entry named `Security vulnerability` pointing at `https://github.com/azrtydxb/momo/security/advisories/new` with the text `Report security issues privately.`
  - `.github/PULL_REQUEST_TEMPLATE.md`: a checklist with the items `- [ ] PR title follows Conventional Commits`, `- [ ] Tests added or updated`, `- [ ] docs/ updated when behaviour changed`, `- [ ] make test-race passes locally`.
  - `.gitignore`: exactly these lines: `/bin/`, `*.test`, `*.out`, `coverage.*`, `/vendor/`, `.DS_Store`, `*.db`, `*.db-wal`, `*.db-shm`.

- [ ] **Step 4: Confirm the community files pass.** Run `go test ./internal/repocheck/ -run '^TestRepoFiles$' -count=1`. Expected: the only remaining failures are `missing SPDX header` lines.

- [ ] **Step 5: Add the SPDX header to every existing Go file.** Run `for f in *.go dialect/*.go schema/*.go; do printf '// SPDX-License-Identifier: Apache-2.0\n\n' | cat - "$f" > "$f.tmp" && mv "$f.tmp" "$f"; done`, then `gofmt -l .` must print nothing. Every new Go file created by later tasks starts with the same two lines followed by a blank line before `package`.

- [ ] **Step 6: Run the test and expect PASS.** `go test ./internal/repocheck/ -run '^TestRepoFiles$' -count=1` prints `ok`.

- [ ] **Step 7: Commit.** `git add LICENSE SECURITY.md CONTRIBUTING.md CODE_OF_CONDUCT.md .github .gitignore internal/repocheck *.go dialect schema && git commit -m "chore: add licence, community files and SPDX headers"`.

## Task 2: Makefile and local command parity

Delivers: S-20 (the local commands CI calls) and S-25 (govulncheck pinned in one place). Milestone M1 (PostgreSQL). SQLite and MySQL targets are added by their milestone plans with the same pattern.

Files:

- Create: `Makefile`
- Create: `internal/repocheck/makefile_test.go`
- Modify: `CONTRIBUTING.md` (one paragraph, step 5)

Interfaces: make targets used by CI: `test`, `test-race`, `test-compilefail`, `test-fuzz`, `test-integration`, `test-integration-postgres`, `bench-smoke`, `fmt-check`, `vet`, `tidy-check`, `govulncheck`, `db-up`, `db-down`. Variables: `FUZZTIME ?= 10s`; `FUZZ_TARGETS = .:FuzzBuilder ./schema:FuzzSchemaSort ./schema:FuzzNormalizeType`; `PG_URL`, `PG_APP_URL`, `PGBOUNCER_URL` default to the compose services of Task 18. Dialect coverage is expressed by subtests from `testopen.Each` (momo-03), never by test-name suffixes; `test-integration-<dialect>` sets that dialect's URL variables and runs every test with `MOMO_REQUIRE_INTEGRATION=1` and the command `go test -tags integration -race -count=1 ./...` (the `integration` build tag is optional per test file; the harness skip/require rule is the gate, and CI and the Makefile always pass the tag and the race detector).

- [ ] **Step 1: Write the failing test.** Create `internal/repocheck/makefile_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os/exec"
	"testing"
)

func TestMakefileTargets(t *testing.T) {
	targets := []string{
		"test", "test-race", "test-compilefail", "test-fuzz", "test-integration",
		"test-integration-postgres", "bench-smoke", "fmt-check", "vet", "tidy-check",
		"govulncheck", "db-up", "db-down",
	}
	for _, target := range targets {
		cmd := exec.Command("make", "-n", target)
		cmd.Dir = repoRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("make -n %s: %v\n%s", target, err, out)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/repocheck/ -run '^TestMakefileTargets$' -count=1` fails with `make -n test: exit status 2` and `No rule to make target 'test'`.

- [ ] **Step 3: Write the Makefile.** Recipe lines use tab indentation.

```make
# SPDX-License-Identifier: Apache-2.0
SHELL := /bin/sh
GO ?= go
FUZZTIME ?= 10s
FUZZ_TARGETS = .:FuzzBuilder ./schema:FuzzSchemaSort ./schema:FuzzNormalizeType
GOVULNCHECK = go run golang.org/x/vuln/cmd/govulncheck@v1.9.0
PG_URL ?= postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable
PG_APP_URL ?= postgres://momo_app:momo_app@127.0.0.1:5432/postgres?sslmode=disable
PGBOUNCER_URL ?= postgres://postgres:momo@127.0.0.1:6432/postgres?sslmode=disable

.PHONY: test test-race test-compilefail test-fuzz test-integration test-integration-postgres bench-smoke fmt-check vet tidy-check govulncheck db-up db-down

test:
	$(GO) test -count=1 ./...

test-race:
	$(GO) test -race -count=1 ./...

test-compilefail:
	$(GO) test -count=1 -run '^TestCompileFail' .

test-fuzz:
	@for t in $(FUZZ_TARGETS); do \
		pkg=$${t%%:*}; fn=$${t##*:}; \
		$(GO) test -run='^$$' -fuzz="^$$fn$$" -fuzztime=$(FUZZTIME) $$pkg || exit 1; \
	done

test-integration: test-integration-postgres

test-integration-postgres:
	MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL='$(PG_URL)' MOMO_TEST_POSTGRES_APP_URL='$(PG_APP_URL)' MOMO_TEST_PGBOUNCER_URL='$(PGBOUNCER_URL)' $(GO) test -tags integration -race -count=1 ./...

bench-smoke:
	$(GO) test -run='^$$' -bench=. -benchtime=1x ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

vet:
	$(GO) vet ./...

tidy-check:
	$(GO) mod tidy -diff

govulncheck:
	$(GOVULNCHECK) ./...

db-up:
	docker compose -f internal/testdb/compose.yaml up -d --wait

db-down:
	docker compose -f internal/testdb/compose.yaml down -v
```

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -run '^TestMakefileTargets$' -count=1` prints `ok`; `make fmt-check vet` exits 0.

- [ ] **Step 5: Document the dialect rule and commit.** Add to `CONTRIBUTING.md`: "Dialect coverage is expressed as subtests from `testopen.Each`, one per dialect built into this version; integration tests have no name suffix. `make test-integration-<dialect>` sets that dialect's URL variables and runs the whole suite." Then `git add Makefile internal/repocheck/makefile_test.go CONTRIBUTING.md && git commit -m "build: add Makefile with CI-parity targets"`.

## Task 3: Dialect: full capability set, flavours and quoting

Delivers: S-4 (capability booleans gate clauses), S-31 (per-dialect placeholders and quoting), contract `dialect` section.

Files:

- Modify: `dialect/dialect.go` (rewrite to the contract)
- Create: `dialect/dialect_test.go`

Interfaces (produced, exact):

- `type Name string`; `const Postgres Name = "postgres"`, `SQLite Name = "sqlite"`, `MySQL Name = "mysql"`.
- `type Flavor string`; `const FlavorMySQL Flavor = "mysql"`, `FlavorMariaDB Flavor = "mariadb"`.
- `type Dialect struct` with fields `Name Name`, `Flavor Flavor`, `MaxParams int`, and boolean fields `Returning, OnConflict, OnDuplicateKey, Locking, NullsOrdering, DefaultInUpdate, DefaultInValues, RecursiveCTE, RowValues, Savepoints, IsolationLevels, TransactionalDDL, ILike, Merge, WindowFunctions bool`.
- `func For(n Name) *Dialect` (nil for unknown names); `func MySQLFlavor(f Flavor) *Dialect`.
- Methods on `*Dialect`: `AppendPlaceholder(b []byte, n int) []byte`, `AppendIdent(b []byte, ident string) []byte`, `QuoteIdent(ident string) string`.

- [ ] **Step 1: Write the failing tests.** Create `dialect/dialect_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package dialect

import "testing"

func TestDialectCapabilities(t *testing.T) {
	pg := For(Postgres)
	if pg == nil || pg.MaxParams != 65535 || !pg.Returning || !pg.Locking || !pg.Merge || !pg.WindowFunctions {
		t.Fatalf("postgres capabilities wrong: %+v", pg)
	}
	sq := For(SQLite)
	if sq == nil || sq.MaxParams != 32766 || !sq.Returning || sq.Locking || sq.ILike || sq.Merge {
		t.Fatalf("sqlite capabilities wrong: %+v", sq)
	}
	if For("oracle") != nil {
		t.Fatal("For must return nil for unknown names")
	}
	my := MySQLFlavor(FlavorMySQL)
	if my.Name != MySQL || my.Flavor != FlavorMySQL || my.MaxParams != 65535 {
		t.Fatalf("mysql flavor wrong: %+v", my)
	}
	if !my.OnDuplicateKey || my.OnConflict || my.NullsOrdering || my.ILike {
		t.Fatalf("mysql capability booleans wrong: %+v", my)
	}
	if MySQLFlavor(FlavorMariaDB).Flavor != FlavorMariaDB {
		t.Fatal("MariaDB flavour not recorded")
	}
}

func TestAppendIdentQuoting(t *testing.T) {
	cases := []struct {
		d    *Dialect
		in   string
		want string
	}{
		{For(Postgres), `order`, `"order"`},
		{For(Postgres), `a"b`, `"a""b"`},
		{For(SQLite), `select`, `"select"`},
		{MySQLFlavor(FlavorMySQL), `order`, "`order`"},
		{MySQLFlavor(FlavorMySQL), "a`b", "`a``b`"},
		{MySQLFlavor(FlavorMariaDB), "a`b", "`a``b`"},
	}
	for _, c := range cases {
		if got := c.d.QuoteIdent(c.in); got != c.want {
			t.Errorf("%s QuoteIdent(%q) = %s, want %s", c.d.Name, c.in, got, c.want)
		}
	}
}

func TestPlaceholderStyle(t *testing.T) {
	if got := string(For(Postgres).AppendPlaceholder(nil, 3)); got != "$3" {
		t.Errorf("postgres placeholder = %q, want $3", got)
	}
	for _, d := range []*Dialect{For(SQLite), MySQLFlavor(FlavorMySQL)} {
		if got := string(d.AppendPlaceholder(nil, 3)); got != "?" {
			t.Errorf("%s placeholder = %q, want ?", d.Name, got)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./dialect/ -count=1` fails with `undefined: MySQL`, `undefined: MySQLFlavor` and `undefined: Flavor`.

- [ ] **Step 3: Implement.** Rewrite `dialect/dialect.go`, keeping the spike's `AppendPlaceholder` and `AppendIdent` logic and adding:
  - the `Flavor` type and constants, and the `MySQL` name constant;
  - the full field list above;
  - the `postgres` value: `MaxParams 65535`; Returning, OnConflict, Locking, NullsOrdering, DefaultInUpdate, DefaultInValues, RecursiveCTE, RowValues, Savepoints, IsolationLevels, TransactionalDDL, ILike, Merge and WindowFunctions true; OnDuplicateKey false;
  - the `sqlite` value: `MaxParams 32766`; Returning, OnConflict, NullsOrdering, RecursiveCTE, RowValues, Savepoints, TransactionalDDL, DefaultInValues and WindowFunctions true; Locking, ILike, Merge, IsolationLevels and DefaultInUpdate false;
  - `func MySQLFlavor(f Flavor) *Dialect` returning a pointer to a new value: `Name MySQL`, the given `Flavor`, `MaxParams 65535`; OnDuplicateKey, Locking, DefaultInUpdate, DefaultInValues, RecursiveCTE, RowValues, Savepoints, IsolationLevels and WindowFunctions true; Returning, OnConflict, NullsOrdering, ILike, Merge and TransactionalDDL false. Add a comment: "MariaDB RETURNING is enabled by the MySQL plan after version verification (S-59)."
  - `AppendIdent`: when `d.Name == MySQL`, quote with backticks and double embedded backticks; otherwise keep the double-quote rule.
  - `AppendPlaceholder`: `$n` only for Postgres.
  - `For(MySQL)` returns the MySQL flavour value with `Flavor FlavorMySQL`, so `For` is total over the three names.

- [ ] **Step 4: Run and expect PASS.** `go test ./dialect/ -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add dialect && git commit -m "feat(dialect): add MySQL flavours and the full capability set"`.

## Task 4: Schema base types, with JSON round-trip

Delivers: S-1 base types and `FormatVersion`. The managed-object types (`Namespace`, `View`, `Function`, `Trigger`, `Policy`, partitions, RLS flags) and the schema-qualified `Table.Schema` belong to momo-04 Task 1, not to this plan.

Files:

- Modify: `schema/schema.go`
- Create: `schema/schema_test.go`

Interfaces (produced, exact):

- `const FormatVersion = 1`.
- `type Schema struct { FormatVersion int \`json:"format_version"\`; Dialect dialect.Name \`json:"dialect"\`; Extensions []Extension \`json:"extensions,omitempty"\`; Enums []Enum \`json:"enums,omitempty"\`; Tables []Table \`json:"tables"\` }`.
- The spike types `Table`, `Column`, `Unique`, `Index`, `IndexPart`, `KV`, `ForeignKey`, `Check`, `Extension`, `Enum` keep their fields and json tags, with `Table` unchanged (no `Schema` field).
- Methods `(*Schema).Table(name string) (*Table, bool)` and `(*Table).Column(name string) (*Column, bool)` keep their spike form.

- [ ] **Step 1: Write the failing test.** Create `schema/schema_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func populated() *Schema {
	return &Schema{
		FormatVersion: FormatVersion,
		Dialect:       dialect.Postgres,
		Extensions:    []Extension{{Name: "vector", Version: "0.8.7"}},
		Enums:         []Enum{{Name: "status", Values: []string{"open", "closed"}}},
		Tables: []Table{{
			Name: "documents",
			Columns: []Column{
				{Name: "id", Type: "bigint", Identity: "by_default"},
				{Name: "embedding", Type: "vector(3)", Nullable: true, Default: "NULL"},
				{Name: "gen", Type: "integer", Generated: "(id * 2)"},
				{Name: "st", Type: "status", Enum: "status"},
			},
			PrimaryKey:   []string{"id"},
			Uniques:      []Unique{{Name: "documents_u", Columns: []string{"st"}}},
			Indexes:      []Index{{Name: "documents_i", Unique: true, Method: "hnsw", Where: "st IS NOT NULL", With: []KV{{Key: "m", Value: "16"}}, Concurrently: true, Parts: []IndexPart{{Column: "embedding", OpClass: "vector_cosine_ops", Desc: true, Nulls: "last"}, {Expr: "lower(st)"}}}},
			ForeignKeys:  []ForeignKey{{Name: "documents_fk", Columns: []string{"id"}, RefTable: "owners", RefColumns: []string{"id"}, OnDelete: "CASCADE", OnUpdate: "NO ACTION"}},
			Checks:       []Check{{Name: "documents_c", Expr: "id > 0"}},
			WithoutRowID: true,
		}},
	}
}

func TestSchemaJSONRoundTrip(t *testing.T) {
	s := populated()
	first, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Schema
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(&back)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("round trip changed the JSON:\n%s\n%s", first, second)
	}
	if !reflect.DeepEqual(s, &back) {
		t.Fatal("round trip changed the value")
	}
}

// TestSchemaEveryFieldSerialized walks the type tree and fails when a field
// has no json tag, since it would be silently dropped from snapshots.
func TestSchemaEveryFieldSerialized(t *testing.T) {
	var walk func(reflect.Type, string)
	walk = func(rt reflect.Type, path string) {
		if rt.Kind() == reflect.Slice {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			if tag == "" || tag == "-" {
				t.Errorf("%s.%s has no json tag", path, f.Name)
				continue
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(Schema{}), "Schema")
}

func TestSchemaFieldSet(t *testing.T) {
	b, err := json.Marshal(populated())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"format_version":1`, `"extensions"`, `"enums"`, `"without_rowid":true`} {
		if !strings.Contains(string(b), key) {
			t.Errorf("snapshot JSON lacks %s", key)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./schema/ -count=1` fails with `undefined: FormatVersion` and `s.FormatVersion undefined`.

- [ ] **Step 3: Implement.** Edit `schema/schema.go`: import `github.com/azrtydxb/momo/dialect`; add `const FormatVersion = 1`; set the `Schema` struct to the field list above with the json tags shown. Keep the spike's doc comments.

- [ ] **Step 4: Run and expect PASS.** `go test ./schema/ -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add schema && git commit -m "feat(schema): base snapshot types with format_version and JSON round-trip"`.

## Task 5: Snapshot format versions and refusal of newer formats

Delivers: S-22 (`TestFormatVersions`, snapshot part). The checksum-file and history-table parts are added by their own plans; this task creates the reader they reuse.

Files:

- Create: `schema/load.go`
- Create: `schema/load_test.go`

Interfaces (produced, exact):

- `func Load(r io.Reader) (*Schema, error)`.
- `var ErrNewerFormat = errors.New("schema: snapshot format is newer than this momo")`.
- `var ErrMissingFormatVersion = errors.New("schema: snapshot has no format_version")`.
- Readers are registered in `var readers = map[int]func([]byte) (*Schema, error){1: decodeV1}`.

- [ ] **Step 1: Write the failing test.** Create `schema/load_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFormatVersions(t *testing.T) {
	cur, err := json.Marshal(populated())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(bytes.NewReader(cur)); err != nil {
		t.Fatalf("current format rejected: %v", err)
	}

	future := bytes.Replace(cur, []byte(`"format_version":1`), []byte(`"format_version":2`), 1)
	_, err = Load(bytes.NewReader(future))
	if !errors.Is(err, ErrNewerFormat) {
		t.Fatalf("format 2 error = %v, want ErrNewerFormat", err)
	}
	if !strings.Contains(err.Error(), "format_version 2") {
		t.Errorf("error does not name the format: %v", err)
	}

	missing := bytes.Replace(cur, []byte(`"format_version":1,`), []byte(``), 1)
	if _, err := Load(bytes.NewReader(missing)); !errors.Is(err, ErrMissingFormatVersion) {
		t.Fatalf("missing format_version error = %v, want ErrMissingFormatVersion", err)
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./schema/ -run '^TestFormatVersions$' -count=1` fails with `undefined: Load`.

- [ ] **Step 3: Implement `schema/load.go`.** `Load` reads all bytes with `io.ReadAll`, unmarshals a header `struct{ FormatVersion int \`json:"format_version"\` }`, and returns `ErrMissingFormatVersion`when the value is 0. When the value is greater than`FormatVersion`it returns`fmt.Errorf("%w: format_version %d; this momo reads format_version %d and earlier, upgrade momo to read it", ErrNewerFormat, v, FormatVersion)`. Otherwise it calls `readers[v]`, returning `fmt.Errorf("schema: no reader for format_version %d", v)`when the entry is absent.`decodeV1`unmarshals into`Schema`and calls`s.Sort()`.

- [ ] **Step 4: Run and expect PASS.** `go test ./schema/ -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add schema && git commit -m "feat(schema): read snapshots by format version and refuse newer formats"`.

## Task 6: Normalization of types, expressions and schema equality

Delivers: S-1 normalization, including the edge case "normalization of DB-rewritten expressions so round-trip diffs are empty".

Files:

- Create: `schema/normalize.go`
- Create: `schema/normalize_test.go`

Interfaces (produced, exact):

- `func NormalizeType(d dialect.Name, t string) string`
- `func NormalizeExpr(d dialect.Name, e string) string`
- `func Equal(a, b *Schema) bool`

- [ ] **Step 1: Write the failing tests.** Create `schema/normalize_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestNormalizeType(t *testing.T) {
	cases := []struct {
		d       dialect.Name
		in, out string
	}{
		{dialect.Postgres, "INT4", "integer"},
		{dialect.Postgres, "int", "integer"},
		{dialect.Postgres, "int8", "bigint"},
		{dialect.Postgres, "bool", "boolean"},
		{dialect.Postgres, "varchar(255)", "character varying(255)"},
		{dialect.Postgres, "timestamptz", "timestamp with time zone"},
		{dialect.Postgres, "vector( 3 )", "vector(3)"},
		{dialect.Postgres, "  Text  ", "text"},
		{dialect.SQLite, "INTEGER", "integer"},
		{dialect.SQLite, "varchar(20)", "varchar(20)"},
	}
	for _, c := range cases {
		if got := NormalizeType(c.d, c.in); got != c.out {
			t.Errorf("NormalizeType(%s, %q) = %q, want %q", c.d, c.in, got, c.out)
		}
	}
}

func TestNormalizeExpr(t *testing.T) {
	cases := []struct {
		d       dialect.Name
		in, out string
	}{
		{dialect.Postgres, "((now()))", "now()"},
		{dialect.Postgres, "  COALESCE( a ,  0 )  ", "coalesce( a , 0 )"},
		{dialect.Postgres, "'Mixed CASE'::text", "'Mixed CASE'::text"},
		{dialect.Postgres, `"Quoted" > 0`, `"Quoted" > 0`},
		{dialect.SQLite, "(a > 0)", "a > 0"},
	}
	for _, c := range cases {
		if got := NormalizeExpr(c.d, c.in); got != c.out {
			t.Errorf("NormalizeExpr(%s, %q) = %q, want %q", c.d, c.in, got, c.out)
		}
	}
}

func TestEqualIgnoresOrderAndConcurrently(t *testing.T) {
	a := populated()
	b := populated()
	b.Tables[0].Indexes[0].Concurrently = false
	a.Tables[0].Indexes[0].Concurrently = true
	b.Tables[0].Columns[0].Type = "BIGINT"
	if !Equal(a, b) {
		t.Fatal("schemas differing only in Concurrently and type case must be equal")
	}
	c := populated()
	c.Tables[0].Columns[0].Type = "text"
	if Equal(a, c) {
		t.Fatal("schemas with different column types must not be equal")
	}
}

func FuzzNormalizeType(f *testing.F) {
	f.Add("int4")
	f.Add("varchar(255)")
	f.Fuzz(func(t *testing.T, s string) {
		once := NormalizeType(dialect.Postgres, s)
		if NormalizeType(dialect.Postgres, once) != once {
			t.Fatalf("NormalizeType is not idempotent for %q", s)
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./schema/ -run 'Normalize|Equal' -count=1` fails with `undefined: NormalizeType`, `undefined: NormalizeExpr` and `undefined: Equal`.

- [ ] **Step 3: Implement `schema/normalize.go`.**
  - `NormalizeType`: trim, lowercase, collapse runs of spaces, remove spaces inside parentheses. For `dialect.Postgres`, map the base name (the text before any `(`) through this table: `int`, `int4`, `integer` to `integer`; `int8`, `bigint` to `bigint`; `int2`, `smallint` to `smallint`; `bool`, `boolean` to `boolean`; `varchar` to `character varying`; `timestamptz` to `timestamp with time zone`; `timestamp` to `timestamp without time zone`; `float8`, `double precision` to `double precision`; `float4`, `real` to `real`. Keep the parenthesized suffix. For `dialect.SQLite` and `dialect.MySQL`, only lowercasing and whitespace are normalized.
  - `NormalizeExpr`: trim; collapse runs of whitespace to one space outside quoted text (single and double quotes); lowercase characters outside quoted text; then, while the whole expression is enclosed in one matching pair of parentheses, remove that pair and trim.
  - `Equal(a, b)`: deep-copy both schemas with a JSON round trip, zero every `Indexes[i].Concurrently`, apply `NormalizeType` to every column type using the schema's dialect, apply `NormalizeExpr` to defaults, generated columns, checks, index predicates and index part expressions, call `Sort`, marshal both and compare the bytes.

- [ ] **Step 4: Run and expect PASS.** `go test ./schema/ -run 'Normalize|Equal' -count=1` prints `ok`, and `go test ./schema/ -run '^$' -fuzz '^FuzzNormalizeType$' -fuzztime=10s` completes without a failure.

- [ ] **Step 5: Commit.** `git add schema && git commit -m "feat(schema): normalize types and expressions and compare schemas canonically"`.

## Task 7: Canonical Sort for every base collection

Delivers: S-1 (`FuzzSchemaSort`, `TestSortCoversAllCollections`) for the base collections. Sorting of managed-object collections is added with those types in momo-04 Task 1.

Files:

- Verify (no change expected): `schema/schema.go` (the `Sort` method already landed with the spike in commit 586d784)
- Create: `schema/sort_test.go`

Interfaces (produced, exact): `func (s *Schema) Sort()` sorts `Extensions` and `Enums` by `Name`, `Tables` by `Name`, and within each table `Uniques`, `Indexes`, `ForeignKeys` and `Checks` by `Name`. Column order is preserved and is not sorted.

- [ ] **Step 1: Write the failing test.** Create `schema/sort_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package schema

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"testing"
)

func TestSortCoversAllCollections(t *testing.T) {
	s := &Schema{
		Extensions: []Extension{{Name: "vector"}, {Name: "citext"}},
		Enums:      []Enum{{Name: "b"}, {Name: "a"}},
		Tables: []Table{{
			Name:    "t",
			Uniques: []Unique{{Name: "u2"}, {Name: "u1"}},
			Checks:  []Check{{Name: "c2"}, {Name: "c1"}},
			Indexes: []Index{{Name: "i2"}, {Name: "i1"}},
			ForeignKeys: []ForeignKey{{Name: "f2"}, {Name: "f1"}},
		}, {Name: "a"}},
	}
	s.Sort()
	tb := s.Tables[1]
	if s.Extensions[0].Name != "citext" || s.Enums[0].Name != "a" || s.Tables[0].Name != "a" ||
		tb.Uniques[0].Name != "u1" || tb.Checks[0].Name != "c1" || tb.Indexes[0].Name != "i1" || tb.ForeignKeys[0].Name != "f1" {
		t.Fatalf("Sort left a collection unsorted: %+v", s)
	}
}

// FuzzSchemaSort: shuffling the collections of a schema and then sorting it
// must give the same bytes as sorting the original.
func FuzzSchemaSort(f *testing.F) {
	f.Add(int64(1))
	f.Add(int64(42))
	f.Fuzz(func(t *testing.T, seed int64) {
		r := rand.New(rand.NewSource(seed))
		base := populated()
		for i := 0; i < 4; i++ {
			base.Tables = append(base.Tables, Table{Name: string(rune('a' + i)), Columns: []Column{{Name: "c", Type: "int"}}})
		}
		shuffled := populated()
		for i := 0; i < 4; i++ {
			shuffled.Tables = append(shuffled.Tables, base.Tables[len(base.Tables)-4+i])
		}
		r.Shuffle(len(shuffled.Tables), func(i, j int) { shuffled.Tables[i], shuffled.Tables[j] = shuffled.Tables[j], shuffled.Tables[i] })
		r.Shuffle(len(shuffled.Extensions), func(i, j int) { shuffled.Extensions[i], shuffled.Extensions[j] = shuffled.Extensions[j], shuffled.Extensions[i] })

		base.Sort()
		shuffled.Sort()
		a, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("Sort is order-sensitive:\n%s\n%s", a, b)
		}
	})
}
```

- [ ] **Step 2: Run, then prove the test can fail (mutation check).** The spike's `Sort` already sorts every collection, so `go test ./schema/ -run 'TestSortCoversAllCollections' -count=1` passes immediately. Prove the test catches a break: delete the `Checks` sort from `Sort`, rerun, and expect FAIL with `Sort left a collection unsorted`; then restore it with `git checkout schema/schema.go`. Repeat once for the table-level sort (`Tables`). Record both FAIL outputs as evidence.

- [ ] **Step 3: Implement only what is missing.** If Step 1's test or the fuzz target finds a collection the spike's `Sort` does not order, extend `Sort` with `slices.SortFunc` by `Name`, using `cmp.Compare` for strings. Otherwise `schema/schema.go` stays unchanged.

- [ ] **Step 4: Run and expect PASS.** `go test ./schema/ -count=1` prints `ok`; `go test ./schema/ -run '^$' -fuzz '^FuzzSchemaSort$' -fuzztime=10s` completes without failure.

- [ ] **Step 5: Commit.** `git add schema && git commit -m "test(schema): cover canonical sort for every base collection"`.

## Task 8: Error sentinels, structured Error and sibling sentinels

Delivers: S-8 (`TestErrorSentinels`, `TestErrorIsAs`, `TestMigrateSentinelMessages`). Driver classification (`TestErrorClassification`) and redaction (`TestErrorRedaction`) belong to the driver plans. `Error()` keeps the spike behaviour of appending the wrapped error text, and `Detail()` returns the same text.

Files:

- Modify: `errors.go`
- Create: `errors_test.go` (package `momo_test`)
- Create: `vector/errors.go`, `vector/errors_test.go`
- Create: `migrate/errors.go`, `migrate/errors_test.go`

Interfaces (produced, exact):

- In `momo` (`errors.go`): sentinels `ErrNotFound`, `ErrTooManyRows`, `ErrUniqueViolation`, `ErrForeignKeyViolation`, `ErrCheckViolation`, `ErrNotNullViolation`, `ErrSerialization`, `ErrOptimisticLock`, `ErrUnsupported`, `ErrUnfilteredMutation`, `ErrTxDone`, `ErrValidation`, `ErrBusy`, `ErrLimitExceeded`, and `ErrInvalidVector = vector.ErrInvalidVector`. `type Error struct{ Kind error; Op, Table, Column, Constraint string; Err error }`; methods `Error() string`, `Is(target error) bool`, `Unwrap() error`, and `Detail() string` (returns `e.Err.Error()`, or `""` when `Err` is nil).
- In `vector`: `var ErrInvalidVector = errors.New("vector: invalid vector")`.
- In `migrate`: `ErrChecksumMismatch`, `ErrDirty`, `ErrLocked`, `ErrDestructive`, `ErrBranchConflict`.

- [ ] **Step 1: Write the failing tests.** Create `errors_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/vector"
)

var allSentinels = []error{
	momo.ErrNotFound, momo.ErrTooManyRows, momo.ErrUniqueViolation,
	momo.ErrForeignKeyViolation, momo.ErrCheckViolation, momo.ErrNotNullViolation,
	momo.ErrSerialization, momo.ErrOptimisticLock, momo.ErrUnsupported,
	momo.ErrUnfilteredMutation, momo.ErrTxDone, momo.ErrValidation, momo.ErrBusy,
	momo.ErrLimitExceeded, momo.ErrInvalidVector,
}

func TestErrorSentinels(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range allSentinels {
		if s == nil {
			t.Fatal("nil sentinel")
		}
		if seen[s.Error()] {
			t.Errorf("duplicate sentinel message %q", s.Error())
		}
		seen[s.Error()] = true
	}
	if momo.ErrInvalidVector != vector.ErrInvalidVector {
		t.Fatal("momo.ErrInvalidVector must be vector.ErrInvalidVector")
	}
}

func TestErrorIsAs(t *testing.T) {
	driverErr := errors.New("driver says no")
	for _, kind := range allSentinels {
		err := fmt.Errorf("query failed: %w", &momo.Error{Kind: kind, Op: "select", Table: "documents", Err: driverErr})
		if !errors.Is(err, kind) {
			t.Errorf("errors.Is(%v) = false", kind)
		}
		var me *momo.Error
		if !errors.As(err, &me) || me.Table != "documents" {
			t.Errorf("errors.As failed for %v", kind)
		}
		if !errors.Is(err, driverErr) {
			t.Errorf("driver error not reachable for %v", kind)
		}
		if me.Detail() != driverErr.Error() {
			t.Errorf("Detail() = %q", me.Detail())
		}
	}
	if (&momo.Error{Kind: momo.ErrNotFound}).Detail() != "" {
		t.Error("Detail() must be empty when Err is nil")
	}
}
```

Create `vector/errors_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package vector

import "testing"

func TestInvalidVectorSentinel(t *testing.T) {
	if ErrInvalidVector.Error() != "vector: invalid vector" {
		t.Fatalf("message = %q", ErrInvalidVector.Error())
	}
}
```

Create `migrate/errors_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package migrate

import (
	"strings"
	"testing"
)

func TestMigrateSentinelMessages(t *testing.T) {
	for _, err := range []error{ErrChecksumMismatch, ErrDirty, ErrLocked, ErrDestructive, ErrBranchConflict} {
		if !strings.HasPrefix(err.Error(), "migrate: ") {
			t.Errorf("%q lacks the migrate: prefix", err.Error())
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./... -run 'Sentinel|IsAs|InvalidVector' -count=1` fails with `undefined: momo.ErrValidation`, `undefined: vector` and `undefined: ErrChecksumMismatch`.

- [ ] **Step 3: Implement.**
  - `vector/errors.go`: `ErrInvalidVector` as above, with a doc comment.
  - `migrate/errors.go`: the five variables with messages `migrate: checksum mismatch`, `migrate: database is dirty`, `migrate: lock held by another process`, `migrate: destructive change not authorized` and `migrate: migration branches conflict`.
  - `errors.go`: keep the spike sentinels; add `ErrValidation = errors.New("momo: validation failed")`, `ErrBusy = errors.New("momo: database is busy")`, `ErrLimitExceeded = errors.New("momo: limit exceeded")`, and `ErrInvalidVector = vector.ErrInvalidVector` (importing `github.com/azrtydxb/momo/vector`); add `func (e *Error) Detail() string`.

- [ ] **Step 4: Run and expect PASS.** `go test ./... -run 'Sentinel|IsAs|InvalidVector' -count=1` prints `ok` for `.`, `./vector` and `./migrate`.

- [ ] **Step 5: Commit.** `git add errors.go errors_test.go vector migrate && git commit -m "feat(errors): add the sentinel set, Detail accessor and vector and migrate sentinels"`.

## Task 9: Fake driver and shared model fixtures

Delivers: the test double every root test uses (no database), and hand-written fixture models shaped like generated code. The generator plan replaces the fixture model with generated output; the fixture keeps the same names.

Files:

- Create: `internal/fakedriver/fakedriver.go`, `internal/fakedriver/fakedriver_test.go`
- Create: `unit_fixtures_test.go` (package `momo_test`)

Interfaces (produced, exact):

- `package fakedriver`: `type Call struct{ SQL string; Args []any }`; `type Driver struct`; `func New(d dialect.Name) *Driver`; `(*Driver) Dialect() *dialect.Dialect`; `(*Driver) Query(ctx, sql, args) (momo.Rows, error)`; `(*Driver) Exec(ctx, sql, args) (int64, error)`; `(*Driver) Begin(ctx, opts momo.TxOptions) (momo.DriverTx, error)`; fields and methods `Calls() []Call`, `SetRows(rows [][]any)`, `Closed() int` (number of Rows.Close calls, race-safe), `Commits() int`, `Rollbacks() int`.
- `package momo_test` fixtures: a type `unitDocument` with fields `ID` and `TenantID` of Go type int64, `Title` of type string, `Published` of type bool and `DeletedAt` of type `*time.Time`; a method `MomoMeta` on `*unitDocument` that returns a pointer to a package-level value of type `momo.Meta[unitDocument]`. Package-level typed columns: `DocID`, `DocTenantID`, `DocTitle` and `DocPublished` are built with `momo.NewColumn`, and `DocDeletedAt` is built with `momo.NewNullColumn`; all name the table `documents` and the columns id, tenant_id, title, published and deleted_at. `DocID` carries FlagPK and FlagIdentity; `DocDeletedAt` carries FlagNullable and FlagSoftDelete.
- Meta: `Table "documents"`, `Columns` in the order id, tenant_id, title, published, deleted_at; `SoftDelete: 4`, `Identity: 0`, `Version: -1`, `PK: []int{0}`. (`CreatedAt`/`UpdatedAt` do not exist on the spike's `Meta` yet; Task 10 adds them and sets both to `-1` here.)

- [ ] **Step 1: Write the failing test.** Create `internal/fakedriver/fakedriver_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package fakedriver

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

func TestFakeDriverRecordsAndScripts(t *testing.T) {
	d := New(dialect.SQLite)
	d.SetRows([][]any{{int64(1), "a"}, {int64(2), "b"}})
	rows, err := d.Query(context.Background(), "SELECT 1", []any{"x"})
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	var name string
	n := 0
	for rows.Next() {
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if n != 2 || id != 2 || name != "b" {
		t.Fatalf("scripted rows wrong: n=%d id=%d name=%s", n, id, name)
	}
	if d.Closed() != 1 || len(d.Calls()) != 1 || d.Calls()[0].SQL != "SELECT 1" {
		t.Fatalf("recording wrong: closed=%d calls=%v", d.Closed(), d.Calls())
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/fakedriver/ -count=1` fails with `undefined: New`.

- [ ] **Step 3: Implement `internal/fakedriver/fakedriver.go`.** The driver guards `calls`, `rows` and counters with a `sync.Mutex`. `Query` appends a `Call` and returns a fresh cursor over a copy of the scripted rows; `Rows.Scan` assigns each row value into the destination with `reflect.ValueOf(dest).Elem().Set(reflect.ValueOf(v))` and returns an error when the kinds are not assignable; `Rows.Close` increments the closed counter once per call and is idempotent for `Next`/`Err`. `Exec` records the call and returns 0. `Begin` returns a `DriverTx` that records through the same counters; `Commit` increments commits and `Rollback` increments rollbacks. The scripted rows are returned by every `Query`.

- [ ] **Step 4: Run and expect PASS; then write the fixtures.** `go test ./internal/fakedriver/ -count=1` prints `ok`. Create `unit_fixtures_test.go` with the `unitDocument` type, the `MomoMeta` method returning a pointer to a package-level `momo.Meta[unitDocument]` value whose `Ptr` and `Value` closures switch on `i` over the five fields (`Ptr` returns the field address as `any`; `Value` returns the field value as `any`, with `DeletedAt` returning the `*time.Time` value). Then run `go vet . ` which must exit 0.

- [ ] **Step 5: Commit.** `git add internal/fakedriver unit_fixtures_test.go && git commit -m "test: add fake driver and shared model fixtures"`.

## Task 10: Spike repairs: flags, qualified table references and soft-delete filter

Delivers: contract names in `meta.go` and `TableRef`, and a fix for a spike defect that lets a column-restricted query on a soft-delete model return deleted rows (S-4, S-75 precondition).

Files:

- Modify: `meta.go`, `select.go`
- Create: `select_repair_test.go` (package `momo_test`), with the shared fixtures in Task 9

Interfaces (produced, exact):

- `const ( FlagPK uint16 = 1 << iota; FlagUnique; FlagNullable; FlagIdentity; FlagReadOnly; FlagVersion; FlagSoftDelete; FlagHasDefault; FlagCreateOnly; FlagImmutable; FlagSensitive; FlagCreatedAt; FlagUpdatedAt )` in `meta.go`.
- `Meta[T]` fields: `Table, Schema string; Columns []ColumnMeta; Ptr func(*T, int) any; Value func(*T, int) any; TableSchema *schema.Table; Identity, Version, SoftDelete, CreatedAt, UpdatedAt int; PK []int`. `Hooks` is added by the hooks plan (S-85) and is absent here.
- `TableRef{Name, Alias, Schema string}`; `appendFrom` writes `"schema"."name"` when Schema is set.
- Removed: the `SQLFor` stub at `select.go` (it has an empty body and a wrong signature).

- [ ] **Step 1: Write the failing test.** Create `select_repair_test.go` (uses fixtures from Task 9; for this task only, the test file declares the minimal inline fixture it needs):

It uses the fixtures from Task 9. This task's failing test is:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestSoftDeleteFilterWithColumns(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	all := []momo.Expr{DocID, DocTenantID, DocTitle, DocPublished, DocDeletedAt}
	c, err := momo.Select[unitDocument](db).Columns(all...).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL`
	if c.SQL != want {
		t.Fatalf("soft-delete filter lost when columns are restricted:\n got %s\nwant %s", c.SQL, want)
	}
}

func TestTableRefSchemaQualified(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](db).From(momo.TableRef{Name: "documents", Schema: "tenant"}).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "tenant"."documents" WHERE "documents"."deleted_at" IS NULL`; c.SQL != want {
		t.Fatalf("got %s\nwant %s", c.SQL, want)
	}
}
```

Note: the `From` argument is a `TableSource`; `TableRef` satisfies it. The bare-column form on the second query is correct because the spike qualifies with the FROM alias only when `From` supplies one; the implementer checks the rendered text against the expectation and, if the qualifier differs, changes `qualifier()` only.

- [ ] **Step 2: Run and expect FAIL.** `go test . -run 'TestSoftDeleteFilterWithColumns|TestTableRefSchemaQualified' -count=1` fails with the rendered SQL lacking `WHERE "documents"."deleted_at" IS NULL` (first test) and with `undefined` for `Schema` on `TableRef` (second test).

- [ ] **Step 3: Implement.**
  - `meta.go`: add the flag constants; rename `Schema *schema.Table` to `TableSchema *schema.Table`; add `Schema string`, `CreatedAt int`, `UpdatedAt int` (and in the same change set `CreatedAt: -1, UpdatedAt: -1` on `unitDocument`'s Meta in `unit_fixtures_test.go`, because the zero value would mark column 0 as created-at); add comments that `-1` means absent for Identity, Version, SoftDelete, CreatedAt, UpdatedAt.
  - `TableRef`: add `Schema string`; `appendFrom` writes `b.Ident(t.Schema); b.WriteByte('.')` before the table name when Schema is non-empty; `Qualifier()` is unchanged.
  - `select.go`: delete the `SQLFor` method; in `render`, change the soft-delete condition to `if sq.meta.SoftDelete >= 0 && sq.meta.Table != "" && !s.withDeleted` (remove the `&& s.cols == nil` part) so the filter applies whatever the select list is.

- [ ] **Step 4: Run and expect PASS.** `go test . -run 'TestSoftDeleteFilterWithColumns|TestTableRefSchemaQualified' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add meta.go select.go select_repair_test.go && git commit -m "fix(momo): keep the soft-delete filter with restricted columns and align Meta with the contract"`.

## Task 11: Placeholders, quoting and the builder fuzz target

Delivers: S-4 (`TestPlaceholdersAndQuoting`, `FuzzBuilder`) and the security rule that identifiers are quoted and values are bound.

Files:

- Create: `builder_test.go` (package `momo_test`)

Interfaces: uses `momo.Select`, `momo.NewColumn`, `momo.Compiled` and `SelectQuery.SQL()`, and the fixtures of Task 9. No new production names.

Rendering rule used by the expectations: a WHERE list with more than one predicate parenthesizes each predicate and joins them with AND; a model query always carries the soft-delete predicate of `unitDocument`, which binds no argument.

- [ ] **Step 1: Write the tests.** Create `builder_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestPlaceholdersAndQuoting(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).Where(DocTenantID.Eq(7), DocTitle.Eq("o'brien")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `"documents"."tenant_id" = $1`) || !strings.Contains(c.SQL, `"documents"."title" = $2`) {
		t.Fatalf("postgres numbering wrong: %s", c.SQL)
	}
	if strings.Contains(c.SQL, "o'brien") {
		t.Fatalf("value interpolated into SQL: %s", c.SQL)
	}
	if len(c.Args) != 2 || c.Args[0] != int64(7) || c.Args[1] != "o'brien" {
		t.Fatalf("args wrong: %#v", c.Args)
	}

	sq := momo.New(fakedriver.New(dialect.SQLite))
	c, err = momo.Select[unitDocument](sq).Where(DocTitle.Eq("x")).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(c.SQL, "$") || !strings.Contains(c.SQL, `"documents"."title" = ?`) {
		t.Fatalf("sqlite placeholders wrong: %s", c.SQL)
	}
}

// FuzzBuilder: identifiers are always quoted (quotes doubled) and the value
// is always bound, never present in the SQL text.
func FuzzBuilder(f *testing.F) {
	f.Add("title", "x")
	f.Add(`a"b`, "'; DROP TABLE t; --")
	f.Fuzz(func(t *testing.T, name, value string) {
		for _, d := range []dialect.Name{dialect.Postgres, dialect.SQLite} {
			db := momo.New(fakedriver.New(d))
			col := momo.NewColumn[string]("tbl", name, 0)
			c, err := momo.Select[unitDocument](db).Where(col.Eq(value)).SQL()
			if err != nil {
				t.Fatal(err)
			}
			quoted := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
			if !strings.Contains(c.SQL, quoted) {
				t.Fatalf("%s: identifier %q not quoted in %s", d, name, c.SQL)
			}
			if len(c.Args) != 1 || c.Args[0] != value {
				t.Fatalf("%s: value not bound: %#v", d, c.Args)
			}
			if value != "" && strings.Contains(c.SQL, "'"+value+"'") {
				t.Fatalf("%s: value interpolated: %s", d, c.SQL)
			}
		}
	})
}
```

- [ ] **Step 2: Run and expect FAIL or guard PASS.** `go test . -run '^TestPlaceholdersAndQuoting$' -count=1`. The tests need the fakedriver and fixtures of Task 9; on a correct foundation both assertions hold on the spike and the run is a guard PASS. Then `go test . -run '^$' -fuzz '^FuzzBuilder$' -fuzztime=10s`.

- [ ] **Step 3: Implement only what a failing assertion requires.** `Builder.Arg` already emits `$n` or `?` through `AppendPlaceholder`, and `Builder.Ident` already quotes through `AppendIdent`. If the fuzz run finds an unquoted identifier, route that path through `Ident`; if it finds an interpolated value, route that path through `Arg`.

- [ ] **Step 4: Run and expect PASS.** `go test . -run '^TestPlaceholdersAndQuoting$' -count=1` prints `ok`, and the fuzz command completes.

- [ ] **Step 5: Commit.** `git add builder_test.go builder.go expr.go && git commit -m "test(momo): verify placeholders, quoting and bound values with a builder fuzz target"`.

## Task 12: Raw SQL placeholders

Delivers: S-4 raw parameterized SQL: `?` positional, `??` as a literal `?`, expression arguments, and a placeholder/argument mismatch as a build error. S-77 extends this later.

Files:

- Create: `raw_test.go` (package `momo_test`)

Interfaces: `momo.SQL(fragment string, args ...any) momo.Predicate` (spike), `momo.Select`, `SelectQuery.Where`, `SelectQuery.SQL`.

- [ ] **Step 1: Write the test.** Create `raw_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"strings"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestRawPlaceholders(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).Where(momo.SQL("tenant_id = ? AND title ?? 'k'", int64(3))).SQL()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(c.SQL, `tenant_id = $1 AND title ? 'k'`) {
		t.Fatalf("raw rendering wrong: %s", c.SQL)
	}
	if len(c.Args) != 1 || c.Args[0] != int64(3) {
		t.Fatalf("raw args wrong: %#v", c.Args)
	}

	if _, err := momo.Select[unitDocument](pg).Where(momo.SQL("a = ? AND b = ?", 1)).SQL(); err == nil {
		t.Fatal("placeholder/argument mismatch must fail to build")
	}
}
```

- [ ] **Step 2: Run and expect FAIL or guard PASS.** `go test . -run '^TestRawPlaceholders$' -count=1`. The mismatch case passes on the spike when `appendRaw` reports through `SetErr` and `compile` returns that error; a FAIL shows which assertion broke.

- [ ] **Step 3: Implement only if red.** If `SQL()` returns no error for the mismatch, the fix is in `appendRaw`: it must call `b.SetErr` in both the too-few and too-many branches.

- [ ] **Step 4: Run and expect PASS.** `go test . -run '^TestRawPlaceholders$' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add raw_test.go && git commit -m "test(momo): cover raw SQL placeholders and mismatch errors"`.

## Task 13: SELECT rendering golden tests

Delivers: S-4 unit rendering (joins with aliases and self-joins, projections, aggregates, grouping, CTEs, NULLS placement, locking, limit and offset), capability gating (`ErrUnsupported`).

Files:

- Create: `select_golden_test.go` (package `momo_test`)

Interfaces: `momo.Select`, `SelectQuery` methods `Where, Columns, From, Join, LeftJoin, CrossJoin, GroupBy, Having, OrderBy, Limit, Offset, ForUpdate, ForShare, With, WithRecursive, SQL`; `momo.TableRef`; `momo.As`; `momo.Count`; `Column.Asc/Desc/NullsFirst/NullsLast`.

- [ ] **Step 1: Write the failing test.** Create `select_golden_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestSelectGolden(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	sq := momo.New(fakedriver.New(dialect.SQLite))
	my := momo.New(fakedriver.New(dialect.MySQL))

	cases := []struct {
		name string
		q    func() (momo.Compiled, error)
		want string
	}{
		{"where and order with nulls", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Where(DocTitle.Eq("a")).OrderBy(DocTitle.Asc().NullsLast()).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."title" = $1) AND ("documents"."deleted_at" IS NULL) ORDER BY "documents"."title" ASC NULLS LAST`},
		{"limit and offset postgres", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](pg).Limit(10).Offset(20).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT $1 OFFSET $2`},
		{"offset without limit on sqlite", func() (momo.Compiled, error) {
			return momo.Select[unitDocument](sq).Offset(5).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE "documents"."deleted_at" IS NULL LIMIT -1 OFFSET ?`},
		{"self join with alias", func() (momo.Compiled, error) {
			p := momo.TableRef{Name: "documents", Alias: "parent"}
			return momo.Select[unitDocument](pg).Join(p, DocTenantID.As("documents").EqExpr(DocTenantID.As("parent"))).SQL()
		}, `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" INNER JOIN "documents" AS "parent" ON "documents"."tenant_id" = "parent"."tenant_id" WHERE "documents"."deleted_at" IS NULL`},
	}
	for _, c := range cases {
		got, err := c.q()
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got.SQL != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got.SQL, c.want)
		}
	}

	if _, err := momo.Select[unitDocument](my).OrderBy(DocTitle.Asc().NullsFirst()).SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("NULLS FIRST on MySQL = %v, want ErrUnsupported", err)
	}
	if _, err := momo.Select[unitDocument](sq).ForUpdate().SQL(); !errors.Is(err, momo.ErrUnsupported) {
		t.Errorf("FOR UPDATE on SQLite = %v, want ErrUnsupported", err)
	}
}
```

The expected strings follow `appendAnd` in select.go (a WHERE list with more than one predicate parenthesizes each predicate, a single predicate is left bare) and `junction` rendering in expr.go. The soft-delete predicate of unitDocument is part of every model query.

- [ ] **Step 2: Run and expect FAIL.** `go test . -run '^TestSelectGolden$' -count=1`. Expected failures list rendering differences per case and the `ForUpdate` gating case.

- [ ] **Step 3: Implement.** Fix the spike where a case fails: `ForUpdate`/`withLock` must set an `ErrUnsupported` build error when `!b.d.Locking`; `Order.appendSQL` must emit `NULLS FIRST|LAST` only when `b.d.NullsOrdering` is true and otherwise record `ErrUnsupported`; the join kind text is `INNER JOIN` for `Join`, `LEFT JOIN` for `LeftJoin`, and `CROSS JOIN` for `CrossJoin`. `As(alias)` on `Column` re-qualifies with the alias as the spike already does.

- [ ] **Step 4: Run and expect PASS.** `go test . -run '^TestSelectGolden$' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add select_golden_test.go select.go expr.go && git commit -m "test(momo): golden SELECT rendering and capability gating"`.

## Task 14: Keyset pagination with deterministic tie-breaking

Delivers: S-4 keyset pagination with deterministic tie-breaking, and the edge case "keyset pagination with mixed sort directions and non-unique sort keys".

Files:

- Modify: `select.go` (add `After`)
- Create: `keyset_test.go` (package `momo_test`)

Interfaces (produced, exact):

- `func (sq SelectQuery[T]) After(values ...any) SelectQuery[T]`. The values are the cursor for the effective ORDER BY list, in order. The effective list is the OrderBy items followed by the primary key ascending when the primary key is not already an item. Keys with ascending direction compare with `>`, descending with `<`. The predicate is the lexicographic expansion: disjunct i is equality on the keys before i and the comparison on key i. Each value is bound with `Builder.Arg`.
- `After` without `OrderBy` makes `SQL()` fail with `ErrValidation`. The number of values must equal the number of effective keys, otherwise `SQL()` fails with `ErrValidation`. `After` with a nullable effective key makes `SQL()` fail with `ErrUnsupported`; the NULL-aware form is outside this plan.

- [ ] **Step 1: Write the failing test.** Create `keyset_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestKeysetPagination(t *testing.T) {
	pg := momo.New(fakedriver.New(dialect.Postgres))
	c, err := momo.Select[unitDocument](pg).OrderBy(DocTenantID.Desc(), DocTitle.Asc()).After(int64(7), "m", int64(42)).Limit(10).SQL()
	if err != nil {
		t.Fatal(err)
	}
	want := `SELECT "documents"."id", "documents"."tenant_id", "documents"."title", "documents"."published", "documents"."deleted_at" FROM "documents" WHERE ("documents"."tenant_id" < $1 OR ("documents"."tenant_id" = $2 AND "documents"."title" > $3) OR ("documents"."tenant_id" = $4 AND "documents"."title" = $5 AND "documents"."id" > $6)) AND ("documents"."deleted_at" IS NULL) ORDER BY "documents"."tenant_id" DESC, "documents"."title" ASC, "documents"."id" ASC LIMIT $7`
	if c.SQL != want {
		t.Fatalf("keyset SQL:\n got %s\nwant %s", c.SQL, want)
	}
	wantArgs := []any{int64(7), int64(7), "m", int64(7), "m", int64(42), int64(10)}
	if len(c.Args) != len(wantArgs) {
		t.Fatalf("keyset args: %#v", c.Args)
	}
	for i := range wantArgs {
		if c.Args[i] != wantArgs[i] {
			t.Fatalf("arg %d = %#v, want %#v", i, c.Args[i], wantArgs[i])
		}
	}
	if _, err := momo.Select[unitDocument](pg).After(1).SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("After without OrderBy = %v, want ErrValidation", err)
	}
	if _, err := momo.Select[unitDocument](pg).OrderBy(DocTitle.Asc()).After("a").SQL(); !errors.Is(err, momo.ErrValidation) {
		t.Errorf("After with too few values = %v, want ErrValidation", err)
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test . -run '^TestKeysetPagination$' -count=1` fails with `undefined method After`.

- [ ] **Step 3: Implement `After`.** Add a `keyset []any` field to `selectCore`; `After` appends with `slices.Clip` and returns a copy. In `render`, build the effective key list from `s.order` (each `Order` carries `Expr` and `Desc`), append the primary key from `sq.meta.PK` ascending when it is absent, check the value count and the nullable flag (`FlagNullable` through `ColumnRef`) and record `ErrValidation` or `ErrUnsupported` through `b.SetErr`. Build the predicate as `junction{" OR ", disjuncts}` where disjunct i is `junction{" AND ", equalities for keys before i followed by the comparison on key i}`; disjunct 0 is the bare comparison. Place this predicate first in the WHERE list, so the soft-delete predicate follows it.

- [ ] **Step 4: Run and expect PASS.** `go test . -run '^TestKeysetPagination$' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add select.go keyset_test.go && git commit -m "feat(momo): keyset pagination with deterministic tie-breaking"`.

## Task 15: Query immutability under concurrent reuse

Delivers: S-4 `TestQueryReuseNoLeak` under `go test -race`.

Files:

- Create: `reuse_test.go` (package `momo_test`)

- [ ] **Step 1: Write the test.** Create `reuse_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"sync"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func TestQueryReuseNoLeak(t *testing.T) {
	db := momo.New(fakedriver.New(dialect.Postgres))
	base := momo.Select[unitDocument](db).Where(DocPublished.Eq(true)).OrderBy(DocID.Asc())
	before, err := base.SQL()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d := base.Where(DocTitle.Eq("t")).Limit(int64(i))
			if _, err := d.SQL(); err != nil {
				t.Error(err)
			}
			e := base.Where(DocTenantID.Eq(int64(i)))
			if _, err := e.SQL(); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	after, err := base.SQL()
	if err != nil {
		t.Fatal(err)
	}
	if before.SQL != after.SQL || len(after.Args) != len(before.Args) {
		t.Fatalf("base query changed:\n%s\n%s", before.SQL, after.SQL)
	}
}
```

- [ ] **Step 2: Run and expect PASS (guard) or FAIL.** `go test -race . -run '^TestQueryReuseNoLeak$' -count=1`. The spike uses `slices.Clip` on every append, so this passes; a FAIL reports a data race in a builder method that appends into a shared backing array.

- [ ] **Step 3: Implement only if red.** Any builder method that appends without `slices.Clip` gets the clip; keep immutability by copying where the slice is stored.

- [ ] **Step 4: Run and expect PASS.** `go test -race . -run '^TestQueryReuseNoLeak$' -count=1` prints `ok` with no `DATA RACE` lines.

- [ ] **Step 5: Commit.** `git add reuse_test.go && git commit -m "test(momo): concurrent query reuse must not leak predicates"`.

## Task 16: Cursor closing and empty-result semantics

Delivers: S-4 `TestIterClosesCursor` (unit form with the fake driver; the pool-statistics form belongs to the PostgreSQL integration plan) and S-8 `TestEmptyResultNotNotFound`.

Files:

- Create: `cursor_test.go` (package `momo_test`)

Interfaces: `SelectQuery.All`, `One`, `Each`, `Iter` (spike); `fakedriver.Driver.SetRows`, `Closed()`.

- [ ] **Step 1: Write the tests.** Create `cursor_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
)

func threeRows() [][]any {
	return [][]any{
		{int64(1), int64(1), "a", true, nil},
		{int64(2), int64(1), "b", false, nil},
		{int64(3), int64(1), "c", true, nil},
	}
}

func TestIterClosesCursor(t *testing.T) {
	ctx := context.Background()

	drv := fakedriver.New(dialect.SQLite)
	drv.SetRows(threeRows())
	for _, err := range momo.Select[unitDocument](momo.New(drv)).Iter(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if drv.Closed() != 1 {
		t.Fatalf("early break: closed=%d, want 1", drv.Closed())
	}

	drv2 := fakedriver.New(dialect.SQLite)
	drv2.SetRows(threeRows())
	boom := errors.New("callback failed")
	err := momo.Select[unitDocument](momo.New(drv2)).Each(ctx, func(*unitDocument) error { return boom })
	if !errors.Is(err, boom) || drv2.Closed() != 1 {
		t.Fatalf("callback error: err=%v closed=%d", err, drv2.Closed())
	}

	drv3 := fakedriver.New(dialect.SQLite)
	drv3.SetRows(threeRows())
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = momo.Select[unitDocument](momo.New(drv3)).Each(cctx, func(*unitDocument) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || drv3.Closed() != 1 {
		t.Fatalf("cancel mid-iteration: err=%v closed=%d, want context.Canceled and 1", err, drv3.Closed())
	}
}

func TestEmptyResultNotNotFound(t *testing.T) {
	ctx := context.Background()
	db := momo.New(fakedriver.New(dialect.SQLite))
	docs, err := momo.Select[unitDocument](db).All(ctx)
	if err != nil || len(docs) != 0 {
		t.Fatalf("All on zero rows = (%v, %v), want empty and nil error", docs, err)
	}
	if _, err := momo.Select[unitDocument](db).One(ctx); !errors.Is(err, momo.ErrNotFound) {
		t.Fatalf("One on zero rows = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test . -run 'TestIterClosesCursor|TestEmptyResultNotNotFound' -count=1`. A path that skips `Close` fails the closed-count assertion; `Each` with a cancelled context must return `context.Canceled` before the next row.

- [ ] **Step 3: Implement.** In `select.go`, `Each` and `Iter` open rows through `rows(ctx)`, `defer` `Close` right after a successful open, check `ctx.Err()` before each `Next`, and return the context error wrapped with `%w` when cancelled. `Iter` yields `(T, error)`, yields the error once and stops. `One` maps zero rows to `&Error{Kind: ErrNotFound, Op: "select"}` and two rows to `&Error{Kind: ErrTooManyRows, Op: "select"}`.

- [ ] **Step 4: Run and expect PASS.** `go test . -run 'TestIterClosesCursor|TestEmptyResultNotNotFound' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add cursor_test.go select.go && git commit -m "test(momo): cursors close on every return path and empty results are not errors"`.

## Task 17: Compile-fail harness and valid-example build

Delivers: S-4 `TestCompileFail` and `TestValidExamplesCompile`. Typed columns reject wrong-typed values at compile time.

Files:

- Create: `testdata/compilefail/wrongtype/main.go`
- Create: `testdata/compilefail/wrongtype_any/main.go`
- Create: `testdata/valid/basic/main.go`
- Create: `compilefail_test.go` (package `momo_test`)

Interfaces: none produced; the fixtures import `github.com/azrtydxb/momo`.

- [ ] **Step 1: Write the fixtures and the failing test.** `testdata/compilefail/wrongtype/main.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var title = momo.NewColumn[string]("documents", "title", 0)

func main() {
	_ = title.Eq(42)
}
```

`testdata/compilefail/wrongtype_any/main.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var title = momo.NewColumn[string]("documents", "title", 0)

func f(v any) momo.Predicate {
	return title.Eq(v)
}

func main() { _ = f }
```

`testdata/valid/basic/main.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/azrtydxb/momo"

var title = momo.NewColumn[string]("documents", "title", 0)

func main() {
	_ = title.Eq("ok")
}
```

`compilefail_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCompileFail(t *testing.T) {
	for _, name := range []string{"wrongtype", "wrongtype_any"} {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/compilefail/"+name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("%s compiled; want a compile error", name)
			continue
		}
		if !strings.Contains(string(out), "cannot use") {
			t.Errorf("%s: unexpected error text:\n%s", name, out)
		}
	}
}

func TestValidExamplesCompile(t *testing.T) {
	cmd := exec.Command("go", "build", "-o", os.DevNull, "./testdata/valid/basic")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("valid example failed to build: %v\n%s", err, out)
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** Before the fixtures exist the test fails with `no required module` or `directory not found`; after creating them, run `go test . -run 'TestCompileFail|TestValidExamplesCompile' -count=1`. Expected FAIL only if `Eq` accepts `any`; the spike's `Eq(v T)` makes both compile-fail fixtures fail as intended, so this step is the guard. Record the red state as the missing-directory failure from the first run.

- [ ] **Step 3: Implement.** No production change is expected. If `wrongtype_any` builds, `Eq` has a non-generic signature and must be restored to `Eq(v T) Predicate` in `expr.go`.

- [ ] **Step 4: Run and expect PASS.** `make test-compilefail` prints `ok`.

- [ ] **Step 5: Commit.** `git add testdata compilefail_test.go && git commit -m "test(momo): compile-fail fixtures and valid example build"`.

## Task 18: Test database URL helpers and PostgreSQL containers

Delivers: S-20 harness as URL helpers only (docs/ARCHITECTURE.md, round 2), the non-superuser application role for S-47, and the M1 compose services (PostgreSQL 17 and 16, PgBouncer). `testopen` belongs to momo-03. The MySQL and MariaDB compose services are added by momo-08 in M3, and SQLite needs no container.

Files:

- Create: `internal/testdb/testdb.go`, `internal/testdb/testdb_test.go`
- Create: `internal/testdb/isolation_integration_test.go`
- Create: `internal/testdb/compose.yaml`
- Modify: `go.mod`, `go.sum` (adds `github.com/jackc/pgx/v5 v5.11.0` and `github.com/go-sql-driver/mysql v1.10.1`)

Interfaces (produced, exact; no driver package of momo is imported, so driver plans use them without an import cycle):

- `func PostgresURL(t testing.TB) string`: per-test isolation (docs/ARCHITECTURE.md, "Per-test isolation"). Connects to `MOMO_TEST_POSTGRES_URL` with `github.com/jackc/pgx/v5` directly, creates the schema `t_<16 hex chars from crypto/rand>`, and returns the base URL with the query parameter `search_path=t_xxx,public`. The result is memoized per `testing.TB` (a `sync.Map` keyed by `t`), so `testopen.Postgres(t)` and raw `PostgresURL(t)` calls in one test share one schema; a subtest calling it with its own `t` gets its own schema. `t.Cleanup` runs `DROP SCHEMA "t_xxx" CASCADE` and reports an error through `t.Errorf`. Once per process (`sync.Once` plus `pg_advisory_lock(7_000_001)`) it runs `CREATE EXTENSION IF NOT EXISTS vector SCHEMA public`, so the `vector` types resolve through `public`.
- `func PostgresAppRoleURL(t testing.TB) string`: the same schema as `PostgresURL(t)` for that `t` (the helper calls `PostgresURL(t)` first), plus, run as the administrator, `GRANT USAGE ON SCHEMA "t_xxx" TO <app role>`, `ALTER DEFAULT PRIVILEGES IN SCHEMA "t_xxx" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO <app role>` and `ALTER DEFAULT PRIVILEGES IN SCHEMA "t_xxx" GRANT USAGE, SELECT ON SEQUENCES TO <app role>` (so tables the administrator creates in that schema are covered; the app role has no `CREATE` and cannot run DDL). The role and password come from `MOMO_TEST_POSTGRES_APP_URL` (created when absent), and the returned URL carries that role's credentials with the same `search_path` appended.
- `func PgBouncerURL(t testing.TB) string`: `MOMO_TEST_PGBOUNCER_URL` with the schema of `PostgresURL(t)` for that `t` (the helper calls `PostgresURL(t)` first) and the same `search_path` parameter appended. The PgBouncer database must be the database named in `MOMO_TEST_POSTGRES_URL`. The PgBouncer configuration must set `track_extra_parameters = search_path` (PgBouncer 1.20 or later); see Task 19 and momo-03 Task 10.
- `func Schema(t testing.TB) string`: the per-test schema name `t_xxx` of `PostgresURL(t)` (it calls `PostgresURL(t)`, so it creates the schema when this is the first call for `t`). Migration tests that manage extra namespaces derive their names from it.
- `func SQLitePath(t testing.TB) string`: `filepath.Join(t.TempDir(), "momo.db")`; no environment variable. Each test gets its own directory, so SQLite tests never collide.
- `func MySQL84URL(t testing.TB) string`, `func MySQL97URL(t testing.TB) string`, `func MariaDBURL(t testing.TB) string`: go-sql-driver DSNs for a database `t_<16 hex chars>` created per test on the servers named by `MOMO_TEST_MYSQL84_URL`, `MOMO_TEST_MYSQL97_URL` and `MOMO_TEST_MARIADB_URL`, memoized per `t`, dropped in `t.Cleanup`. These helpers are provided now; no test calls them until M3.
- Tests never `CREATE` or `DROP` schemas or databases themselves and need not use unique table names; `DROP TABLE IF EXISTS` is unnecessary but harmless.
- Constants `envPostgres`, `envPostgresApp`, `envPgBouncer`, `envMySQL84`, `envMySQL97`, `envMariaDB` hold the six variable names listed above.
- Unexported `func resolve(env string, lookup func(string) (string, bool), require bool) (string, error)`, `var errSkip = errors.New("testdb: integration URL not set")`, and `func randomName() string` (returns `t_` followed by 8 bytes from `crypto/rand` as 16 lowercase hex characters).
- Missing URLs: when the variable is unset, a helper calls `t.Fatal` if `MOMO_REQUIRE_INTEGRATION=1` and `t.Skip` otherwise. Only the helpers call `t.Skip`.

- [ ] **Step 1: Write the failing test.** Create `internal/testdb/testdb_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	set := func(string) (string, bool) { return "postgres://x", true }
	unset := func(string) (string, bool) { return "", false }

	if v, err := resolve(envPostgres, set, true); err != nil || v != "postgres://x" {
		t.Fatalf("set variable: %q, %v", v, err)
	}
	if _, err := resolve(envPostgres, unset, false); err != errSkip {
		t.Fatalf("unset, not required: err=%v, want errSkip", err)
	}
	_, err := resolve(envPostgres, unset, true)
	if err == nil || err == errSkip || !strings.Contains(err.Error(), envPostgres) {
		t.Fatalf("unset, required: err=%v, want an error naming the variable", err)
	}
}

func TestEnvVarNames(t *testing.T) {
	want := map[string]string{
		envPostgres:    "MOMO_TEST_POSTGRES_URL",
		envPostgresApp: "MOMO_TEST_POSTGRES_APP_URL",
		envPgBouncer:   "MOMO_TEST_PGBOUNCER_URL",
		envMySQL84:     "MOMO_TEST_MYSQL84_URL",
		envMySQL97:     "MOMO_TEST_MYSQL97_URL",
		envMariaDB:     "MOMO_TEST_MARIADB_URL",
	}
	for got, name := range want {
		if got != name {
			t.Errorf("variable name %q, want %q", got, name)
		}
	}
}

func TestSQLitePathIsInTempDir(t *testing.T) {
	if p := SQLitePath(t); filepath.Base(p) != "momo.db" {
		t.Fatalf("SQLitePath = %q, want a momo.db file", p)
	}
}

func TestRandomSuffixIsUniqueAndSafe(t *testing.T) {
	a, b := randomName(), randomName()
	if a == b || len(a) != 18 || !strings.HasPrefix(a, "t_") || strings.ContainsAny(a, `"'; `) {
		t.Fatalf("randomName gave %q and %q", a, b)
	}
}
```

Also create `internal/testdb/isolation_integration_test.go` (the integration build tag; package `testdb`, so it can reach `envPostgres` and `require`):

```go
//go:build integration

// SPDX-License-Identifier: Apache-2.0

package testdb

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func searchPathSchema(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	name, rest, ok := strings.Cut(u.Query().Get("search_path"), ",")
	if !ok || rest != "public" || !strings.HasPrefix(name, "t_") || len(name) != 18 {
		t.Fatalf("search_path = %q, want t_<16 hex>,public", u.Query().Get("search_path"))
	}
	return name
}

func TestPostgresURLIsolated(t *testing.T) {
	ctx := context.Background()
	admin := require(t, envPostgres)

	var seen []string
	for _, sub := range []string{"first", "second"} {
		t.Run(sub, func(t *testing.T) {
			u1, u2 := PostgresURL(t), PostgresURL(t)
			if u1 != u2 {
				t.Fatalf("same t returned two URLs: %q and %q", u1, u2)
			}
			name := searchPathSchema(t, u1)
			if name != Schema(t) {
				t.Fatalf("Schema(t) = %q, URL search_path schema = %q", Schema(t), name)
			}
			for _, prev := range seen {
				if prev == name {
					t.Fatalf("schema %q reused by a second test", name)
				}
			}
			seen = append(seen, name)

			conn, err := pgx.Connect(ctx, u1)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close(ctx)
			var current string
			if err := conn.QueryRow(ctx, "SELECT current_schema()").Scan(&current); err != nil || current != name {
				t.Fatalf("current_schema() = %q, %v; want %q", current, err, name)
			}
			if _, err := conn.Exec(ctx, "CREATE TABLE docs (id int PRIMARY KEY)"); err != nil {
				t.Fatalf("CREATE TABLE docs in %s: %v", name, err)
			}
		})
	}

	if len(seen) != 2 || seen[0] == seen[1] {
		t.Fatalf("subtests shared a schema: %v", seen)
	}
	t.Run("app role cannot create but can read", func(t *testing.T) {
		adminURL := PostgresURL(t)
		appURL := PostgresAppRoleURL(t)
		name := searchPathSchema(t, appURL)
		ac, err := pgx.Connect(ctx, adminURL)
		if err != nil {
			t.Fatal(err)
		}
		defer ac.Close(ctx)
		if _, err := ac.Exec(ctx, "CREATE TABLE admin_made (id int PRIMARY KEY)"); err != nil {
			t.Fatal(err)
		}
		app, err := pgx.Connect(ctx, appURL)
		if err != nil {
			t.Fatal(err)
		}
		defer app.Close(ctx)
		_, err = app.Exec(ctx, "CREATE TABLE app_made (id int)")
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Fatalf("app role CREATE TABLE in %s: err = %v, want SQLSTATE 42501", name, err)
		}
		var n int
		if err := app.QueryRow(ctx, "SELECT count(*) FROM admin_made").Scan(&n); err != nil {
			t.Fatalf("app role SELECT on an admin-created table: %v", err)
		}
	})
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, name := range seen {
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM pg_namespace WHERE nspname = $1", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("schema %s still exists after its test finished", name)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/testdb/ -count=1` fails with `undefined: resolve`, `undefined: envPostgres`, `undefined: SQLitePath` and `undefined: randomName`. Then run `MOMO_TEST_POSTGRES_URL=postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable go test -tags integration ./internal/testdb/ -run TestPostgresURLIsolated -count=1`; expect FAIL with `undefined: require`, `undefined: envPostgres`, `undefined: PostgresURL` and `undefined: Schema`.

- [ ] **Step 3: Implement `internal/testdb/testdb.go`.**
  - `resolve`: returns the value when set; when unset returns `errSkip` if `require` is false, otherwise `fmt.Errorf("testdb: %s is not set and MOMO_REQUIRE_INTEGRATION=1", env)`.
  - `require(t, env) string`: calls `resolve` with `require` set to `os.Getenv("MOMO_REQUIRE_INTEGRATION") == "1"`; on `errSkip` calls `t.Skip`; on any other error calls `t.Fatal`.
  - `randomName() string`: `"t_"` followed by 8 bytes from `crypto/rand` in lowercase hex (16 characters).
  - `schemas sync.Map` (key `testing.TB`, value the schema name) and `func schemaFor(t testing.TB) (admin, name string)`: on the first call for `t`, resolves `MOMO_TEST_POSTGRES_URL` with `require`, connects with `pgx.Connect` (`github.com/jackc/pgx/v5`, not the stdlib adapter), runs the once-per-process `ensureVector` (`sync.Once`; inside it takes `SELECT pg_advisory_lock(7000001)` on that connection, runs `CREATE EXTENSION IF NOT EXISTS vector SCHEMA public`, then unlocks), runs `CREATE SCHEMA "<name>"`, stores the name with `LoadOrStore`, and registers `t.Cleanup` that connects again and runs `DROP SCHEMA IF EXISTS "<name>" CASCADE`, calling `t.Errorf` on error and deleting the key.
  - `withSearchPath(raw, name string) string`: parses `raw` with `net/url`, sets the query parameter `search_path` to `<name>,public`, and returns the URL string.
  - `PostgresURL(t)` is `withSearchPath(admin, name)` from `schemaFor(t)`. `Schema(t)` returns the name from `schemaFor(t)`.
  - `PostgresAppRoleURL(t)` calls `PostgresURL(t)` first, then as the administrator runs a `DO` block that creates the role named in `MOMO_TEST_POSTGRES_APP_URL` with its password when absent, `GRANT CONNECT ON DATABASE <current database> TO <role>`, `GRANT USAGE ON SCHEMA "<name>" TO <role>`, `ALTER DEFAULT PRIVILEGES IN SCHEMA "<name>" GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO <role>` and `ALTER DEFAULT PRIVILEGES IN SCHEMA "<name>" GRANT USAGE, SELECT ON SEQUENCES TO <role>` (all run as the administrator; no `CREATE` privilege is granted), and returns `withSearchPath(appURL, name)`.
  - `freshMySQL(t, env string) string`: memoized per `(t, env)` in a second `sync.Map`; `mysql.ParseDSN` of the administrator DSN (blank import `_ "github.com/go-sql-driver/mysql"`), a connection with `DBName` cleared, `CREATE DATABASE \`<name>\``, cleanup `DROP DATABASE IF EXISTS`(error goes to`t.Errorf`), then `cfg.DBName = name`and`cfg.FormatDSN()`. `MySQL84URL`, `MySQL97URL`and`MariaDBURL` call it with their variables.
  - `PgBouncerURL(t)` calls `PostgresURL(t)` first, then returns `withSearchPath(require(t, envPgBouncer), Schema(t))`.
  - `SQLitePath(t)` returns `filepath.Join(t.TempDir(), "momo.db")`.
- Pin the modules: `go get github.com/jackc/pgx/v5@v5.11.0 github.com/go-sql-driver/mysql@v1.10.1`, then `go mod tidy`.

- [ ] **Step 4: Run and expect PASS; write the compose file.** `go test ./internal/testdb/ -count=1` prints `ok`, and `MOMO_TEST_POSTGRES_URL=postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable go test -tags integration ./internal/testdb/ -run TestPostgresURLIsolated -count=1` prints `ok` (start the server first with `make db-up`; expect PASS). Create `internal/testdb/compose.yaml`:

```yaml
name: momo-test
services:
  postgres17:
    image: pgvector/pgvector:0.8.7-pg17
    environment: { POSTGRES_PASSWORD: momo, POSTGRES_USER: postgres }
    ports: ["127.0.0.1:5432:5432"]
    healthcheck:
      {
        test: ["CMD-SHELL", "pg_isready -U postgres"],
        interval: 2s,
        retries: 30,
      }
  postgres16:
    image: pgvector/pgvector:0.8.7-pg16
    environment: { POSTGRES_PASSWORD: momo, POSTGRES_USER: postgres }
    ports: ["127.0.0.1:5433:5432"]
    healthcheck:
      {
        test: ["CMD-SHELL", "pg_isready -U postgres"],
        interval: 2s,
        retries: 30,
      }
  pgbouncer:
    image: edoburu/pgbouncer:v1.26.1-p0
    environment:
      DATABASE_URL: postgres://postgres:momo@postgres17:5432/postgres
      POOL_MODE: transaction
      AUTH_TYPE: trust
      TRACK_EXTRA_PARAMETERS: search_path
    ports: ["127.0.0.1:6432:5432"]
    depends_on: { postgres17: { condition: service_healthy } }
```

The compose file holds only the M1 services. momo-08 adds `mysql84` (`mysql:8.4.13`, port 3306), `mysql97` (`mysql:9.7.4`, port 3307) and `mariadb118` (`mariadb:11.8.9`, port 3308) with health checks in M3. The PgBouncer `AUTH_TYPE: trust` is for the local test container only; step 5 checks the pooled route. `PgBouncerURL(t)` carries `search_path` as a startup parameter, so the PgBouncer configuration must set `track_extra_parameters = search_path` (PgBouncer 1.20 or later); the edoburu image reads it from the environment as `TRACK_EXTRA_PARAMETERS: search_path`, which this compose service sets below, and momo-03 Task 10 sets the same line in `pgbouncer.ini`.

- [ ] **Step 5: Verify the containers and commit.** `docker compose -f internal/testdb/compose.yaml config -q` exits 0. Then `make db-up`; `docker compose -f internal/testdb/compose.yaml exec -T postgres17 psql -U postgres -c 'select 1'` prints `1`; and `psql "postgres://postgres:momo@127.0.0.1:6432/postgres" -c 'select 1'` prints `1` through PgBouncer. If the PgBouncer check fails, change only the `AUTH_TYPE` line and record the working value. Run `make db-down`, then `git add internal/testdb go.mod go.sum && git commit -m "test: add URL helpers and PostgreSQL service containers"`.

## Task 19: CI workflow core with the PostgreSQL integration job

Delivers: S-20 (unit, compile-fail, race, fuzz smoke, PostgreSQL integration on pgvector pg17 and pg16, Go 1.26 and 1.27), S-25 (govulncheck), S-26 (windows and macos jobs).

Milestone rule: the single `integration` job has a `dialect` matrix. M1 lists `postgres`. A milestone enables its dialect by adding that dialect to the list (`sqlite` in M2; `mysql84`, `mysql97` and `mariadb` in M3, each with its service block beside the postgres service). No further job is written.

Files:

- Create: `.github/workflows/ci.yml`
- Create: `internal/repocheck/ci_test.go`
- Modify: `go.mod`, `go.sum` (adds `go.yaml.in/yaml/v3 v3.0.6`, test-only)

Interfaces: none produced.

- [ ] **Step 1: Write the failing test.** Run `go get go.yaml.in/yaml/v3@v3.0.6`. Create `internal/repocheck/ci_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflow struct {
	On          map[string]any `yaml:"on"`
	Permissions any            `yaml:"permissions"`
	Jobs        map[string]job `yaml:"jobs"`
}

type job struct {
	RunsOn          any    `yaml:"runs-on"`
	If              string `yaml:"if"`
	ContinueOnError any    `yaml:"continue-on-error"`
	Permissions     any    `yaml:"permissions"`
	Strategy        struct {
		Matrix map[string]any `yaml:"matrix"`
	} `yaml:"strategy"`
	Services map[string]struct {
		Image string `yaml:"image"`
	} `yaml:"services"`
	Steps []step `yaml:"steps"`
}

type step struct {
	Uses string         `yaml:"uses"`
	Run  string         `yaml:"run"`
	With map[string]any `yaml:"with"`
}

func loadWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return w
}

func matrixHas(v any, want string) bool {
	list, ok := v.([]any)
	if !ok {
		return false
	}
	for _, x := range list {
		if s, ok := x.(string); ok && s == want {
			return true
		}
	}
	return false
}

func runsContain(j job, sub string) bool {
	for _, s := range j.Steps {
		if strings.Contains(s.Run, sub) {
			return true
		}
	}
	return false
}

func usesAction(j job, prefix string) bool {
	for _, s := range j.Steps {
		if strings.HasPrefix(s.Uses, prefix) {
			return true
		}
	}
	return false
}

func TestCIWorkflowCoverage(t *testing.T) {
	w := loadWorkflow(t, "ci.yml")
	required := map[string]string{
		"unit":         "make test",
		"compile-fail": "make test-compilefail",
		"race":         "make test-race",
		"fuzz-smoke":   "make test-fuzz",
		"integration":  "make test-integration-",
	}
	for name, cmd := range required {
		j, ok := w.Jobs[name]
		if !ok {
			t.Errorf("ci.yml has no job %q", name)
			continue
		}
		if !runsContain(j, cmd) {
			t.Errorf("job %q does not run %q", name, cmd)
		}
		if j.If != "" || j.ContinueOnError == true {
			t.Errorf("job %q may be skipped", name)
		}
		if !matrixHas(j.Strategy.Matrix["go"], "1.26") || !matrixHas(j.Strategy.Matrix["go"], "1.27") {
			t.Errorf("job %q does not test Go 1.26 and 1.27", name)
		}
	}
	integ := w.Jobs["integration"]
	// Every integration job runs `go test -tags integration -race ./...` with
	// MOMO_REQUIRE_INTEGRATION=1, directly or through its make target.
	for name, j := range w.Jobs {
		if !strings.HasPrefix(name, "integration") {
			continue
		}
		for _, s := range j.Steps {
			if !strings.Contains(s.Run, "make test-integration-") && !strings.Contains(s.Run, "go test") {
				continue
			}
			cmd := s.Run
			if strings.Contains(cmd, "make test-integration-") {
				var dialects []string
				for _, d := range j.Strategy.Matrix["dialect"].([]any) {
					dialects = append(dialects, d.(string))
				}
				for _, d := range dialects {
					c := exec.Command("make", "-n", "test-integration-"+d)
					c.Dir = repoRoot(t)
					out, err := c.CombinedOutput()
					if err != nil {
						t.Fatalf("make -n test-integration-%s: %v\n%s", d, err, out)
					}
					cmd = string(out)
					for _, want := range []string{"-tags integration", "-race", "MOMO_REQUIRE_INTEGRATION=1"} {
						if !strings.Contains(cmd, want) {
							t.Errorf("job %q dialect %s: command lacks %q:\n%s", name, d, want, cmd)
						}
					}
				}
				continue
			}
			for _, want := range []string{"-tags integration", "-race", "MOMO_REQUIRE_INTEGRATION=1"} {
				if !strings.Contains(cmd, want) {
					t.Errorf("job %q step lacks %q: %s", name, want, cmd)
				}
			}
		}
	}
	if !matrixHas(integ.Strategy.Matrix["dialect"], "postgres") {
		t.Error("the integration matrix does not enable postgres")
	}
	if pg := integ.Services["postgres"].Image; !strings.HasPrefix(pg, "pgvector/pgvector:") {
		t.Errorf("postgres service image = %q, want pgvector/pgvector", pg)
	}
	if !matrixHas(integ.Strategy.Matrix["pg"], "0.8.7-pg17") || !matrixHas(integ.Strategy.Matrix["pg"], "0.8.7-pg16") {
		t.Error("the integration matrix must test pgvector pg17 and one older major")
	}
	for _, name := range []string{"windows", "macos"} {
		if _, ok := w.Jobs[name]; !ok {
			t.Errorf("ci.yml has no %s job", name)
		}
	}
	if w.Jobs["windows"].RunsOn != "windows-latest" || w.Jobs["macos"].RunsOn != "macos-latest" {
		t.Error("windows and macos jobs must run on windows-latest and macos-latest")
	}
	if !usesAction(w.Jobs["govulncheck"], "golang/govulncheck-action@") {
		t.Error("govulncheck job must use the pinned govulncheck action")
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/repocheck/ -run '^TestCIWorkflowCoverage$' -count=1` fails with `open .../ci.yml: no such file or directory`.

- [ ] **Step 3: Write `.github/workflows/ci.yml`.** Top level: `name: ci`; `on: { push: { branches: [main] }, pull_request: {} }`; `permissions: { contents: read }`; `concurrency: { group: 'ci-${{ github.ref }}', cancel-in-progress: true }`. Every job sets `timeout-minutes` and uses these pinned actions: `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` with `persist-credentials: false`, and `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0` with `go-version: ${{ matrix.go }}`. Jobs:
  - `lint` (ubuntu-latest, go `1.26`): `make fmt-check`, `make vet`, `make tidy-check`.
  - `unit` (ubuntu-latest, matrix `go: ['1.26', '1.27']`): `make test`.
  - `compile-fail` (same matrix): `make test-compilefail`.
  - `race` (same matrix): `make test-race`.
  - `fuzz-smoke` (same matrix): `make test-fuzz FUZZTIME=10s`.
  - `bench-smoke` (go `1.27`): `make bench-smoke`.
  - `govulncheck` (go `1.27`): `golang/govulncheck-action@032d45514ae346b1db93c04b0c90b841c370344f # v1.1.0` with `go-version-input: '1.27'` and `go-package: ./...`.
  - `windows` (windows-latest, go `1.27`): `go test ./...`.
  - `macos` (macos-latest, go `1.27`): `CGO_ENABLED=1 go test ./...`.
  - `integration` (ubuntu-latest, timeout 30): `strategy.matrix` with `go: ['1.26', '1.27']`, `pg: ['0.8.7-pg17', '0.8.7-pg16']`, `dialect: [postgres]` and the milestone comment `# M2: add sqlite; M3: add mysql84, mysql97, mariadb`. The service `postgres` has `image: 'pgvector/pgvector:${{ matrix.pg }}'`, `env: { POSTGRES_PASSWORD: momo, POSTGRES_USER: postgres }`, `ports: ['5432:5432']` and `options: --health-cmd "pg_isready -U postgres" --health-interval 2s --health-retries 30`. Job `env`: `PG_URL: postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable` and `PG_APP_URL: postgres://momo_app:momo_app@127.0.0.1:5432/postgres?sslmode=disable`. Steps: checkout, setup-go, and `make test-integration-${{ matrix.dialect }}`, whose recipe is `MOMO_REQUIRE_INTEGRATION=1 ... go test -tags integration -race -count=1 ./...` (`TestCIWorkflowCoverage` runs `make -n test-integration-<dialect>` for each matrix dialect and asserts `-tags integration`, `-race` and `MOMO_REQUIRE_INTEGRATION=1` are present). The workflow also defines a service `pgbouncer` (`image: 'edoburu/pgbouncer:v1.26.1-p0'`, `env: { DATABASE_URL: 'postgres://postgres:momo@postgres:5432/postgres', POOL_MODE: transaction, AUTH_TYPE: trust, TRACK_EXTRA_PARAMETERS: search_path }`, `ports: ['6432:5432']`) and job `env` `PGBOUNCER_URL: postgres://postgres:momo@127.0.0.1:6432/postgres?sslmode=disable`; `TRACK_EXTRA_PARAMETERS: search_path` is required because `testdb.PgBouncerURL(t)` passes `search_path` as a startup parameter (PgBouncer 1.20 or later). The `pg` axis applies to every dialect; a dialect that does not use the PostgreSQL service may be excluded by its milestone plan.

  Add a comment at the top of the file: "The integration matrix is the only place a dialect is enabled in CI."

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -run '^TestCIWorkflowCoverage$' -count=1` prints `ok`. If `actionlint` is installed, `actionlint .github/workflows/ci.yml` prints nothing; otherwise the report records that it was not run.

- [ ] **Step 5: Commit.** `git add .github/workflows/ci.yml internal/repocheck go.mod go.sum && git commit -m "ci: add the CI workflow with the PostgreSQL integration matrix"`.

## Task 20: Security and supply-chain workflows

Delivers: S-25 (pinned third-party actions, least-privilege `permissions:`, Dependabot for Go modules and GitHub Actions, OpenSSF Scorecard), S-49 (CodeQL and gosec on pull requests).

Files:

- Create: `.github/workflows/codeql.yml`, `.github/workflows/gosec.yml`, `.github/workflows/scorecard.yml`
- Create: `.github/dependabot.yml`
- Create: `internal/repocheck/workflows_test.go`

Interfaces: none produced.

- [ ] **Step 1: Write the failing test.** Create `internal/repocheck/workflows_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var pinnedRef = regexp.MustCompile(`@[0-9a-f]{40}$`)

func TestWorkflowsPinned(t *testing.T) {
	dir := filepath.Join(repoRoot(t), ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		w := loadWorkflow(t, e.Name())
		if w.Permissions == nil {
			t.Errorf("%s declares no top-level permissions", e.Name())
		}
		for name, j := range w.Jobs {
			for _, s := range j.Steps {
				if s.Uses == "" || strings.HasPrefix(s.Uses, "./") || strings.HasPrefix(s.Uses, "docker://") {
					continue
				}
				if !pinnedRef.MatchString(s.Uses) {
					t.Errorf("%s job %s: %s is not pinned by a 40-character SHA", e.Name(), name, s.Uses)
				}
			}
		}
	}
}

func TestCodeQLAndGosecRunOnPullRequests(t *testing.T) {
	for _, name := range []string{"codeql.yml", "gosec.yml"} {
		w := loadWorkflow(t, name)
		if _, ok := w.On["pull_request"]; !ok {
			t.Errorf("%s does not run on pull_request", name)
		}
	}
	if _, err := os.Stat(filepath.Join(repoRoot(t), ".github", "dependabot.yml")); err != nil {
		t.Errorf("dependabot.yml missing: %v", err)
	}
	if _, ok := loadWorkflow(t, "scorecard.yml").On["schedule"]; !ok {
		t.Error("scorecard.yml must run on a schedule")
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/repocheck/ -run 'TestWorkflowsPinned|TestCodeQLAndGosecRunOnPullRequests' -count=1` fails with `codeql.yml` and `gosec.yml` not found.

- [ ] **Step 3: Write the files.**
  - `codeql.yml`: `on: { push: { branches: [main] }, pull_request: {}, schedule: [{ cron: '23 3 * * 1' }] }`; `permissions: { contents: read }`; job `analyze` (ubuntu-latest) with `permissions: { contents: read, security-events: write, actions: read }`; steps: checkout (pinned, `persist-credentials: false`), `github/codeql-action/init@9b3254085494b55a0f7c4c2b3d2d52cc712ce370 # v4.39.1` with `languages: go` and `queries: security-extended`, then `github/codeql-action/analyze@9b3254085494b55a0f7c4c2b3d2d52cc712ce370 # v4.39.1`.
  - `gosec.yml`: `on: { push: { branches: [main] }, pull_request: {} }`; `permissions: { contents: read }`; job `gosec` (ubuntu-latest) with `permissions: { contents: read, security-events: write }`; steps: checkout, `securego/gosec@deb54465fea23d19a77f037e11e6589021f8501d # v2.29.0` with `args: '-no-fail -fmt sarif -out gosec.sarif ./...'`, then `github/codeql-action/upload-sarif@9b3254085494b55a0f7c4c2b3d2d52cc712ce370 # v4.39.1` with `sarif_file: gosec.sarif`.
  - `scorecard.yml`: `on: { schedule: [{ cron: '17 4 * * 2' }], push: { branches: [main] }, branch_protection_rule: {} }`; `permissions: read-all`; job `analysis` (ubuntu-latest) with `permissions: { security-events: write, id-token: write, contents: read, actions: read }`; steps: checkout, `ossf/scorecard-action@fa7117db83c52d787ae5a89f350999701505d78e # v2.5.0` with `results_file: results.sarif`, `results_format: sarif`, `publish_results: true`; `actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2` with `name: SARIF file`, `path: results.sarif`, `retention-days: 5`; `github/codeql-action/upload-sarif@9b3254085494b55a0f7c4c2b3d2d52cc712ce370 # v4.39.1` with `sarif_file: results.sarif`.
  - `.github/dependabot.yml`: `version: 2` with `updates` entries for `package-ecosystem: gomodules` is wrong; use `package-ecosystem: gomod` directory `/` schedule weekly, and `package-ecosystem: github-actions` directory `/` schedule weekly. The docs Python requirements entry is added by the docs plan.

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -count=1` prints `ok` for every test in the package (`TestRepoFiles`, `TestMakefileTargets`, `TestCIWorkflowCoverage`, `TestWorkflowsPinned`, `TestCodeQLAndGosecRunOnPullRequests`).

- [ ] **Step 5: Commit.** `git add .github internal/repocheck && git commit -m "ci: add CodeQL, gosec, Scorecard and Dependabot with pinned actions"`.

## Task 21: Release automation and PR title check

Delivers: S-22 (release-please from Conventional Commits, pre-1.0 minor bumps, changelog path, PR-title enforcement), `TestReleaseConfig`.

Files:

- Create: `release-please-config.json`, `.release-please-manifest.json`
- Create: `.github/workflows/release-please.yml`, `.github/workflows/pr-title.yml`
- Create: `internal/repocheck/release_test.go`

Interfaces: none produced.

- [ ] **Step 1: Write the failing test.** Create `internal/repocheck/release_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseConfig(t *testing.T) {
	root := repoRoot(t)
	b, err := os.ReadFile(filepath.Join(root, "release-please-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["release-type"] != "go" {
		t.Errorf("release-type = %v, want go", cfg["release-type"])
	}
	if cfg["bump-minor-pre-major"] != true {
		t.Error("bump-minor-pre-major must be true: feat! bumps the minor before 1.0")
	}
	if cfg["changelog-path"] != "CHANGELOG.md" {
		t.Errorf("changelog-path = %v", cfg["changelog-path"])
	}
	if cfg["include-v-in-tag"] != true {
		t.Error("tags must be vX.Y.Z")
	}
	m, err := os.ReadFile(filepath.Join(root, ".release-please-manifest.json"))
	if err != nil || !strings.Contains(string(m), `"."`) {
		t.Errorf("manifest missing root package: %v", err)
	}
	w := loadWorkflow(t, "pr-title.yml")
	if _, ok := w.On["pull_request"]; !ok {
		t.Error("pr-title.yml must run on pull_request")
	}
	if !strings.Contains(stepUses(w), "amannn/action-semantic-pull-request@48f256284bd46cdaab1048c3721360e808335d50") {
		t.Error("pr-title.yml must use the pinned semantic PR action")
	}
	if _, err := os.Stat(filepath.Join(root, ".github", "workflows", "release-please.yml")); err != nil {
		t.Error(err)
	}
}

func stepUses(w workflow) string {
	var out strings.Builder
	for _, j := range w.Jobs {
		for _, s := range j.Steps {
			out.WriteString(s.Uses + "\n")
		}
	}
	return out.String()
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/repocheck/ -run '^TestReleaseConfig$' -count=1` fails with `open .../release-please-config.json: no such file or directory`.

- [ ] **Step 3: Write the files.**
  - `release-please-config.json`: `{"$schema": "https://raw.githubusercontent.com/googleapis/release-please/main/schemas/config.json", "release-type": "go", "bump-minor-pre-major": true, "bump-patch-for-minor-pre-major": false, "include-v-in-tag": true, "changelog-path": "CHANGELOG.md", "packages": {".": {}}}`.
  - `.release-please-manifest.json`: `{".": "0.0.0"}`. The first release PR proposes `0.1.0`; the spec allows that PR to be opened but the tag is cut only when all delivery stages are complete (the merge of that PR is the human decision).
  - `.github/workflows/release-please.yml`: `on: { push: { branches: [main] } }`; `permissions: { contents: write, pull-requests: write }`; job `release-please` (ubuntu-latest) with `googleapis/release-please-action@45996ed1f6d02564a971a2fa1b5860e934307cf7 # v5.0.0` and inputs `config-file: release-please-config.json` and `manifest-file: .release-please-manifest.json`.
  - `.github/workflows/pr-title.yml`: `on: { pull_request: { types: [opened, edited, synchronize, reopened] } }`; `permissions: { pull-requests: read }`; job `title` (ubuntu-latest) with `amannn/action-semantic-pull-request@48f256284bd46cdaab1048c3721360e808335d50 # v6.1.1` and `with: { types: "feat\nfix\nperf\nrefactor\ndocs\ntest\nbuild\nci\nchore\nrevert" }`, `requireScope: false`, `env: { GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} }`.

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add release-please-config.json .release-please-manifest.json .github internal/repocheck && git commit -m "ci: add release-please and Conventional Commit PR title checks"`.

## Task 22: Import-graph guard

Delivers: S-20 `TestImportGraph` (rule 1 of the architecture contract), and the statement that the core has no OpenTelemetry, driver, YAML or x/tools dependency.

Files:

- Create: `internal/repocheck/importgraph_test.go`

Interfaces: none produced.

- [ ] **Step 1: Write the test.** Create `internal/repocheck/importgraph_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package repocheck

import (
	"os/exec"
	"strings"
	"testing"
)

const modPath = "github.com/azrtydxb/momo"

// coreAllowed lists the module-internal packages the core may depend on.
var coreAllowed = map[string]bool{
	modPath:             true,
	modPath + "/dialect": true,
	modPath + "/schema":  true,
	modPath + "/vector":  true,
	modPath + "/types":   true,
}

func TestImportGraph(t *testing.T) {
	for _, pkg := range []string{modPath, modPath + "/dialect", modPath + "/schema", modPath + "/vector"} {
		cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", pkg)
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -deps %s: %v\n%s", pkg, err, out)
		}
		for _, dep := range strings.Fields(string(out)) {
			if !coreAllowed[dep] {
				t.Errorf("%s depends on %s, which the core may not import", pkg, dep)
			}
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL or PASS.** `go test ./internal/repocheck/ -run '^TestImportGraph$' -count=1`. Expected: PASS once `vector` exists (created in Task 8). If `go list` reports a missing package, the failure text names it; that is a Task 8 defect.

- [ ] **Step 3: Implement.** No production change. If a core package imports `github.com/...` outside the module, remove the import.

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add internal/repocheck/importgraph_test.go && git commit -m "test: guard the core import graph"`.

## Task 23: Foundation gate

Delivers: the acceptance state later plans rely on. No code is written in this task.

Files: none.

- [ ] **Step 1: Run the unit gate.** `make fmt-check vet tidy-check test test-race test-compilefail`. Expected: every target exits 0.

- [ ] **Step 2: Run the fuzz smoke.** `make test-fuzz FUZZTIME=10s`. Expected: `FuzzBuilder`, `FuzzSchemaSort` and `FuzzNormalizeType` each complete without a failure.

- [ ] **Step 3: Run the PostgreSQL integration gate.** `make db-up`, then `make test-integration-postgres`. Expected: exit 0; the suite is empty of dialect subtests until momo-03 adds the PostgreSQL entry to `testopen.Each`. Then `make db-down`.

- [ ] **Step 4: Confirm the contract names exist.** Run `go doc github.com/azrtydxb/momo Querier`, `go doc github.com/azrtydxb/momo Error`, `go doc github.com/azrtydxb/momo/schema Load` and `go doc github.com/azrtydxb/momo/dialect MySQLFlavor`. Each prints a declaration.

- [ ] **Step 5: No commit.** The gate result is the output of steps 1 to 4.

## Spec traceability

| Spec id    | Delivered by           | Acceptance test(s)                                                                                                                                                                          |
| ---------- | ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| S-1        | Tasks 4, 6, 7          | TestSchemaJSONRoundTrip, FuzzSchemaSort, TestSchemaEveryFieldSerialized, TestEqualIgnoresOrderAndConcurrently                                                                               |
| S-4 (unit) | Tasks 3, 9-17          | TestCompileFail, TestValidExamplesCompile, TestQueryReuseNoLeak, TestPlaceholdersAndQuoting, FuzzBuilder, TestRawPlaceholders, TestSelectGolden, TestKeysetPagination, TestIterClosesCursor |
| S-8        | Tasks 8, 16            | TestErrorSentinels, TestErrorIsAs, TestMigrateSentinelMessages, TestEmptyResultNotNotFound                                                                                                  |
| S-20       | Tasks 1, 2, 18, 19, 22 | TestCIWorkflowCoverage, TestMakefileTargets, TestImportGraph, TestResolve, TestEnvVarNames                                                                                                  |
| S-22       | Tasks 5, 21            | TestFormatVersions, TestReleaseConfig                                                                                                                                                       |
| S-24       | Task 1                 | TestRepoFiles                                                                                                                                                                               |
| S-25       | Tasks 2, 19, 20        | TestWorkflowsPinned, TestCIWorkflowCoverage (govulncheck)                                                                                                                                   |
| S-26       | Task 19                | TestCIWorkflowCoverage (windows job present; TestWindowsPaths is owned by plan 04 (CLI task))                                                                                               |
| S-49       | Task 20                | TestCodeQLAndGosecRunOnPullRequests                                                                                                                                                         |

Not covered here, by design: S-41 (driver-error redaction, with driver classification in momo-03), S-75 (soft-delete variants beyond the filter repair in Task 10), S-30 (PgBouncer query suite, momo-03), and the dialect-parameterized forms of the S-4 and S-8 tests, which run through `testopen.Each` once momo-03 lands.

## Contract gaps for the report

- The contract's `Schema` line lists `Schemas`, `Views`, `Functions`, `Triggers` and `Policies`; plan 04 Task 1 owns them, and plan 01 defines only the base fields.
- The contract does not state the `Columns` or `After` signatures; this plan uses the spike's `Columns(es ...Expr)` and the new `After(values ...any)`.
- The round-2 ownership decisions are applied: driver optional interfaces are momo-03's, `testopen` is momo-03's, and the root fixture is `unitDocument` in `unit_fixtures_test.go`.
- `Error()` still appends the wrapped error text until momo-03's classification and redaction tasks land; `Detail()` exposes the same text.
- The MySQL and MariaDB compose services are added by momo-08 in M3; the CI matrix enables each dialect with one entry.
