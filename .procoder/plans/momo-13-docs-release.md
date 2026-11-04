# momo-13-docs-release — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Ship the documentation site, the runnable examples, and the AI-facing documentation for Momo: MkDocs Material pages deployed with mike on GitHub Pages, `llms.txt` and `llms-full.txt` with a raw Markdown twin for every page, an agent usage guide with a copy-paste AGENTS.md snippet, and CI tests that compile every Go code block in the docs and run the examples end to end on PostgreSQL and SQLite.

## Architecture

Documentation sources stay under `docs/`, which is also the MkDocs `docs_dir`; the contract documents (`docs/ARCHITECTURE.md`) are published as-is. The examples are ordinary `package main` programs under `examples/`, shared through `examples/internal/open` and an embedded migrations tree, and the docs embed them with pymdownx snippets so the published code is the tested code. Examples and runnable documentation blocks run through `testopen.Each`, so each build covers the dialects it has (PostgreSQL in M1); Go tests in `internal/docscheck` read the source pages and the built site; the Python toolchain runs only in the docs workflow, never in the Go module CI. This plan runs last: it documents and exercises the APIs the earlier plans deliver, and every name it uses is taken from those plans' Interfaces blocks.

## Constraints

Copied verbatim from `.procoder/specs/momo.md` (Constraints section) where they apply to this plan:

- "Documentation site: https://azrtydxb.github.io/momo/ served by GitHub Pages from a `gh-pages` branch maintained by mike; MkDocs Material and mike are pinned in a docs requirements file and used only by the docs workflow (no Python in the Go module or library CI). Releases (release-please tags) publish `/vX.Y/` and move `/latest/`; pushes to main publish `/dev/`."
- "CI: GitHub Actions on free GitHub-hosted runners only (public repo): ubuntu-latest for most jobs, windows-latest for the SQLite/generator suite, PostgreSQL+pgvector as a `pgvector/pgvector` service container, SQLite+sqlite-vec via cgo on the runner; a macos-latest job builds and runs the SQLite suite to cover darwin/cgo. No self-hosted or paid larger runners. Benchmarks published as release evidence are run on a documented dedicated machine, because shared runners are too noisy for regression gating; CI only smoke-runs benchmarks."
- "Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled."
- "License: Apache-2.0."
- "Module path: github.com/azrtydxb/momo; public repository https://github.com/azrtydxb/momo."
- "CLI distribution: `go install github.com/azrtydxb/momo/cmd/momo@vX.Y.Z` only, so the CLI and library share one version; no prebuilt binaries."

Pinned versions, verified 2026-11-04 (each published at least two weeks earlier; plan 01 holds the Actions SHAs and the container tags):

| Item | Pin |
|---|---|
| mkdocs | `1.6.1` |
| mkdocs-material | `9.7.7` (9.8.0 was published 2026-10-24 and is excluded) |
| mike | `2.2.0` |
| pip-tools (generates the hashed lock) | `7.6.2` (published 2026-10-07) |
| Python for docs jobs | `3.12` |
| actions/setup-python | `actions/setup-python@6d4fd22b20465ccc0f7354476588e8df7c9ca0e0 # v7.1.0` |

Paths owned by other plans and used here: `docs/migrations.md` (momo-04 Task 35), `docs/vectors.md` (momo-07), `docs/guides/gorm.md`, `docs/guides/bun.md`, `docs/guides/ent.md` (momo-11 Task 14), the capability matrix page (momo-08 Task 8). Every path must exist with exactly that name when this plan runs; `TestEveryDocPageInNav` fails otherwise.

## Task 1: Docs toolchain pins and Dependabot for Python

Delivers: S-23 (pinned MkDocs Material and mike in a docs requirements file used only by the docs workflow), S-25 (Dependabot for the docs Python requirements).

Files:
- Create: `docs/requirements.in`
- Create: `docs/requirements.txt` (generated, hashed)
- Modify: `.github/dependabot.yml` (created in plan 01 Task 20; add one entry)
- Create: `internal/docscheck/requirements_test.go` (package `docscheck`, with the helper `repoRoot`)

Interfaces: `func repoRoot(t *testing.T) string` in package `docscheck` returns the module root; the package has no production code yet.

- [ ] **Step 1: Write the failing test.** Create `internal/docscheck/requirements_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

func hashedPin(name, version string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `==` + regexp.QuoteMeta(version) + ` \\\n\s+--hash=sha256:[0-9a-f]{64}`)
}

func TestDocsRequirementsPinned(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for name, version := range map[string]string{"mkdocs": "1.6.1", "mkdocs-material": "9.7.7", "mike": "2.2.0"} {
		if !hashedPin(name, version).MatchString(string(b)) {
			t.Errorf("docs/requirements.txt does not pin %s==%s with a sha256 hash", name, version)
		}
	}
	d, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "dependabot.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(d), "package-ecosystem: pip") || !strings.Contains(string(d), "directory: /docs") {
		t.Error("dependabot.yml has no pip entry for /docs")
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/docscheck/ -run '^TestDocsRequirementsPinned$' -count=1` fails with `open .../docs/requirements.txt: no such file or directory`.

- [ ] **Step 3: Create the requirements and the lock.** Create `docs/requirements.in` with exactly these lines:

```text
mkdocs==1.6.1
mkdocs-material==9.7.7
mike==2.2.0
```

Then, with Python 3.12 in a scratch virtual environment outside the repository, run `python3.12 -m venv "$SCRATCH/venv" && "$SCRATCH/venv/bin/pip" install pip-tools==7.6.2 && "$SCRATCH/venv/bin/pip-compile" --generate-hashes --strip-extras --output-file docs/requirements.txt docs/requirements.in`, where `$SCRATCH` is the session scratchpad directory. The output keeps the three direct pins and adds hashes for every transitive package. Confirm `grep -c "sha256:" docs/requirements.txt` prints a number greater than 3.

- [ ] **Step 4: Add the Dependabot entry.** In `.github/dependabot.yml`, add a second `updates` entry: `package-ecosystem: pip`, `directory: /docs`, `schedule: { interval: weekly }`.

- [ ] **Step 5: Run and expect PASS.** `go test ./internal/docscheck/ -run '^TestDocsRequirementsPinned$' -count=1` prints `ok`.

- [ ] **Step 6: Commit.** `git add docs/requirements.in docs/requirements.txt .github/dependabot.yml internal/docscheck && git commit -m "build(docs): pin MkDocs Material and mike with hashes and track them in Dependabot"`.

## Task 2: MkDocs configuration, navigation and version selector

Delivers: S-23 (MkDocs Material site, navigation over every page, the mike version selector, the llms hook registration point), plus the rule that every page in `docs/` is reachable from the navigation.

Files:
- Create: `mkdocs.yml`
- Create: `internal/docscheck/nav.go` (non-test: `NavPages`)
- Create: `internal/docscheck/nav_test.go`

Interfaces (produced, exact):
- `func NavPages(root string) ([]string, error)` returns every `.md` path listed in the `nav` of `<root>/mkdocs.yml`, in document order, relative to `docs/`.

- [ ] **Step 1: Write the failing test.** Create `internal/docscheck/nav_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMkdocsNavPagesExist(t *testing.T) {
	root := repoRoot(t)
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 20 {
		t.Fatalf("nav lists %d pages, want at least 20", len(pages))
	}
	for _, p := range pages {
		if _, err := os.Stat(filepath.Join(root, "docs", p)); err != nil {
			t.Errorf("nav page %s: %v", p, err)
		}
	}
}

func TestEveryDocPageInNav(t *testing.T) {
	root := repoRoot(t)
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	inNav := map[string]bool{}
	for _, p := range pages {
		inNav[filepath.ToSlash(p)] = true
	}
	docs := filepath.Join(root, "docs")
	err = filepath.WalkDir(docs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, err := filepath.Rel(docs, path)
		if err != nil {
			return err
		}
		if !inNav[filepath.ToSlash(rel)] {
			t.Errorf("docs/%s is not in the mkdocs.yml nav", filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/docscheck/ -run 'TestMkdocsNavPagesExist|TestEveryDocPageInNav' -count=1` fails with `undefined: NavPages`.

- [ ] **Step 3: Implement `internal/docscheck/nav.go` and `mkdocs.yml`.** `NavPages` reads `mkdocs.yml` with `go.yaml.in/yaml/v3` into `struct{ Nav []any \`yaml:"nav"\` }`, walks the value recursively (a string ending in `.md` is a page; a map contributes the values of its single key; a slice is walked in order), and returns the pages. Add the module requirement with `go get go.yaml.in/yaml/v3@v3.0.6` if plan 01 did not already add it.

  `mkdocs.yml` content:

```yaml
site_name: momo
site_url: https://azrtydxb.github.io/momo/
site_description: Go ORM with typed queries, explicit SQL and a self-contained migration engine
repo_url: https://github.com/azrtydxb/momo
docs_dir: docs
exclude_docs: |
  requirements.in
  requirements.txt
theme:
  name: material
  features:
    - navigation.sections
    - navigation.top
    - content.code.copy
  palette:
    - scheme: default
      media: "(prefers-color-scheme: light)"
    - scheme: slate
      media: "(prefers-color-scheme: dark)"
hooks:
  - tools/docs/llms_hook.py
plugins:
  - search
markdown_extensions:
  - admonition
  - attr_list
  - toc:
      permalink: true
  - pymdownx.details
  - pymdownx.superfences
  - pymdownx.tabbed:
      alternate_style: true
  - pymdownx.snippets:
      base_path:
        - .
        - docs
      check_paths: true
extra:
  version:
    provider: mike
    default: latest
nav:
  - Home: README.md
  - Getting started: getting-started.md
  - Concepts:
      - Models: models.md
      - Queries: queries.md
      - Writes: writes.md
      - Relations: relations.md
      - Transactions: transactions.md
      - Errors: errors.md
      - Types: types.md
      - Raw SQL: raw-sql.md
      - Observability: observability.md
  - Vectors: vectors.md
  - MySQL and MariaDB: mysql.md
  - Migrations: migrations.md
  - Examples: examples.md
  - Guides:
      - GORM: guides/gorm.md
      - Bun: guides/bun.md
      - Ent: guides/ent.md
  - Reference:
      - Capability matrix: capability-matrix.md
      - Supported versions: supported-versions.md
      - Performance: PERFORMANCE.md
      - Architecture: ARCHITECTURE.md
  - Contributing:
      - Contributor guide: contributing.md
      - Agent usage guide: agents.md
```

- [ ] **Step 4: Run and expect FAIL on missing pages, then add them in Tasks 3 to 7.** `go test ./internal/docscheck/ -run 'TestMkdocsNavPagesExist|TestEveryDocPageInNav' -count=1` now reports the nav pages that Tasks 3 to 7 create (`getting-started.md` and the others); it must report nothing else. Record the list; the pages are written in Task 4 and Task 7.

- [ ] **Step 5: Commit after Task 4 and Task 7 pass.** Commit the configuration with the pages: `git add mkdocs.yml internal/docscheck docs && git commit -m "docs: add the MkDocs site, navigation and version selector"`.

## Task 3: Runnable examples, run through testopen.Each

Delivers: S-21 runnable examples for CRUD, zero/NULL updates, transactions, relations, projection and pagination, vector search, create/upgrade a database, custom SQL migration, and adopting an existing schema. Each example is a library function `Run(ctx context.Context, db *momo.DB) error`, so the test runs it against the database that `testopen.Each` provides; there are no example binaries. In M1 only the PostgreSQL entry exists, so the examples run on PostgreSQL; SQLite and MySQL join when their milestone entries land, with no change here.

Files:
- Create: `examples/momo.yaml`
- Create: `examples/migrations/embed.go` (package `migrations`) and the generated `examples/migrations/postgres/*` files; the sqlite and mysql directories are generated by their milestones
- Create: `examples/internal/open/open.go` (package `open`)
- Create: `examples/crud/example.go` (package `crud`), `examples/zero-null-updates/example.go` (package `zeronull`), `examples/transactions/example.go` (package `transactions`), `examples/relations/example.go` (package `relations`), `examples/projection-pagination/example.go` (package `projection`), `examples/vector-search/example.go` (package `vectorsearch`), `examples/create-upgrade-db/example.go` (package `upgradedb`), `examples/custom-sql-migration/example.go` (package `custommigration`), `examples/adopt-existing-schema/example.go` (package `adoptschema`)
- Create: `examples/examples_test.go` (package `examples`)

Interfaces (produced, exact):
- `package migrations`: `var FS embed.FS` embedding `postgres` (and later `sqlite`, `mysql`).
- `package open`: `func Migrate(ctx context.Context, db *momo.DB) error` applies the embedded migrations for `db.Dialect().Name`: it takes `fs.Sub(migrations.FS, <dialect>)`, calls `migrate.NewEngine(db.Driver(), dir, <the migrate dialect implementation for that driver, as momo-04 exports it>)`, then `Apply(ctx, migrate.ApplyOptions{})`. It is idempotent: a second call applies nothing.
- Each example package exports `func Run(ctx context.Context, db *momo.DB) error`. An example calls `open.Migrate` first, except `create-upgrade-db` and `adopt-existing-schema`, which build the schema themselves on the empty database they receive. Examples do not assume empty tables: each creates its own tenant with a unique name.

- [ ] **Step 1: Write the failing test.** Create `examples/examples_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package examples

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb/testopen"

	adoptschema "github.com/azrtydxb/momo/examples/adopt-existing-schema"
	"github.com/azrtydxb/momo/examples/crud"
	upgradedb "github.com/azrtydxb/momo/examples/create-upgrade-db"
	custommigration "github.com/azrtydxb/momo/examples/custom-sql-migration"
	projection "github.com/azrtydxb/momo/examples/projection-pagination"
	"github.com/azrtydxb/momo/examples/relations"
	vectorsearch "github.com/azrtydxb/momo/examples/vector-search"
	zeronull "github.com/azrtydxb/momo/examples/zero-null-updates"
	"github.com/azrtydxb/momo/examples/transactions"
)

// runs registers every example. discover fails when an example directory
// is not registered here, so an example cannot exist without running.
var runs = map[string]func(context.Context, *momo.DB) error{
	"crud":                  crud.Run,
	"zero-null-updates":     zeronull.Run,
	"transactions":          transactions.Run,
	"relations":             relations.Run,
	"projection-pagination": projection.Run,
	"vector-search":         vectorsearch.Run,
	"create-upgrade-db":     upgradedb.Run,
	"custom-sql-migration":  custommigration.Run,
	"adopt-existing-schema": adoptschema.Run,
}

func discover(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "internal" || e.Name() == "migrations" {
			continue
		}
		if _, err := os.Stat(filepath.Join(e.Name(), "example.go")); err == nil {
			names = append(names, e.Name())
		}
	}
	if len(names) != len(runs) {
		t.Fatalf("found %d example directories, %d registered: %v", len(names), len(runs), names)
	}
	for _, n := range names {
		if runs[n] == nil {
			t.Fatalf("example %s is not registered in examples_test.go", n)
		}
	}
	return names
}

func TestExamples(t *testing.T) {
	for _, name := range discover(t) {
		run := runs[name]
		t.Run(name, func(t *testing.T) {
			testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
				if err := run(context.Background(), db); err != nil {
					t.Fatalf("example %s on %s: %v", name, d, err)
				}
			})
		})
	}
}
```

Note: the import of the directory `create-upgrade-db` needs an alias, `upgradedb "github.com/azrtydxb/momo/examples/create-upgrade-db"`; use that alias in the import block before running the test.

- [ ] **Step 2: Run and expect FAIL.** `make test-integration-postgres` fails at build time with `package .../examples/adopt-existing-schema is not in std` or `no Go files`, because the example packages do not exist yet.

- [ ] **Step 3: Generate the initial migrations.** Write `examples/momo.yaml` in the single schema of momo-02 Task 21 (`config.File`, with the `migrate:` section of type `config.MigrateConfig`):

```yaml
version: 1
packages:
  - ./internal/testmodels/example
dialects: [postgres]
migrate:
  dir:
    postgres: examples/migrations/postgres
  url_env:
    postgres: MOMO_POSTGRES_URL
```

  Semantics of the `migrate:` keys are momo-04's. Run `go run ./cmd/momo migrate diff initial --config examples/momo.yaml`, selecting the PostgreSQL dialect as momo-04 Task 33 documents. Run `go run ./cmd/momo migrate validate --config examples/momo.yaml`; it exits 0. Create `examples/migrations/embed.go` with `//go:embed postgres` on `var FS embed.FS`.

- [ ] **Step 4: Write `examples/internal/open/open.go`.** Implement `Migrate` as the Interfaces block describes. Use `errors` wrapping, not `log.Fatal`.

- [ ] **Step 5: Write the nine examples.** Each `example.go` has the same shape: `package <name>`, the doc comment, `Run` opening with `open.Migrate` (where the Interfaces block says so), the body, and `return nil`. The CRUD example is given literally; the others follow the bodies described below.

  `examples/crud/example.go`:

```go
// SPDX-License-Identifier: Apache-2.0

// Package crud creates, reads, updates and deletes a document.
package crud

import (
	"context"
	"fmt"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/examples/internal/open"
	"github.com/azrtydxb/momo/internal/testmodels/example"
	"github.com/azrtydxb/momo/internal/testmodels/example/document"
)

// Run performs one create, read, update and delete round trip.
func Run(ctx context.Context, db *momo.DB) error {
	if err := open.Migrate(ctx, db); err != nil {
		return err
	}
	tenant := example.Tenant{Name: fmt.Sprintf("acme-%d", time.Now().UnixNano())}
	if _, err := momo.Insert[example.Tenant](db).Model(&tenant).Exec(ctx); err != nil {
		return err
	}
	doc := example.Document{TenantID: tenant.ID, Title: "hello", Body: "first", Status: "draft"}
	if _, err := momo.Insert[example.Document](db).Model(&doc).Exec(ctx); err != nil {
		return err
	}
	got, err := momo.Select[example.Document](db).Where(document.ID.Eq(doc.ID)).One(ctx)
	if err != nil {
		return err
	}
	if got.Body != "first" {
		return fmt.Errorf("read back body %q", got.Body)
	}
	if _, err := momo.Update[example.Document](db).Set(document.Status.Set("published")).Where(document.ID.Eq(got.ID)).Exec(ctx); err != nil {
		return err
	}
	_, err = momo.Delete[example.Document](db).Where(document.ID.Eq(got.ID)).Exec(ctx)
	return err
}
```

  The tenant name carries a nanosecond timestamp so that repeated runs on one database do not collide. The field names (`Tenant.Name`, `Document.Status`) are those declared in `internal/testmodels/example/models.go` (momo-02 Task 22); the test catches a mismatch.

  The other eight examples:
  - `zero-null-updates`: insert a document with `Body` set to a non-empty value; update with `document.Body.Set("")`, with `document.Body.SetNull()` (the column is nullable), and with `document.Body.SetDefault()`; select after each and return an error unless the three stored values are distinct.
  - `transactions`: `db.Tx(ctx, momo.TxOptions{}, fn)` commits when `fn` returns nil; a `fn` that returns an error leaves no row; a `fn` that panics leaves no row and the panic reaches the caller (recovered in the example and checked); a failing nested `tx.Tx` savepoint keeps the outer insert.
  - `relations`: insert a tenant, a document and two comments; load the comments with momo.Load, passing the document slice and the relation descriptor named `rel` that the generated package exposes for `Document.Comments` (momo-06 Task 3 defines the descriptor); assert two comments are attached; repeat with `momo.Select[example.Document]` on db, followed by `.Preload(rel).All(ctx)` and assert the same result.
  - `projection-pagination`: insert five documents for one tenant; select a projection of title and status (the form momo-05 Task 5 defines) and page through them with `After` and `Limit(2)` on the document ID; return an error unless the three pages hold five distinct IDs in order.
  - `vector-search`: insert three documents whose `Embedding` values are one-hot 1536-dimension `vector.Vector` values; order by the `CosineDistance` expression on the `Embedding` column (the function momo-07 Task 3 defines) with `Limit(1)`; return an error unless the nearest document is the one whose one-hot position matches the query.
  - `create-upgrade-db`: on the empty database run `open.Migrate` twice; the second run must apply nothing; compare the `Status` results of the two runs.
  - `custom-sql-migration`: register a Go migration with `migrate.WithGoMigrations(migrate.GoMigration{Version: "20260601000000", Name: "backfill_status", Up: ...})`, where `Up` runs `UPDATE documents SET status = 'draft' WHERE status IS NULL` through `momo.Exec`; apply it; return an error unless the history row has kind `go` (the type is momo-11 Task 13's).
  - `adopt-existing-schema`: on the empty database execute the SQL of the initial migration with `momo.Exec`, then `Engine.Baseline(ctx, <initial version>, false)`, then `Engine.Drift(ctx)`; return an error unless the drift report is empty.

- [ ] **Step 6: Run and expect PASS.** `make db-up`, then `make test-integration-postgres` runs `TestExamples/<name>/postgres` for all nine examples and each prints `ok`. Then `make db-down`.

- [ ] **Step 7: Commit.** `git add examples && git commit -m "docs(examples): runnable examples run through testopen.Each"`.

## Task 4: Core documentation pages and the docs index

Delivers: the S-21 pages this plan owns: the site home and index, getting started, the concept pages, the examples page, the capability matrix (generated from the `dialect` package), supported versions, the contributor guide. Feature plans own `docs/migrations.md` (momo-04), `docs/vectors.md` (momo-07), `docs/mysql.md` (momo-08) and `docs/PERFORMANCE.md` (momo-12); this task links them and does not write them.

Files:
- Modify: `docs/README.md` (rewritten as the site home and index)
- Create: `docs/getting-started.md`, `docs/models.md`, `docs/queries.md`, `docs/writes.md`, `docs/relations.md`, `docs/transactions.md`, `docs/errors.md`, `docs/types.md`, `docs/raw-sql.md`, `docs/observability.md`, `docs/examples.md`, `docs/capability-matrix.md`, `docs/supported-versions.md`, `docs/contributing.md`
- Create: `internal/docscheck/index_test.go`, `internal/docscheck/capability_test.go`

Interfaces: none produced.

- [ ] **Step 1: Write the failing tests.** Create `internal/docscheck/index_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsIndexLinksEveryPage(t *testing.T) {
	root := repoRoot(t)
	idx, err := os.ReadFile(filepath.Join(root, "docs", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if p == "README.md" {
			continue
		}
		if !strings.Contains(string(idx), "("+p+")") && !strings.Contains(string(idx), "("+p+"#") {
			t.Errorf("docs/README.md does not link %s", p)
		}
	}
}
```

Create `internal/docscheck/capability_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
)

// TestCapabilityMatrixComplete fails when a capability boolean of
// dialect.Dialect has no row in docs/capability-matrix.md.
func TestCapabilityMatrixComplete(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "capability-matrix.md"))
	if err != nil {
		t.Fatal(err)
	}
	rt := reflect.TypeOf(dialect.Dialect{})
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Type.Kind() != reflect.Bool {
			continue
		}
		if !strings.Contains(string(b), "`"+f.Name+"`") {
			t.Errorf("capability-matrix.md has no row for %s", f.Name)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/docscheck/ -run 'TestDocsIndexLinksEveryPage|TestCapabilityMatrixComplete' -count=1` fails with `docs/README.md does not link getting-started.md` and `open .../docs/capability-matrix.md: no such file or directory`.

- [ ] **Step 3: Write the pages.** Each page is written from the code the earlier plans deliver; every Go code block is a complete `package main` program, because Task 5 compiles it.
  - `README.md`: what momo is (typed queries from ordinary structs, explicit SQL, migrations with reviewable SQL, vectors as part of the model); the supported databases as recorded in `supported-versions.md`; an index with one entry per page, each a Markdown link whose text is the page title and whose target is the file name.
  - `getting-started.md`: `go get github.com/azrtydxb/momo`, a `go generate` line, and the CRUD example included by snippet (`--8<-- "examples/crud/example.go"`, a bare line outside any fence).
  - `models.md`, `queries.md`, `writes.md`, `relations.md`, `transactions.md`, `errors.md`, `types.md`, `raw-sql.md`, `observability.md`: one page per concept, each with a "Contract" section quoting the guarantee the spec states for that concept (for example, writes: "omitted, zero, NULL and DEFAULT are four distinct states"), the typed API names of the generated code and the root package, and a "Failure modes" section. `errors.md` lists every sentinel in `errors.go`. `raw-sql.md` documents `?`, `??` and the value/identifier split, and states that raw SQL is the explicit escape hatch.
  - `examples.md`: one section per example, each including its file by snippet (`--8<-- "examples/<name>/example.go"`).
  - `capability-matrix.md`: one row per capability boolean of `dialect.Dialect`, each name in backticks, with the value for PostgreSQL, SQLite and each MySQL flavour, copied from `dialect/dialect.go`. The test above keeps it complete.
  - `supported-versions.md`: a table of each database, version, image tag and CI job, sourced from `internal/testdb/compose.yaml`, the `integration` matrix in `.github/workflows/ci.yml`, and the Go and cgo requirements in the spec's Constraints.
  - `contributing.md`: `--8<-- "CONTRIBUTING.md"` (snippet include of the root file).

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/docscheck/ -run 'TestDocsIndexLinksEveryPage|TestCapabilityMatrixComplete' -count=1` prints `ok`. Pages linked from the nav but written by feature plans must exist when this task runs; `TestEveryDocPageInNav` (Task 2) reports any that do not.

- [ ] **Step 5: Commit with Task 2 and Task 7.** Those tasks complete the navigation; commit once all three pass: `git add docs internal/docscheck && git commit -m "docs: core concept pages, capability matrix and the docs index"`.

## Task 5: Compile and run every Go code block in the docs

Delivers: S-23 `TestDocsCodeBlocks`, the only code-block extractor in the repository (plan 11 uses this one). Every Go block compiles against the current module; runnable blocks execute against each dialect that `testopen.Each` provides.

Files:
- Create: `internal/docscheck/codeblocks_test.go`

Interfaces: a fenced block whose info string is `go` is a complete `package main` program that must compile. A block whose info string is `go run` must also run to exit 0, with `MOMO_EXAMPLE_DRIVER` and `MOMO_EXAMPLE_DSN` set for the dialect under test. Runnable blocks open their own connection from those variables and create their own schema, because `examples/internal` is not importable from the generated packages. Listings that are not programs use the info string `text`.

- [ ] **Step 1: Write the failing test.** Create `internal/docscheck/codeblocks_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/testdb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

var goFence = regexp.MustCompile("^```go( run)?\\s*$")

type docBlock struct {
	file string
	line int
	run  bool
	src  string
}

func docBlocks(t *testing.T, root string) []docBlock {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	guides, err := filepath.Glob(filepath.Join(root, "docs", "*", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	files = append(files, guides...)
	var out []docBlock
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(b), "\n")
		for i := 0; i < len(lines); i++ {
			m := goFence.FindStringSubmatch(lines[i])
			if m == nil {
				continue
			}
			blk := docBlock{file: f, line: i + 1, run: m[1] != ""}
			var body []string
			for i++; i < len(lines) && strings.TrimSpace(lines[i]) != "```"; i++ {
				body = append(body, lines[i])
			}
			blk.src = strings.Join(body, "\n") + "\n"
			out = append(out, blk)
		}
	}
	return out
}

// writeBlock places block n in its own package under internal/docscheck/gen.
func writeBlock(t *testing.T, root string, n int, src string) string {
	t.Helper()
	dir := filepath.Join(root, "internal", "docscheck", "gen", fmt.Sprintf("b%03d", n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return "./internal/docscheck/gen/" + filepath.Base(dir)
}

func TestDocsCodeBlocks(t *testing.T) {
	root := repoRoot(t)
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "internal", "docscheck", "gen")) })
	blocks := docBlocks(t, root)
	if len(blocks) == 0 {
		t.Fatal("no go code blocks found under docs/")
	}
	for n, b := range blocks {
		pkg := writeBlock(t, root, n, b.src)
		cmd := exec.Command("go", "build", "-o", os.DevNull, pkg)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("%s:%d does not compile:\n%s", b.file, b.line, out)
		}
	}
}

// dsnFor returns the DSN of the database testopen.Each provides for d.
// Dialects added by later milestones add their case here.
func dsnFor(t testing.TB, d dialect.Name) string {
	switch d {
	case dialect.Postgres:
		return testdb.PostgresURL(t)
	case dialect.SQLite:
		return testdb.SQLitePath(t)
	}
	t.Fatalf("no DSN for dialect %s", d)
	return ""
}

func TestDocsRunnableBlocks(t *testing.T) {
	root := repoRoot(t)
	testopen.Each(t, func(t *testing.T, db *momo.DB, d dialect.Name) {
		t.Cleanup(func() { _ = os.RemoveAll(filepath.Join(root, "internal", "docscheck", "gen")) })
		for n, b := range docBlocks(t, root) {
			if !b.run {
				continue
			}
			pkg := writeBlock(t, root, n, b.src)
			cmd := exec.Command("go", "run", pkg)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "MOMO_EXAMPLE_DRIVER="+string(d), "MOMO_EXAMPLE_DSN="+dsnFor(t, d))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s:%d failed on %s: %v\n%s", b.file, b.line, d, err, out)
			}
		}
	})
}
```

The import of `momo` is needed by the `Each` callback signature: add `"github.com/azrtydxb/momo"` to the import block.

- [ ] **Step 2: Run and expect FAIL or PASS.** `go test ./internal/docscheck/ -run '^TestDocsCodeBlocks$' -count=1`. Before Task 4 the docs have no Go blocks and the test fails with `no go code blocks found under docs/`; after Task 4 it reports every block that does not compile. Fix the pages, not the test, until it passes.

- [ ] **Step 3: Confirm the runnable path.** `make db-up`, then `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL='postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable' go test ./internal/docscheck/ -run '^TestDocsRunnableBlocks$' -count=1` passes; the dialect subtest is `postgres`.

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/docscheck/ -run '^TestDocsCodeBlocks$' -count=1` prints `ok`.

- [ ] **Step 5: Commit.** `git add internal/docscheck docs && git commit -m "test(docs): compile every Go code block and run the marked ones per dialect"`.

## Task 6: llms.txt, llms-full.txt and raw Markdown twins

Delivers: S-23 `/llms.txt` index, `/llms-full.txt` full-text bundle, raw `.md` twin for every page, all generated at build time, and the `TestLLMsTxt` acceptance test.

Files:
- Create: `tools/docs/llms_hook.py` (MkDocs hook; registered in `mkdocs.yml` in Task 2)
- Create: `internal/docscheck/llms_test.go` (build tag `docs`)

Interfaces: the hook writes `site/llms.txt`, `site/llms-full.txt` and `site/<src_uri>` for every page in `config["nav"].pages`. The test reads the built site from the environment variable `MOMO_DOCS_SITE`.

- [ ] **Step 1: Write the failing test.** Create `internal/docscheck/llms_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

//go:build docs

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLLMsTxt(t *testing.T) {
	root := repoRoot(t)
	site := os.Getenv("MOMO_DOCS_SITE")
	if site == "" {
		t.Fatal("MOMO_DOCS_SITE must point at the built site directory")
	}
	llms, err := os.ReadFile(filepath.Join(site, "llms.txt"))
	if err != nil {
		t.Fatal(err)
	}
	full, err := os.ReadFile(filepath.Join(site, "llms-full.txt"))
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NavPages(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pages {
		if !strings.Contains(string(llms), "/"+p+")") {
			t.Errorf("llms.txt does not list %s", p)
		}
		src, err := os.ReadFile(filepath.Join(root, "docs", p))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(full), "# Source: "+p+"\n") || !strings.Contains(string(full), string(src)) {
			t.Errorf("llms-full.txt lacks the Markdown of %s", p)
		}
		twin, err := os.ReadFile(filepath.Join(site, p))
		if err != nil {
			t.Errorf("no raw Markdown twin for %s: %v", p, err)
			continue
		}
		if string(twin) != string(src) {
			t.Errorf("raw Markdown twin of %s differs from its source", p)
		}
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test -tags docs ./internal/docscheck/ -run '^TestLLMsTxt$' -count=1` fails with `MOMO_DOCS_SITE must point at the built site directory`. With `MOMO_DOCS_SITE` set to a directory that lacks the hook output it fails with `open .../llms.txt: no such file or directory`.

- [ ] **Step 3: Implement the hook.** Create `tools/docs/llms_hook.py`:

```python
# SPDX-License-Identifier: Apache-2.0
"""MkDocs hook: writes llms.txt, llms-full.txt and raw Markdown twins."""

import os
import shutil


def on_post_build(config, **kwargs):
    site = config["site_dir"]
    docs = config["docs_dir"]
    base = config["site_url"].rstrip("/")
    pages = [(p.title or p.file.src_uri, p.file.src_uri) for p in config["nav"].pages]

    lines = [
        "# momo",
        "",
        "> Go ORM with typed queries, explicit SQL and a self-contained migration engine.",
        "",
        "## Pages",
        "",
    ]
    full = []
    for title, src in pages:
        lines.append(f"- [{title}]({base}/{src})")
        source = os.path.join(docs, src)
        with open(source, encoding="utf-8") as f:
            full.append(f"# Source: {src}\n\n{f.read()}\n")
        twin = os.path.join(site, src)
        os.makedirs(os.path.dirname(twin), exist_ok=True)
        shutil.copyfile(source, twin)

    with open(os.path.join(site, "llms.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(lines) + "\n")
    with open(os.path.join(site, "llms-full.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(full))
```

- [ ] **Step 4: Build the site and run and expect PASS.** Locally: `python3.12 -m venv "$SCRATCH/venv" && "$SCRATCH/venv/bin/pip" install --require-hashes -r docs/requirements.txt && "$SCRATCH/venv/bin/mkdocs" build --strict --site-dir "$SCRATCH/site"`, then `MOMO_DOCS_SITE="$SCRATCH/site" go test -tags docs ./internal/docscheck/ -run '^TestLLMsTxt$' -count=1` prints `ok`. Expected: `mkdocs build --strict` exits 0.

- [ ] **Step 5: Commit.** `git add tools/docs internal/docscheck/llms_test.go && git commit -m "docs: generate llms.txt, llms-full.txt and raw Markdown twins at build time"`.

## Task 7: Agent usage guide and the AGENTS.md snippet

Delivers: S-23 agent usage guide with canonical patterns, do/don't rules and a copy-paste AGENTS.md snippet for consumer repositories.

Files:
- Create: `docs/agents.md`
- Create: `internal/docscheck/agents_test.go`

Interfaces: `docs/agents.md` contains exactly one snippet block between the markers `<!-- momo:agents-snippet:start -->` and `<!-- momo:agents-snippet:end -->`; the text between the markers is what consumers paste into their `AGENTS.md`.

- [ ] **Step 1: Write the failing test.** Create `internal/docscheck/agents_test.go`:

```go
// SPDX-License-Identifier: Apache-2.0

package docscheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var agentRules = []string{
	"Never build SQL by string concatenation",
	"identifiers are quoted and values are bound",
	"Use the generated column descriptors",
	"Do not pass user input to `momo.Raw` or `momo.SQL`",
	"Run `momo generate --check` in CI",
	"Do not edit a migration that has been applied",
	"Run `momo migrate validate` in CI",
	"Never call `Update` or `Delete` without a `Where` clause",
}

func TestAgentGuideSnippet(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	const start, end = "<!-- momo:agents-snippet:start -->", "<!-- momo:agents-snippet:end -->"
	i, j := strings.Index(text, start), strings.Index(text, end)
	if i < 0 || j < i {
		t.Fatal("docs/agents.md lacks the agents-snippet markers")
	}
	snippet := text[i+len(start) : j]
	for _, rule := range agentRules {
		if !strings.Contains(snippet, rule) {
			t.Errorf("AGENTS.md snippet lacks the rule %q", rule)
		}
	}
	if strings.Count(text, start) != 1 {
		t.Error("the snippet must appear exactly once")
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/docscheck/ -run '^TestAgentGuideSnippet$' -count=1` fails with `docs/agents.md lacks the agents-snippet markers`.

- [ ] **Step 3: Write `docs/agents.md`.** Sections in this order: "What this guide is for" (the audience is coding agents writing code that uses Momo); "Canonical patterns" (one fenced `go` block each, every block a complete program: a typed select with a generated descriptor, a write with explicit `Omit`, a transaction callback, a relation load with `momo.Load`, a keyset page with `After`, a migration generated by `momo migrate diff`); "Do" and "Don't" lists, with each rule from `agentRules` written as an item of the Do or Don't list; "Verifying a change" (`go test ./...`, `momo generate --check`, `momo migrate validate`); and the snippet section:

```markdown
## Copy into AGENTS.md

<!-- momo:agents-snippet:start -->
### Momo rules

- Never build SQL by string concatenation. Use the typed API from `momo.Select`, `momo.Insert`, `momo.Update` and `momo.Delete`.
- Use the generated column descriptors (for example `document.Title.Eq(x)`) in predicates; identifiers are quoted and values are bound by Momo.
- Do not pass user input to `momo.Raw` or `momo.SQL`; they are the explicit escape hatch.
- Never call `Update` or `Delete` without a `Where` clause; `AllRows` is the explicit opt-in.
- Run `momo generate --check` in CI after any model change, and do not edit generated files.
- Run `momo migrate validate` in CI; do not edit a migration that has been applied.
- Read `docs/agents.md` in the Momo module for the canonical patterns.
<!-- momo:agents-snippet:end -->
```

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/docscheck/ -run '^TestAgentGuideSnippet$' -count=1` prints `ok`. Then `go test ./internal/docscheck/ -count=1` runs every non-tagged test in the package (`TestDocsRequirementsPinned`, `TestMkdocsNavPagesExist`, `TestEveryDocPageInNav`, `TestDocsIndexLinksEveryPage`, `TestDocsCodeBlocks`, `TestAgentGuideSnippet`) and prints `ok`.

- [ ] **Step 5: Commit.** Commit Task 2's configuration with this page: `git add docs/agents.md internal/docscheck mkdocs.yml && git commit -m "docs: agent usage guide with an AGENTS.md snippet"`.

## Task 8: Docs workflow with mike versioning

Delivers: S-23 deployment (`/dev/` on pushes to main; `/vX.Y/` and `/latest/` on release tags), the docs build gate (strict build, `TestLLMsTxt`, compile and runnable blocks), and S-25 least-privilege permissions. The Python toolchain runs only in this workflow.

Files:
- Create: `.github/workflows/docs.yml`
- Create: `internal/repocheck/docs_workflow_test.go` (package `repocheck`)

Interfaces: none produced.

- [ ] **Step 1: Write the failing test.** Create `internal/repocheck/docs_workflow_test.go`:

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

func TestDocsWorkflow(t *testing.T) {
	w := loadWorkflow(t, "docs.yml")
	push, ok := w.On["push"].(map[string]any)
	if !ok {
		t.Fatal("docs.yml must run on push")
	}
	if !matrixHas(push["branches"], "main") {
		t.Error("docs.yml must deploy /dev/ on pushes to main")
	}
	if _, ok := push["tags"]; !ok {
		t.Error("docs.yml must deploy releases on tags")
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "docs.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{
		"mike deploy --push dev",
		"mike deploy --push --update-aliases",
		"mike set-default --push latest",
		"pip install --require-hashes -r docs/requirements.txt",
		"mkdocs build --strict",
		"TestLLMsTxt",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("docs.yml lacks %q", want)
		}
	}
	if w.Jobs["deploy"].Permissions == nil || !strings.Contains(text, "contents: write") {
		t.Error("only the deploy job may write contents")
	}
	ci, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?i)setup-python|pip install|python`).Match(ci) {
		t.Error("ci.yml must not install Python; docs tooling runs only in docs.yml")
	}
}
```

- [ ] **Step 2: Run and expect FAIL.** `go test ./internal/repocheck/ -run '^TestDocsWorkflow$' -count=1` fails with `open .../docs.yml: no such file or directory`.

- [ ] **Step 3: Write `.github/workflows/docs.yml`.** Top level: `name: docs`; `on: { push: { branches: [main], tags: ['v*'] }, pull_request: { paths: ['docs/**', 'mkdocs.yml', 'examples/**', 'tools/docs/**', 'internal/docscheck/**', '.github/workflows/docs.yml'] } }`; `permissions: { contents: read }`; `concurrency: { group: 'docs-${{ github.ref }}', cancel-in-progress: false }`. Jobs:
  - `build` (ubuntu-latest, timeout 25): a service `postgres` with image `pgvector/pgvector:0.8.7-pg17`, `env: { POSTGRES_PASSWORD: momo }`, `ports: ['5432:5432']`, `options: --health-cmd "pg_isready -U postgres" --health-interval 2s --health-retries 30`. Steps: checkout (`actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1`, `persist-credentials: false`); setup-python (`actions/setup-python@6d4fd22b20465ccc0f7354476588e8df7c9ca0e0 # v7.1.0`, `python-version: '3.12'`, `cache: pip`, `cache-dependency-path: docs/requirements.txt`); setup-go (`actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0`, `go-version: '1.27'`); `pip install --require-hashes -r docs/requirements.txt`; `mkdocs build --strict --site-dir site`; `MOMO_DOCS_SITE="$PWD/site" go test -tags docs -count=1 -run '^TestLLMsTxt$' ./internal/docscheck/`; `go test -count=1 -run '^TestDocsCodeBlocks$' ./internal/docscheck/`; `MOMO_REQUIRE_INTEGRATION=1 MOMO_TEST_POSTGRES_URL='postgres://postgres:momo@127.0.0.1:5432/postgres?sslmode=disable' go test -count=1 ./examples/ ./internal/docscheck/`; upload the site (`actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2`, `name: site`, `path: site`, `retention-days: 7`).
  - `deploy` (ubuntu-latest, timeout 15, `needs: build`, `if: github.event_name == 'push'`, `permissions: { contents: write }`): checkout with `fetch-depth: 0` (default credentials, which mike needs to push `gh-pages`); setup-python as above; `pip install --require-hashes -r docs/requirements.txt`; a step `if: github.ref == 'refs/heads/main'` running `mike deploy --push dev`; a step `if: startsWith(github.ref, 'refs/tags/v')` running:

```bash
version="${GITHUB_REF_NAME%.*}"
mike deploy --push --update-aliases "$version" latest
mike set-default --push latest
```

  The git identity for mike commits is set in a step before the deploy steps with `git config user.name "github-actions[bot]"` and `git config user.email "41898282+github-actions[bot]@users.noreply.github.com"`.

- [ ] **Step 4: Run and expect PASS.** `go test ./internal/repocheck/ -count=1` prints `ok` for every test, including `TestWorkflowsPinned` (plan 01) which now covers `docs.yml`. Run `actionlint .github/workflows/docs.yml` if installed; it must print nothing.

- [ ] **Step 5: Commit.** `git add .github/workflows/docs.yml internal/repocheck && git commit -m "ci(docs): build and deploy the site with mike for /dev/ and release versions"`.

## Task 9: Plan gate and GitHub Pages source

Delivers: the final acceptance state for S-21 and S-23, and the GitHub Pages source setting, applied and verified at execution time with `gh api`.

Files: none.

- [ ] **Step 1: Run the Go gate.** `make fmt-check vet test` and `go test ./internal/docscheck/ ./internal/repocheck/ -count=1`. Expected: exit 0 for each.

- [ ] **Step 2: Run the example and block gate against the PostgreSQL service.** `make db-up`, then `make test-integration-postgres` (the examples and the runnable blocks run through `testopen.Each`). Expected: `ok`. Then `make db-down`.

- [ ] **Step 3: Build the site as CI does.** With the venv from Task 6: `mkdocs build --strict --site-dir "$SCRATCH/site"` exits 0, then `MOMO_DOCS_SITE="$SCRATCH/site" go test -tags docs -count=1 -run '^TestLLMsTxt$' ./internal/docscheck/` prints `ok`.

- [ ] **Step 4: Set the Pages source after the first deploy.** The `gh-pages` branch is created by the first `mike deploy --push dev` on main (Task 8), so run this step after that deploy succeeds. Requires repository administration rights.

```bash
gh api repos/azrtydxb/momo/branches/gh-pages --jq .name
if gh api repos/azrtydxb/momo/pages >/dev/null 2>&1; then
  gh api -X PUT repos/azrtydxb/momo/pages -f 'source[branch]=gh-pages' -f 'source[path]=/'
else
  gh api -X POST repos/azrtydxb/momo/pages -f 'source[branch]=gh-pages' -f 'source[path]=/'
fi
```

  The first command prints `gh-pages`. The `if` block creates the Pages site when none exists and otherwise updates its source; both use the same two fields.

- [ ] **Step 5: Verify the Pages source and the published files.** `gh api repos/azrtydxb/momo/pages --jq '.source.branch, .html_url'` must print `gh-pages` and `https://azrtydxb.github.io/momo/`. Then `curl -fsI https://azrtydxb.github.io/momo/llms.txt` must report `HTTP/2 200` (retry for up to ten minutes while the first deploy propagates). If either check fails, the report records the failing command and its output; the plan does not treat the site as published.

## Spec traceability

| Spec id | Delivered by | Acceptance test(s) |
|---|---|---|
| S-21 | Tasks 2, 3, 4, 5 | TestExamples, TestDocsIndexLinksEveryPage, TestEveryDocPageInNav, TestCapabilityMatrixComplete, TestDocsCodeBlocks |
| S-23 | Tasks 1, 2, 5, 6, 7, 8, 9 | TestDocsCodeBlocks, TestDocsRunnableBlocks, TestLLMsTxt, TestAgentGuideSnippet, TestDocsWorkflow |
| S-25 (docs parts) | Tasks 1, 8 | TestDocsRequirementsPinned, TestWorkflowsPinned (plan 01), TestDocsWorkflow |

Owned by other plans, referenced here: the GitHub, Bun and Ent guides and `TestMigrationGuidesComplete` (momo-11 Task 14 and 15, which drops its own code-block extractor in favour of `TestDocsCodeBlocks`); `docs/migrations.md` (momo-04); `docs/vectors.md` (momo-07); `docs/mysql.md` (momo-08); `docs/PERFORMANCE.md` (momo-12).

## Open items for the report

- The examples and docs blocks depend on exact names from earlier plans (the generated example models of momo-02 Task 22, the projection form of momo-05 Task 5, the vector distance method of momo-07 Task 3, the migration dialect constructor of momo-04); each is checked by running the example.
- Runnable docs blocks cannot import `examples/internal`; they open their own connection from the environment.
