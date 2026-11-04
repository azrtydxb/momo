# momo-10-ecosystem-generators — implementation plan

Status: draft
Spec: .procoder/specs/momo.md

## Goal

Deliver the four first-party ecosystem generators as `gen.Plugin` implementations: GraphQL (S-63), gRPC/protobuf with a field-number lock file (S-64), OpenAPI 3.1 with optional `net/http` handlers (S-65), and `momo viz` Mermaid and DOT output (S-66).

## Architecture

Each plugin lives in its own package under `gen/` (`gen/graphql`, `gen/grpc`, `gen/openapi`, `gen/viz`) and receives the `*gen.Graph` and the `*gen.Writer` from plan momo-02, so no plugin writes files directly. Shared request-side helpers (allowlisted filters, keyset cursors, page limits) live in `gen/apiquery` and call the generated `ColumnByName` lookups from plan momo-02. Tests that need gqlgen, gRPC, protoc or the OpenAPI validator live in a separate module `internal/integration`, so the generator packages keep only the standard library and x/tools in their import graph.

## Plan dependencies

This plan assumes `momo-01-foundation` and `momo-02-generator` are complete. It consumes, by the names in `docs/ARCHITECTURE.md`: `gen.Load`, `gen.Graph`, `gen.Model`, `gen.Field`, `gen.Relation`, `gen.Plugin`, `gen.Writer`, `gen.Emit`, `gen.Config`, `gen.PluginAPIVersion`, `gen/config`, the generated `ColumnByName`, `ColumnNames`, `As` and relation descriptors from plan momo-02, `schema.Schema`, `schema.Table`, `schema.Load`, `momo.Load`, `momo.Relation[P,C]` (plan momo-06 owns its final shape), `momo.WithHook`, `momo.Hook`, `momo.QueryEvent`, `momo.Select`, `momo.ErrValidation`, `momo.ColumnRef`, and the example models in `internal/testmodels/example` (plan momo-02 Task 22).

Contract additions this plan needs (listed in the report; ARCHITECTURE is not edited here):
- `momo.ColumnRef` gains `ParseEq(raw string) (momo.Predicate, error)` and `ParseCmp(op, raw string) (momo.Predicate, error)` for `eq`, `ne`, `lt`, `le`, `gt`, `ge`. Each descriptor parses `raw` with the column's Go type and rejects a value that does not parse with `ErrValidation`. Used by Tasks 2, 8, 13 and 14.
- `gen.Writer.Text(path string, data []byte) error` for non-Go generated text (Mermaid, DOT, `.proto`, `.graphqls`, `.yml`, JSON). Same refusal and collision rules as `File`; the caller puts the header line in `data`.
- `gen.Writer.Scaffold(path string, data []byte) error` writes a file only when it does not exist. It is not a generated file, so it carries no header, and it never overwrites.

## Constraints

- S-63 (spec, verbatim): "First-party GraphQL generator: schema SDL, gqlgen-compatible resolvers, Relay connections with keyset pagination, typed filter and order inputs, and batched relation resolution."
- S-64 (spec, verbatim): "First-party gRPC generator: protobuf messages and CRUD/list services from models, with field-number stability recorded in a lock file so regeneration never renumbers fields."
- S-65 (spec, verbatim): "First-party OpenAPI generator: OpenAPI 3.1 document and optional net/http handlers for CRUD/list endpoints with filtering and keyset pagination."
- S-66 (spec, verbatim): "`momo viz`: ER diagrams (Mermaid and Graphviz DOT) from models or a migration snapshot."
- Milestone M4 (ARCHITECTURE delivery order). Every end-to-end test in this plan runs on PostgreSQL through `testopen.Postgres(t)` with the shared fixture `internal/integration/fixture`; golden and compile tests stay database-free. This plan uses no SQLite.
- Out of scope: GraphQL mutations. GraphQL serves queries only, because the spec lists no mutations.
- Generator and migration tooling dependencies (golang.org/x/tools etc.) stay out of the application runtime import graph; benchmark comparison ORMs live in a separate Go module.
- Dependency rule 3 (ARCHITECTURE): `gen`, `importer/*`, `migrate/*`, `cmd/*` may import x/tools, YAML and drivers. Application binaries that import only `momo` + one driver never link them.
- Dependency rule 5 (ARCHITECTURE) applies to model-level generated code only (metadata and descriptors). Framework output in user-chosen packages (GraphQL resolvers, gRPC servers, HTTP handlers) imports its framework; the plan states this as a clarification for the report.
- Security: identifiers always quoted, values always bound; raw SQL is an explicit escape hatch with a documented contract; parameter values not logged unless explicitly enabled.
- No hidden background goroutines, no implicit migrations at connect time, no unexplained global mutable state. Batched loading uses `momo.Load` once per relation per page, called synchronously from the list resolver.
- Go: minimum 1.26 in go.mod; CI tests 1.26 and 1.27. Dependencies pinned in go.mod/go.sum.
- CI: GitHub Actions on free GitHub-hosted runners only (public repo). The integration tests install protoc v36.2 from its GitHub release on the runner, with the archive SHA-256 recorded in `internal/integration/tools.lock`.
- Pinned versions (verified on the module proxy and GitHub releases on 2026-11-04; each at least two weeks old): `github.com/99designs/gqlgen v0.17.97` (2026-10-20), `google.golang.org/grpc v1.84.0` (2026-09-17), `google.golang.org/protobuf v1.36.12` (2026-08-10), `google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2` (2026-05-07), protoc `v36.2` (2026-09-17), `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3` (2026-06-28). Not chosen: buf v1.74.0 (no need for a second protobuf toolchain), kin-openapi v0.152.0 (2026-10-22, too recent and OpenAPI 3.0-first), libopenapi v0.42.3 (2026-11-01, too recent).
- OpenAPI 3.1 validation uses the official meta-schema `https://spec.openapis.org/oas/3.1/schema/2022-10-07` and the dialect `https://spec.openapis.org/oas/3.1/dialect/base`. Both are vendored under `gen/openapi/testdata/` with SHA-256 pins, so the test does not depend on the network.

## Task 1: Integration module with pinned tooling

Delivers: the test module that holds every test needing gqlgen, gRPC, protoc or the OpenAPI validator. Acceptance: `TestIntegrationModuleBuilds`.

Files:
- Create: `internal/integration/go.mod` (module `github.com/azrtydxb/momo/internal/integration`)
- Create: `internal/integration/go.sum` (generated by `go mod tidy`)
- Create: `internal/integration/tools.go` (build tag `tools`; blank imports of the gqlgen command and the protobuf plugins)
- Create: `internal/integration/tools.lock` (protoc archive names and SHA-256 values)
- Create: `internal/integration/integration_test.go`

Interfaces:
- The module requires `github.com/azrtydxb/momo` (replaced with `../..`), `github.com/99designs/gqlgen v0.17.97`, `google.golang.org/grpc v1.84.0`, `google.golang.org/protobuf v1.36.12`, `google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2`, and `github.com/santhosh-tekuri/jsonschema/v6 v6.0.3`.
- `tools.lock` format: one line per platform, `<archive name> <sha256>`, for `protoc-36.2-osx-aarch_64.zip`, `protoc-36.2-linux-x86_64.zip`, and `protoc-36.2-win64.zip`.

- [ ] Step 1: Write `internal/integration/integration_test.go`:

```go
package integration

import (
	"os/exec"
	"testing"
)

func TestIntegrationModuleBuilds(t *testing.T) {
	for _, pkg := range []string{"github.com/99designs/gqlgen", "google.golang.org/grpc", "google.golang.org/protobuf/proto", "github.com/santhosh-tekuri/jsonschema/v6"} {
		cmd := exec.Command("go", "list", pkg)
		cmd.Dir = "."
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", pkg, err, out)
		}
	}
}
```

- [ ] Step 2: Run `cd internal/integration && go test -count=1 ./...`; expect FAIL with `no required module provides package github.com/99designs/gqlgen`.

- [ ] Step 3: Create `go.mod` with the requires above and `replace github.com/azrtydxb/momo => ../..`, then run `go mod tidy`. Create `tools.go` with `//go:build tools` and blank imports of `github.com/99designs/gqlgen`, `google.golang.org/protobuf/cmd/protoc-gen-go` and `google.golang.org/grpc/cmd/protoc-gen-go-grpc`. Download the three protoc archives from the `v36.2` release with `gh release download v36.2 -R protocolbuffers/protobuf -p 'protoc-36.2-*'`, compute `shasum -a 256` and write `tools.lock`.

- [ ] Step 4: Run `cd internal/integration && go test -count=1 ./...`; expect `ok`.

- [ ] Step 5: Commit with `git add internal/integration`, message `test(integration): pinned tooling module for ecosystem generators`.

## Task 2: Shared API helpers: keyset cursors, page limits, allowlisted filters

Delivers: the request-side helpers that GraphQL, gRPC and OpenAPI share. Acceptance: `TestCursorRoundTrip`, `TestFilterAllowlist`, `TestPageLimit`.

Files:
- Create: `gen/apiquery/cursor.go`
- Create: `gen/apiquery/filter.go`
- Create: `gen/apiquery/apiquery_test.go`
- Modify: `momo/column_ref.go` (contract addition: `ParseEq`, `ParseCmp` on `ColumnRef` implementations; created by plan momo-02 Task 15 and extended here)

Interfaces:
- `type Cursor struct { Keys []string }` with `func EncodeCursor(keys ...string) string` (base64url of JSON `{"k":[...]}`, no padding) and `func DecodeCursor(s string) (Cursor, error)`. An invalid cursor returns an error wrapping `momo.ErrValidation`.
- `const DefaultLimit = 50; const MaxLimit = 200`; `func ClampLimit(n int) int` returns `DefaultLimit` for `n <= 0` and `MaxLimit` for `n > MaxLimit`.
- `type Filter struct { Column string; Op string; Value string }`; `func ParseFilter(lookup func(name string) (momo.ColumnRef, error), f Filter) (momo.Predicate, error)` calls `lookup` (the generated `ColumnByName`) and then `ParseCmp(f.Op, f.Value)`. An unknown column returns the error from `lookup`; an op outside `eq|ne|lt|le|gt|ge` returns `ErrValidation`.

- [ ] Step 1: Write `gen/apiquery/apiquery_test.go`:

```go
package apiquery

import (
	"errors"
	"testing"

	"github.com/azrtydxb/momo"
)

func TestCursorRoundTrip(t *testing.T) {
	c := EncodeCursor("42", "2026-01-02T03:04:05Z")
	got, err := DecodeCursor(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 2 || got.Keys[0] != "42" || got.Keys[1] != "2026-01-02T03:04:05Z" {
		t.Fatalf("keys=%v", got.Keys)
	}
	if _, err := DecodeCursor("!!!"); !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("invalid cursor: %v", err)
	}
}

func TestPageLimit(t *testing.T) {
	cases := map[int]int{0: DefaultLimit, -3: DefaultLimit, 10: 10, 500: MaxLimit}
	for in, want := range cases {
		if got := ClampLimit(in); got != want {
			t.Errorf("ClampLimit(%d)=%d want %d", in, got, want)
		}
	}
}

func TestFilterAllowlist(t *testing.T) {
	lookup := func(name string) (momo.ColumnRef, error) {
		return nil, errors.New("not allowed: " + name)
	}
	if _, err := ParseFilter(lookup, Filter{Column: "password", Op: "eq", Value: "x"}); err == nil {
		t.Fatal("unknown column accepted")
	}
	if _, err := ParseFilter(lookup, Filter{Column: "id", Op: "like", Value: "x"}); err == nil {
		t.Fatal("unknown op accepted")
	}
}
```

- [ ] Step 2: Run `go test ./gen/apiquery -count=1`; expect FAIL with `undefined: EncodeCursor`.

- [ ] Step 3: Implement `cursor.go` (`encoding/base64.RawURLEncoding` over `encoding/json`), `filter.go` (the op switch, calling `ParseCmp`). In `momo/column_ref.go` add `ParseEq` and `ParseCmp` to the `ColumnRef` interface and to each descriptor type: parse with `strconv` for integers and floats, `time.Parse(time.RFC3339Nano, ...)` for times, `strconv.ParseBool` for bools, and return `fmt.Errorf("%w: %q is not a valid %s", ErrValidation, raw, columnType)` on failure.

- [ ] Step 4: Run `go test ./gen/apiquery ./momo -count=1`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/apiquery momo/column_ref.go`, message `feat(gen): shared keyset cursor, page limit and allowlisted filter helpers`.

## Task 3: Viz edge model from models and from a snapshot

Delivers: the graph of relations that every viz renderer draws. Acceptance: `TestVizEdges`.

Files:
- Create: `gen/viz/edges.go`
- Create: `gen/viz/edges_test.go`

Interfaces:
- `type Edge struct { From, To string; FromColumns, ToColumns []string; Cardinality string; Label string }`. `Cardinality` is `one-to-one`, `one-to-many` or `many-to-many`.
- `func EdgesFromGraph(g *gen.Graph) []Edge`: one edge per relation of each model. Belongs-to and has-one give `one-to-one` or `one-to-many` according to the relation kind; has-many gives `one-to-many`; many-to-many gives `many-to-many`, with `To` the join table.
- `func EdgesFromSchema(s *schema.Schema) []Edge`: one edge per foreign key (`From` the referencing table, `To` the referenced table, `Cardinality` `one-to-many`).
- `func Tables(g *gen.Graph) []schema.Table` and `func TablesFromSchema(s *schema.Schema) []schema.Table`. Both return tables sorted by name.
- Output is sorted by `From`, `To`, `Label`.

- [ ] Step 1: Write `gen/viz/edges_test.go` using the example models:

```go
package viz

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

func TestVizEdges(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	edges := EdgesFromGraph(g)
	want := map[string]string{
		"comments->documents": "one-to-many",
		"documents->tags":     "many-to-many",
		"documents->tenants":  "one-to-many",
	}
	got := map[string]string{}
	for _, e := range edges {
		got[e.From+"->"+e.To] = e.Cardinality
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("edge %s: got %q want %q (all: %v)", k, got[k], v, got)
		}
	}
}
```

  The `want` keys use the table names of the example models. The `documents->tags` key assumes the join table is named `document_tags`; the implementer confirms the name from plan momo-02 Task 22 output and corrects the test key before Step 2.

- [ ] Step 2: Run `go test ./gen/viz -run TestVizEdges -count=1`; expect FAIL with `undefined: EdgesFromGraph`.

- [ ] Step 3: Implement `gen/viz/edges.go`. Build edges from `Model.Relations` (kind, target and the join table name from the relation) and from `Graph.Schema` foreign keys for `EdgesFromSchema`. Sort the output.

- [ ] Step 4: Run `go test ./gen/viz -run TestVizEdges -count=1`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/viz`, message `feat(viz): edge model from models and from a migration snapshot`.

## Task 4: Mermaid renderer (golden)

Delivers: Mermaid `erDiagram` output. Acceptance: `TestViz` (Mermaid part).

Files:
- Create: `gen/viz/mermaid.go`
- Create: `gen/viz/mermaid_test.go`
- Create: `gen/viz/testdata/example.mmd` (golden, reviewed)

Interfaces:
- `func Mermaid(tables []schema.Table, edges []Edge) []byte`. The first line is `%% Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT.`, then `erDiagram`.
- Each table renders as `<name> {` with one attribute line per column, `<sqltype> <column>` followed by `PK` for primary key columns, `FK` for foreign key columns, and `UK` for unique columns. The SQL type is sanitized to `[A-Za-z0-9_]` by replacing every other character with `_` (so `vector(1536)` becomes `vector_1536_`; trailing underscores are trimmed).
- Each edge renders as `<From> <marker> <To> : "<label>"`, where the marker is `||--||` for one-to-one, `||--o{` for one-to-many and `}o--o{` for many-to-many. Entity names are used verbatim (table names are identifiers). Edges and tables appear in the order given.

- [ ] Step 1: Write `gen/viz/mermaid_test.go`:

```go
package viz

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
)

func TestVizMermaidGolden(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	got := Mermaid(Tables(g), EdgesFromGraph(g))
	ok, err := golden.Diff("testdata/example.mmd", got, *update)
	if err != nil || !ok {
		t.Fatalf("golden mismatch (err=%v); run with -update after review", err)
	}
}
```

  Define `var update = flag.Bool("update", false, "rewrite golden files")` in this test file.

- [ ] Step 2: Run `go test ./gen/viz -run TestVizMermaidGolden -count=1`; expect FAIL with `undefined: Mermaid`.

- [ ] Step 3: Implement `gen/viz/mermaid.go` as specified, with a `strings.Builder` and no map iteration.

- [ ] Step 4: Run `go test ./gen/viz -run TestVizMermaidGolden -count=1 -update`, read `testdata/example.mmd`, then run `go test ./gen/viz -run TestVizMermaidGolden -count=1`; expect `ok`. The golden must include the `documents`, `comments`, `tags`, `tenants` and join-table entities and every relation edge.

- [ ] Step 5: Commit with `git add gen/viz`, message `feat(viz): mermaid erDiagram renderer`.

## Task 5: Graphviz DOT renderer and the TestViz acceptance test

Delivers: DOT output, and `TestViz`, the acceptance test for S-66 that checks both the Mermaid and the DOT goldens.

Files:
- Create: `gen/viz/dot.go`
- Create: `gen/viz/viz_test.go`
- Create: `gen/viz/testdata/example.dot` (golden, reviewed)

Interfaces:
- `func DOT(tables []schema.Table, edges []Edge) []byte`. The first line is `// Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT.`, then `digraph momo {`, `rankdir=LR;`, `node [shape=plaintext];`.
- Each table is a node named by the table name, with an HTML-like label `<TABLE BORDER="0" CELLBORDER="1" CELLSPACING="0">` holding a title row and one row per column (name, type, and `PK`/`FK`/`UK` markers). All text is escaped with `html.EscapeString`.
- Each edge is `<From> -> <To> [label="<label>", arrowhead=<shape>, dir=both]`, where `many-to-many` uses `crowodot`, `one-to-many` uses `crow` and `one-to-one` uses `tee`.
- `Mermaid` (Task 4) and `DOT` have the signature `func(tables []schema.Table, edges []Edge) []byte`, so `TestViz` can call both through one table.
- `update` is the flag variable defined in `gen/viz/mermaid_test.go` (Task 4).

Steps:

- [ ] Step 1: Write `gen/viz/viz_test.go`:

```go
package viz

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/schema"
)

func TestViz(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	tables := Tables(g)
	edges := EdgesFromGraph(g)
	cases := []struct {
		name   string
		golden string
		render func([]schema.Table, []Edge) []byte
	}{
		{name: "mermaid", golden: "testdata/example.mmd", render: Mermaid},
		{name: "dot", golden: "testdata/example.dot", render: DOT},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.render(tables, edges)
			if !bytes.Contains(got[:min(len(got), 80)], []byte("Code generated by momo")) {
				t.Fatalf("%s output lacks the generated header", c.name)
			}
			ok, err := golden.Diff(c.golden, got, *update)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				t.Fatalf("%s differs from %s; review and rerun with -update", c.name, c.golden)
			}
		})
	}
}
```

- [ ] Step 2: Run `go test ./gen/viz -run TestViz -count=1`; expect FAIL with `undefined: DOT`.

- [ ] Step 3: Implement `gen/viz/dot.go` as specified.

- [ ] Step 4: Run `go test ./gen/viz -run TestViz -count=1 -update`, read `testdata/example.dot`, then run `go test ./gen/viz -run TestViz -count=1`; expect `ok`. Both goldens must contain every table and every edge of the example.

- [ ] Step 5: Commit with `git add gen/viz && git commit -m "feat(viz): graphviz DOT renderer and TestViz over both goldens"`.

## Task 6: Viz plugin and the momo viz command

Delivers: `momo viz` from the models by default or from a migration snapshot with `--snapshot <file>`, and the viz plugin that writes through `Writer.Text`. Acceptance: `TestVizCLI` and `TestVizPlugin`. Exit codes follow the plan momo-02 Task 31 contract: 0 ok, 1 diagnostics, 2 usage, configuration or input error.

Files:
- Create: `gen/viz/plugin.go`
- Create: `gen/viz/plugin_test.go`
- Create: `cmd/momo/viz.go`
- Create: `cmd/momo/viz_test.go`
- Modify: `cmd/momo/cmd.go` (add the `viz` case and its help line)
- Modify: `gen/writer.go` (add `Text` and `Scaffold`, the contract additions of this plan)

Interfaces:
- `type Plugin struct { Format string; Out string }` with `Name() string` returning `viz`, `APIVersion() int` returning `gen.PluginAPIVersion`, and `Generate(g *gen.Graph, w *gen.Writer) error`. It writes `<Out>/schema.mmd` for `mermaid` and `<Out>/schema.dot` for `dot`, through `w.Text`. Another format is an error `unknown viz format "<f>"`.
- CLI: `momo viz [--format mermaid|dot] [--snapshot <file>] [-c momo.yaml] [--out <file>]`. The default source is the models, loaded with the packages from `momo.yaml`, or `./...` when there is no `momo.yaml` (the default of plan momo-02 Task 23). `--snapshot <file>` reads a migration snapshot with `schema.Load` and uses `EdgesFromSchema` and `TablesFromSchema`. Without `--out` the output goes to stdout.
- `Writer.Text(path, data)` follows the refusal rules of `Writer.File` and never gofmt-formats. `Writer.Scaffold(path, data)` writes only when the path does not exist.

Steps:

- [ ] Step 1: Write `cmd/momo/viz_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVizCLI(t *testing.T) {
	var out, errb bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing.json")
	if code := Run([]string{"viz", "--snapshot", missing}, &out, &errb); code != 2 {
		t.Fatalf("missing snapshot: exit %d, want 2: %s", code, errb.String())
	}

	dst := filepath.Join(t.TempDir(), "schema.mmd")
	chdir(t, "../../internal/testmodels/example")
	if code := Run([]string{"viz", "--format", "mermaid", "--out", dst}, &out, &errb); code != 0 {
		t.Fatalf("viz from models: exit %d: %s", code, errb.String())
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "%% Code generated by momo") {
		t.Fatalf("output lacks the generated header: %q", b)
	}
}
```

  `chdir` is the helper from plan momo-02 Task 23 (`cmd/momo/helpers_test.go`).

- [ ] Step 2: Write `gen/viz/plugin_test.go`:

```go
package viz

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

func TestVizPlugin(t *testing.T) {
	models, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: models})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}

	root := t.TempDir()
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: root})
	p := Plugin{Format: "dot", Out: filepath.Join(root, "viz")}
	if err := p.Generate(g, w); err != nil {
		t.Fatal(err)
	}
	outs := w.Outputs()
	want := filepath.Join(root, "viz", "schema.dot")
	if len(outs) != 1 || outs[0].Path != want {
		var got []string
		for _, o := range outs {
			got = append(got, o.Path)
		}
		t.Fatalf("outputs = %v, want only %s", got, want)
	}
	firstLine := strings.SplitN(string(outs[0].Data), "\n", 2)[0]
	if firstLine != "// Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT." {
		t.Fatalf("first line = %q", firstLine)
	}

	svg := Plugin{Format: "svg", Out: filepath.Join(root, "viz")}
	if err := svg.Generate(g, gen.NewWriter(gen.WriterOptions{ModuleRoot: root})); err == nil ||
		!strings.Contains(err.Error(), `unknown viz format "svg"`) {
		t.Fatalf("svg: got %v", err)
	}
}
```

  `Plugin.Generate` is called directly, not through `gen.Emit`, so the built-in emitters do not write into the repository tree from a temporary root.

- [ ] Step 3: Run `go test ./gen/viz -run TestVizPlugin -count=1`; expect FAIL with `undefined: Plugin`.

- [ ] Step 3: Run `go test ./cmd/momo ./gen/viz -run 'TestVizCLI|TestVizPlugin' -count=1`; expect FAIL with `unknown command "viz"` and `undefined: Plugin`.

- [ ] Step 4: Implement `Writer.Text` and `Writer.Scaffold` in `gen/writer.go`. Implement `gen/viz/plugin.go`, `cmd/momo/viz.go` (flags through `flag.NewFlagSet`; models through plan momo-02 `config.Load` and `gen.Load`; snapshots through `schema.Load`), and the `viz` case in `cmd/momo/cmd.go`. The help line for `viz` states the exit codes.

- [ ] Step 5: Run `go test ./cmd/momo ./gen/viz -count=1`; expect `ok`.

- [ ] Step 6: Commit with `git add gen cmd/momo && git commit -m "feat(viz): momo viz command and plugin" -m "Contract additions: Writer.Text, Writer.Scaffold."`.

## Task 7: GraphQL SDL generator (golden)

Delivers: the schema SDL with Relay connections and typed filter and order inputs. Acceptance: `TestGraphQLSDL` (the golden text part of `TestGraphQLGenerator`).

Files:
- Create: `gen/graphql/sdl.go`
- Create: `gen/graphql/sdl_test.go`
- Create: `gen/graphql/testdata/example.graphqls` (golden, reviewed)

Interfaces:
- `func SDL(g *gen.Graph) ([]byte, []gen.Diagnostic)`. Output starts with `# Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT.`.
- Vector fields (`vector.Vector`, `HalfVector`, `SparseVector`, `Bits`) are not exposed in GraphQL: they get no filter input and no field, and no diagnostic is raised.
- For each model `M` with GraphQL name `M` (the Go name): `type M { <field>: <type> ... }` with one field per allowlisted column (the names of `ColumnNames()`, camelCased: `tenant_id` becomes `tenantId`), and one field per relation: has-many and many-to-many become `[Target!]!`, belongs-to and has-one become `Target` (nullable). The `id` field is `ID!` when the primary key is a single integer.
- For each model: `type MConnection { edges: [MEdge!]!, pageInfo: PageInfo! }`, `type MEdge { node: M!, cursor: String! }`, `input MFilter { and: [MFilter!], or: [MFilter!], not: MFilter, <column>: <Scalar>Filter }`, `enum MOrderField { <COLUMN> }` (the allowlisted columns in upper snake case), `input MOrder { field: MOrderField!, direction: OrderDirection! }`.
- Shared: `type PageInfo { hasNextPage: Boolean!, endCursor: String }`, `enum OrderDirection { ASC DESC }`, `input StringFilter { eq: String, ne: String, in: [String!], contains: String }`, `input IntFilter { eq: Int, ne: Int, in: [Int!], lt: Int, lte: Int, gt: Int, gte: Int }`, and the matching `Bool`, `Time` and `Float` filters emitted only when a model uses them.
- Query root: `type Query { <plural camelCase>(first: Int, after: String, filter: MFilter, order: MOrder): MConnection!, <singular camelCase>(id: ID!): M }` for each model.

- [ ] Step 1: Write `gen/graphql/sdl_test.go`:

```go
package graphql

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestGraphQLSDL(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	src, ds := SDL(g)
	if len(ds) != 0 {
		t.Fatalf("diags %v", ds)
	}
	ok, err := golden.Diff("testdata/example.graphqls", src, *update)
	if err != nil || !ok {
		t.Fatalf("golden mismatch (err=%v); run with -update after review", err)
	}
}
```

- [ ] Step 2: Run `go test ./gen/graphql -run TestGraphQLSDL -count=1`; expect FAIL with `undefined: SDL`.

- [ ] Step 3: Implement `gen/graphql/sdl.go` as specified, using `strings.Builder`, sorted models and the `ColumnNames` order. A column type with no filter input maps to a diagnostic `no GraphQL filter for <type>`.

- [ ] Step 4: Run the test with `-update`, review `testdata/example.graphqls` (it must parse: the integration module validates it with `parser.ParseSchema` from `github.com/vektah/gqlparser/v2` in Task 9), then run without `-update`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/graphql`, message `feat(graphql): SDL with relay connections and typed filter and order inputs`.

## Task 8: GraphQL Go glue and gqlgen configuration (scaffold)

Delivers: the Go side of the GraphQL server: generated types and list functions, the `gqlgen.yml`, and a scaffolded resolver that is never overwritten. Acceptance: `TestGraphQLScaffold`.

Files:
- Create: `gen/graphql/goemit.go`
- Create: `gen/graphql/goemit_test.go`
- Create: `gen/graphql/plugin.go` (the `gen.Plugin` for GraphQL: `Name` `graphql`, `APIVersion` returns `gen.PluginAPIVersion`, `Generate` writes all files)

Interfaces:
- `type Plugin struct { Dir string; Package string; ModelsPackage string }`. `Generate` writes `<Dir>/schema.graphqls` (SDL, Task 7) via `w.Text`; `<Dir>/momo_gen.go` (package `Package`) via `w.File`, containing the input and connection Go types for each model, the list and get functions `func List<Model>(ctx context.Context, q momo.Querier, args ListArgs) (*Connection, error)` and `func Get<Model>(ctx context.Context, q momo.Querier, id int64) (*models.M, error)`; `<Dir>/gqlgen.yml` via `w.Text`, mapping each GraphQL type to its Go type and setting `resolver: {layout: single-file, filename: <Dir>/resolver.go, package: <Package>}`; `<Dir>/resolver.go` via `w.Scaffold`, containing a `Resolver` struct that delegates to the generated functions.
- Generated list functions use the typed API only: `momo.Select` over the model type, with `.Where(...)` with the filter predicates from `ParseCmp` over `ColumnByName`, `apiquery.DecodeCursor` for `after`, `ClampLimit` for `first`, and `OrderBy` on the allowlisted column plus `id` as tie-breaker.

- [ ] Step 1: Write `gen/graphql/goemit_test.go`:

```go
package graphql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azrtydxb/momo/gen"
)

func TestGraphQLScaffold(t *testing.T) {
	root := t.TempDir()
	src, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, root); err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	p := Plugin{Dir: filepath.Join(root, "graph"), Package: "graph", ModelsPackage: "github.com/azrtydxb/momo/internal/testmodels/example"}
	if err := emitWith(t, root, g, p); err != nil {
		t.Fatal(err)
	}
	resolver := filepath.Join(root, "graph", "resolver.go")
	if err := os.WriteFile(resolver, []byte("package graph\n// hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := emitWith(t, root, g, p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "// hand edit") {
		t.Fatal("scaffold overwrote a handwritten resolver")
	}
}
```

  `emitWith` runs `gen.Emit` with a `gen.Writer` rooted at `root` and a plugin list containing `p`, then `Commit`; `copyDir` copies a directory tree. Define both in this test file.

- [ ] Step 2: Run `go test ./gen/graphql -run TestGraphQLScaffold -count=1`; expect FAIL with `undefined: Plugin`.

- [ ] Step 3: Implement `gen/graphql/goemit.go` and `gen/graphql/plugin.go` as specified. The `Plugin` passes its outputs through the `gen.Writer` only.

- [ ] Step 4: Run `go test ./gen/graphql -count=1`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/graphql`, message `feat(graphql): go glue, gqlgen configuration and scaffolded resolver`.

## Task 9: GraphQL server end to end with batched relations

Delivers: a gqlgen server built from the generated output, with batched relation loading, a statement-count assertion through a counting `momo.Hook`, and keyset pagination with `after`. Acceptance: `TestGraphQLGenerator`.

Scope: GraphQL serves queries only. The spec lists no mutations, so GraphQL mutations are out of scope for this plan.

Files:
- Create: `internal/integration/graphql/graphql_test.go` (package `graphql_test`)
- Create: `internal/integration/fixture/fixture.go` (PostgreSQL DDL and seed rows, shared by the GraphQL and OpenAPI end-to-end tests)
- Create: `internal/integration/graphql/example/graph/` (committed generated output: `momo_gen.go`, `schema.graphqls`, `resolver.go` scaffold, `generated/exec.go` from gqlgen)
- Create: `internal/integration/graphql/example/gqlgen.yml` (committed, generated)
- Modify: `gen/graphql/goemit.go` (list functions use `SelectQuery.Preload`, one statement per relation per page)

Interfaces:
- The list function runs the page query with `.Preload(document.Comments.Scope(), document.Tags.Scope())` (plan momo-06 `SelectQuery.Preload`, with `Relation.Scope()` loaders), so each relation costs one statement per page regardless of the number of documents, where `document` is the generated descriptor package of plan momo-02 Task 13. Relation loading is never `SelectQuery.With`, which is the CTE method.
- The scaffolded `Resolver` struct has the field `DB *momo.DB` and implements the gqlgen `ResolverRoot` methods by calling the generated functions.
- Shared fixture `internal/integration/fixture` (PostgreSQL with pgvector): `Apply(t testing.TB, db *momo.DB)` creates the example schema and seeds five documents, each with two comments and one tag. The vector column is included, because the model scans every column.

Steps:

- [ ] Step 1: Write `internal/integration/graphql/graphql_test.go`:

```go
package graphql_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/gen"
	ggraphql "github.com/azrtydxb/momo/gen/graphql"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph"
	"github.com/azrtydxb/momo/internal/integration/graphql/example/graph/generated"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// counter counts the statements momo sends to the database.
type counter struct{ n int }

func (c *counter) BeforeQuery(ctx context.Context, _ *momo.QueryEvent) context.Context {
	c.n++
	return ctx
}

func (c *counter) AfterQuery(context.Context, *momo.QueryEvent) {}

func openDB(t *testing.T, c *counter) *momo.DB {
	t.Helper()
	db := testopen.Postgres(t, momo.WithHook(c))
	fixture.Apply(t, db)
	c.n = 0 // count only the statements of the requests
	return db
}

func post(t *testing.T, url, q string) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]string{"query": q})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	var out struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) != 0 {
		t.Fatalf("graphql errors: %v", out.Errors)
	}
	return out.Data
}

func connection(t *testing.T, data map[string]any) (ids []string, endCursor string) {
	t.Helper()
	conn, ok := data["documents"].(map[string]any)
	if !ok {
		t.Fatalf("no documents connection in %v", data)
	}
	edges, ok := conn["edges"].([]any)
	if !ok {
		t.Fatalf("no edges in %v", conn)
	}
	for _, e := range edges {
		node := e.(map[string]any)["node"].(map[string]any)
		ids = append(ids, node["id"].(string))
	}
	page := conn["pageInfo"].(map[string]any)
	endCursor, ok = page["endCursor"].(string)
	if !ok {
		t.Fatalf("no endCursor in %v", page)
	}
	return ids, endCursor
}

const pageQuery = `query {
  documents(first: %d%s, filter: {title: {contains: "a"}}, order: {field: ID, direction: ASC}) {
    edges { node { id title comments { id body } tags { id name } } cursor }
    pageInfo { hasNextPage endCursor }
  }
}`

func TestGraphQLGenerator(t *testing.T) {
	// 1. the committed output equals a fresh generation from the example models
	models, err := filepath.Abs("../../../testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	outDir, err := filepath.Abs("example/graph")
	if err != nil {
		t.Fatal(err)
	}
	modRoot, err := filepath.Abs("example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: models})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	g.Plugins = []gen.Plugin{ggraphql.Plugin{Dir: outDir, Package: "graph", ModelsPackage: "github.com/azrtydxb/momo/internal/testmodels/example"}}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: modRoot})
	if err := gen.Emit(g, w); err != nil {
		t.Fatal(err)
	}
	for _, o := range w.Outputs() {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; regenerate the example with momo generate and gqlgen generate", o.Path)
		}
	}

	// 2. serve the gqlgen server over HTTP, backed by the database
	c := &counter{}
	db := openDB(t, c)
	srv := httptest.NewServer(handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{
		Resolvers: &graph.Resolver{DB: db},
	})))
	t.Cleanup(srv.Close)

	// 3. batched relations: the statement count does not grow with the page size
	c.n = 0
	first2 := post(t, srv.URL, fmt.Sprintf(pageQuery, 2, ""))
	n2 := c.n
	c.n = 0
	first5 := post(t, srv.URL, fmt.Sprintf(pageQuery, 5, ""))
	n5 := c.n
	if n2 != n5 || n2 > 3 {
		t.Fatalf("statements for page of 2 = %d, of 5 = %d; want equal and at most 3 (list plus one batch per relation)", n2, n5)
	}

	// 4. keyset pagination: the page after endCursor has no overlap with the first page
	ids1, cursor := connection(t, first2)
	if len(ids1) != 2 || cursor == "" {
		t.Fatalf("first page = %v cursor=%q", ids1, cursor)
	}
	ids2, lastCursor := connection(t, post(t, srv.URL, fmt.Sprintf(pageQuery, 2, fmt.Sprintf(`, after: %q`, cursor))))
	if lastCursor == "" {
		t.Fatal("second page has no endCursor")
	}
	if len(ids2) != 2 {
		t.Fatalf("second page = %v", ids2)
	}
	seen := map[string]bool{}
	for _, id := range ids1 {
		seen[id] = true
	}
	for _, id := range ids2 {
		if seen[id] {
			t.Fatalf("id %s appears on both pages", id)
		}
	}
	if ids5, last5 := connection(t, first5); len(ids5) != 5 || last5 == "" {
		t.Fatalf("page of 5 = %v cursor=%q", ids5, last5)
	}
}
```

  The page of 5 is checked for five ids and a cursor, so its response is used, not discarded. Every error returned by `filepath.Abs`, `gen.Load`, `gen.Emit`, `json` and `http` calls is checked.

- [ ] Step 1b: Write `internal/integration/fixture/fixture.go`:

```go
// Package fixture holds the PostgreSQL schema and seed rows of the example models for end-to-end tests.
package fixture

import (
	"context"
	"fmt"
	"testing"

	"github.com/azrtydxb/momo"
)

var ddl = []string{
	`CREATE EXTENSION IF NOT EXISTS vector`,
	`CREATE TABLE tenants (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, name text NOT NULL)`,
	`CREATE TABLE documents (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, tenant_id bigint NOT NULL REFERENCES tenants(id), title text NOT NULL, body text, embedding vector(1536), status text NOT NULL DEFAULT 'draft', created_at timestamptz NOT NULL DEFAULT now())`,
	`CREATE TABLE comments (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, document_id bigint NOT NULL REFERENCES documents(id), body text NOT NULL)`,
	`CREATE TABLE tags (id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY, name text NOT NULL UNIQUE)`,
	`CREATE TABLE document_tags (document_id bigint NOT NULL REFERENCES documents(id), tag_id bigint NOT NULL REFERENCES tags(id), PRIMARY KEY (document_id, tag_id))`,
}

// Apply creates the schema on a fresh database and seeds five documents, each with two comments and one tag.
func Apply(t testing.TB, db *momo.DB) {
	t.Helper()
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := momo.Exec(ctx, db, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range ddl {
		exec(s)
	}
	exec(`INSERT INTO tenants (id, name) VALUES (1, 'acme')`)
	for i, title := range []string{"alpha", "beta", "gamma", "delta", "data"} {
		id := i + 1
		exec(`INSERT INTO documents (id, tenant_id, title) VALUES (?, 1, ?)`, id, title)
		exec(`INSERT INTO comments (document_id, body) VALUES (?, ?)`, id, fmt.Sprintf("c%d-1", id))
		exec(`INSERT INTO comments (document_id, body) VALUES (?, ?)`, id, fmt.Sprintf("c%d-2", id))
		exec(`INSERT INTO tags (id, name) VALUES (?, ?)`, id, fmt.Sprintf("t%d", id))
		exec(`INSERT INTO document_tags (document_id, tag_id) VALUES (?, ?)`, id, id)
	}
}
```

  Run `cd internal/integration && go vet ./fixture`; expect exit 0.

- [ ] Step 2: Run `cd internal/integration && go test -count=1 -run TestGraphQLGenerator ./graphql`; expect FAIL with `undefined: ggraphql.Plugin` or `no required module provides package .../example/graph`.

- [ ] Step 3: Implement the plugin, the scaffold, and the preload in `gen/graphql/goemit.go`. Generate with `go run ./cmd/momo generate` pointed at the example models using the plugin configuration (`plugins: graphql: {dir: internal/integration/graphql/example/graph, package: graph, models_package: github.com/azrtydxb/momo/internal/testmodels/example}`), then run `go run github.com/99designs/gqlgen generate` in `internal/integration/graphql/example` to produce `graph/generated/exec.go`. Commit the generated files. Do not hand-edit generated files.

- [ ] Step 4: Run `cd internal/integration && go test -count=1 -run TestGraphQLGenerator ./graphql`; expect `ok`.

- [ ] Step 5: Commit with `git add internal/integration/graphql gen/graphql && git commit -m "feat(graphql): batched relation loading verified against a gqlgen server"`.

## Task 10: gRPC protobuf emitter (golden)

Delivers: `.proto` messages and CRUD/list services from the models. Acceptance: `TestProtoEmitGolden` (the emitter golden part).

Files:
- Create: `gen/grpc/proto.go`
- Create: `gen/grpc/proto_test.go`
- Create: `gen/grpc/testdata/example.proto` (golden, reviewed)

Interfaces:
- `func Proto(g *gen.Graph, lock *Lock, pkg string) ([]byte, []gen.Diagnostic)`, where `pkg` is the proto package name (for example `momo.example.v1`). The output starts with `// Code generated by momo v0.1.0-dev (format 1). DO NOT EDIT.` and `syntax = "proto3";`.
- Type mapping: `int64` to `int64`, `int32` to `int32`, `string` to `string`, `bool` to `bool`, `float64` to `double`, `float32` to `float`, `[]byte` to `bytes`, `time.Time` to `google.protobuf.Timestamp`, `vector.Vector` to `repeated float`, pointer fields to `optional` fields. Any other type is a diagnostic `no protobuf mapping for <type>`.
- Per model `M`: `message M { ... }` with one field per persisted column, numbered by the lock; `message MListRequest { string after = 1; int32 limit = 2; repeated Filter filter = 3; }`; `message MListResponse { repeated M items = 1; string next_cursor = 2; }`; `message Filter { string column = 1; string op = 2; string value = 3; }` shared once per file; `service MService { rpc Get(MGetRequest) returns (M); rpc List(MListRequest) returns (MListResponse); rpc Create(M) returns (M); rpc Update(M) returns (M); rpc Delete(MDeleteRequest) returns (MDeleteResponse); }`.

- [ ] Step 1: Write `gen/grpc/proto_test.go`:

```go
package grpc

import (
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
)

func TestProtoEmitGolden(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil || len(diags) != 0 {
		t.Fatalf("load: %v %v", err, diags)
	}
	lock := &Lock{}
	src, ds := Proto(g, lock, "momo.example.v1")
	if len(ds) != 0 {
		t.Fatalf("diags %v", ds)
	}
	ok, err := golden.Diff("testdata/example.proto", src, *update)
	if err != nil || !ok {
		t.Fatalf("golden mismatch (err=%v); run with -update after review", err)
	}
}
```

  Define `var update = flag.Bool("update", false, "rewrite golden files")` in this file.

- [ ] Step 2: Run `go test ./gen/grpc -run TestProtoEmitGolden -count=1`; expect FAIL with `undefined: Proto`.

- [ ] Step 3: Implement `gen/grpc/proto.go` as specified, with `Lock` from Task 11 (an empty lock assigns numbers in declaration order from 1).

- [ ] Step 4: Run the test with `-update`, review `testdata/example.proto`, then run without `-update`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/grpc`, message `feat(grpc): protobuf messages and CRUD services from models`.

## Task 11: Field-number lock file

Delivers: field numbers that never change once recorded. Acceptance: `TestProtoLockStableNumbers`.

Files:
- Create: `gen/grpc/lock.go`
- Create: `gen/grpc/lock_test.go`

Interfaces:
- `type Lock struct { Version int \`json:"version"\`; Messages map[string]*MessageLock \`json:"messages"\` }`, `type MessageLock struct { Fields map[string]int \`json:"fields"\`; Reserved []int \`json:"reserved,omitempty"\` }`. Field identity is the database column name, so renaming a Go field that keeps its `column:` tag keeps its number.
- `func LoadLock(path string) (*Lock, error)` (a missing file returns an empty lock with `Version: 1`) and `func (l *Lock) Save(path string) error` writing indented JSON with sorted keys (encoding/json sorts map keys).
- `func (l *Lock) Assign(message string, columns []string) (map[string]int, []gen.Diagnostic)`: an existing column keeps its number; a new column gets the smallest number above the maximum ever used for that message (including reserved numbers); a column recorded earlier but absent now moves its number to `Reserved` and is never reused.
- Output of `Proto` (Task 10) uses `Assign`, and `momo generate` for the grpc plugin writes `momo.proto.lock` next to the `.proto` file through `w.Text`.

- [ ] Step 1: Write `gen/grpc/lock_test.go`:

```go
package grpc

import (
	"path/filepath"
	"testing"
)

func TestProtoLockStableNumbers(t *testing.T) {
	l := &Lock{Version: 1, Messages: map[string]*MessageLock{}}
	first, diags := l.Assign("Document", []string{"id", "title", "body"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if first["id"] != 1 || first["title"] != 2 || first["body"] != 3 {
		t.Fatalf("first=%v", first)
	}
	// rename of the Go field keeps the column, so the number is kept
	again, diags := l.Assign("Document", []string{"id", "title", "body"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if again["title"] != 2 {
		t.Fatalf("renumbered: %v", again)
	}
	// removing a column reserves its number and it is never reused
	dropped, diags := l.Assign("Document", []string{"id", "title"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if _, ok := dropped["body"]; ok {
		t.Fatal("removed column still assigned")
	}
	added, diags := l.Assign("Document", []string{"id", "title", "summary"})
	if len(diags) != 0 {
		t.Fatalf("assign: %v", diags)
	}
	if added["summary"] != 4 {
		t.Fatalf("new column reused a reserved number: %v", added)
	}
	path := filepath.Join(t.TempDir(), "momo.proto.lock")
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Messages["Document"].Fields["title"] != 2 {
		t.Fatalf("lock round trip lost numbers: %+v", loaded.Messages["Document"])
	}
}
```

- [ ] Step 2: Run `go test ./gen/grpc -run TestProtoLockStableNumbers -count=1`; expect FAIL with `undefined: Lock`.

- [ ] Step 3: Implement `gen/grpc/lock.go` as specified. Keep `Reserved` sorted.

- [ ] Step 4: Run `go test ./gen/grpc -run TestProtoLockStableNumbers -count=1`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/grpc`, message `feat(grpc): field-number lock file so regeneration never renumbers fields`.

## Task 12: protoc compile, generated Go stubs and an in-process round trip

Delivers: a gRPC server backed by momo, verified over bufconn with protoc-generated stubs. Acceptance: `TestProtoGenerator` (protoc and round-trip parts).

Files:
- Create: `gen/grpc/server.go` (the `Server` emitter: `func ServerSource(g *gen.Graph, pkg string) ([]byte, []gen.Diagnostic)`)
- Create: `gen/grpc/plugin.go` (the `gen.Plugin` for gRPC: writes `<Dir>/<name>.proto` and `<Dir>/momo.proto.lock` through `w.Text`, and `<Dir>/server_gen.go` through `w.File`)
- Create: `internal/integration/grpc/grpc_test.go`
- Create: `internal/integration/grpc/testdata/rename/` (a copy of the example package with `Title` renamed to `Name` and its tag kept as `column:title`)

Interfaces:
- `Server` implements the generated `MServiceServer` interface from protoc-gen-go-grpc, calling the typed momo API. Its `List` builds the filter with `apiquery.ParseFilter` over `ColumnByName`, decodes `after` with `apiquery.DecodeCursor`, clamps `limit` with `apiquery.ClampLimit`, and returns `next_cursor` from the last row's key.
- The test pipeline: (a) run the plugin and write the `.proto` and lock, (b) run `protoc` v36.2 with `--go_out` (protoc-gen-go v1.36.12) and `--go-grpc_out` (protoc-gen-go-grpc v1.6.2) into the test module, (c) compile the output, (d) run a CRUD round trip (Create, Get, List with filter and cursor, Update, Delete) over a `google.golang.org/grpc/test/bufconn` listener with an in-process server backed by PostgreSQL (`testopen.Postgres(t)`, seeded with `fixture.Apply`), (e) regenerate the rename copy and require that the `title` field keeps its number in `momo.proto.lock`.

- [ ] Step 1: Write `internal/integration/grpc/grpc_test.go`. The test needs `PROTOC` (protoc v36.2, see `tools.lock`); a missing value fails the test, it does not skip.

```go
package grpc_test

import (
	"bytes"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/azrtydxb/momo/gen"
	ggrpc "github.com/azrtydxb/momo/gen/grpc"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/grpc/example"
	"github.com/azrtydxb/momo/internal/integration/grpc/example/pb"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

func TestProtoGenerator(t *testing.T) {
	protoc := os.Getenv("PROTOC")
	if protoc == "" {
		t.Fatal("PROTOC is required: install protoc v36.2 and set PROTOC to its path (see internal/integration/tools.lock)")
	}
	pkgDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	modRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(pkgDir, "example")
	modelsDir, err := filepath.Abs("../../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}

	// (a) the plugin output equals the committed .proto, lock and server
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: modelsDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	g.Plugins = []gen.Plugin{ggrpc.Plugin{
		Dir:          outDir,
		Package:      "example",
		ProtoPackage: "momo.example.v1",
		GoPackage:    "github.com/azrtydxb/momo/internal/integration/grpc/example/pb",
	}}
	w := gen.NewWriter(gen.WriterOptions{ModuleRoot: pkgDir})
	if err := gen.Emit(g, w); err != nil {
		t.Fatal(err)
	}
	for _, o := range w.Outputs() {
		ok, err := golden.Diff(o.Path, o.Data, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("%s is stale; regenerate with momo generate", o.Path)
		}
	}

	// (b) protoc output equals the committed stubs
	tmp := t.TempDir()
	for _, args := range [][]string{{
		"--proto_path=" + outDir,
		"--go_out=" + tmp, "--go_opt=paths=source_relative",
		"--go-grpc_out=" + tmp, "--go-grpc_opt=paths=source_relative",
		"momo.proto",
	}} {
		if out, err := exec.Command(protoc, args...).CombinedOutput(); err != nil {
			t.Fatalf("protoc: %v\n%s", err, out)
		}
	}
	for _, name := range []string{"momo.pb.go", "momo_grpc.pb.go"} {
		got, err := os.ReadFile(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		ok, err := golden.Diff(filepath.Join(outDir, "pb", name), got, false)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatalf("pb/%s is stale; rerun protoc with the committed options", name)
		}
	}

	// (c) the stubs and the server compile
	build := exec.Command("go", "build", "./grpc/...")
	build.Dir = modRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	// (d) CRUD round trip over bufconn, backed by PostgreSQL
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterDocumentServiceServer(srv, example.NewDocumentServer(db))
	served := make(chan error, 1)
	go func() { served <- srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		if err := <-served; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	client := pb.NewDocumentServiceClient(conn)
	ctx := context.Background()

	created, err := client.Create(ctx, &pb.Document{TenantId: 1, Title: "round trip", Status: "draft"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Id == 0 {
		t.Fatal("create returned no id")
	}
	got, err := client.Get(ctx, &pb.DocumentGetRequest{Id: created.Id})
	if err != nil || got.Title != "round trip" {
		t.Fatalf("get = %v, %v", got, err)
	}
	created.Title = "round trip v2"
	if _, err := client.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	list, err := client.List(ctx, &pb.DocumentListRequest{Limit: 10, Filter: []*pb.Filter{{Column: "title", Op: "eq", Value: "round trip v2"}}})
	if err != nil || len(list.Items) != 1 || list.Items[0].Id != created.Id {
		t.Fatalf("filtered list = %v, %v", list, err)
	}
	first, err := client.List(ctx, &pb.DocumentListRequest{Limit: 2})
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %v, %v", first, err)
	}
	second, err := client.List(ctx, &pb.DocumentListRequest{Limit: 2, After: first.NextCursor})
	if err != nil || len(second.Items) == 0 || second.Items[0].Id == first.Items[1].Id {
		t.Fatalf("keyset page = %v, %v", second, err)
	}
	if _, err := client.Delete(ctx, &pb.DocumentDeleteRequest{Id: created.Id}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(ctx, &pb.DocumentGetRequest{Id: created.Id}); status.Code(err) != codes.NotFound {
		t.Fatalf("get after delete: want NotFound, got %v", err)
	}

	// (e) renaming a Go field keeps the field number recorded for its column
	renameDir, err := filepath.Abs("testdata/rename")
	if err != nil {
		t.Fatal(err)
	}
	rg, rdiags, err := gen.Load([]string{"./"}, gen.Config{Dir: renameDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(rdiags) != 0 {
		t.Fatalf("rename diagnostics: %v", rdiags)
	}
	lock, err := ggrpc.LoadLock(filepath.Join(outDir, "momo.proto.lock"))
	if err != nil {
		t.Fatal(err)
	}
	before := lock.Messages["Document"].Fields["title"]
	src, ds := ggrpc.Proto(rg, lock, "momo.example.v1")
	if len(ds) != 0 {
		t.Fatalf("diagnostics: %v", ds)
	}
	if lock.Messages["Document"].Fields["title"] != before || before != 3 {
		t.Fatalf("title number changed: before %d, after %d", before, lock.Messages["Document"].Fields["title"])
	}
	if !bytes.Contains(src, []byte("string name = 3;")) {
		t.Fatalf("renamed field does not keep number 3:\n%s", src)
	}
	if _, invented := lock.Messages["Document"].Fields["name"]; invented {
		t.Fatal("rename created a new field identity")
	}
}
```

  The rename fixture `internal/integration/grpc/testdata/rename/rename.go` is package `rename` with the `Document` model of the example, where the `Title` field is renamed to `Name` and keeps `momo:"column:title,size:300,notnull"`. `ggrpc.Plugin` takes `Dir`, `Package`, `ProtoPackage` and `GoPackage`; it writes `momo.proto`, `momo.proto.lock` and `server_gen.go` (package `example`, with `NewDocumentServer(db *momo.DB) pb.DocumentServiceServer` per model) through the writer.

- [ ] Step 2: Run `cd internal/integration && PROTOC=$(pwd)/bin/protoc go test -count=1 -run TestProtoGenerator ./grpc`; expect FAIL with `undefined: ServerSource` or `stale`.

- [ ] Step 3: Implement `gen/grpc/server.go`, `gen/grpc/plugin.go` and the rename fixture until every step passes. Generate the committed `example/` output with the plugin and `protoc`; do not hand-edit it.

- [ ] Step 4: Run `cd internal/integration && PROTOC=$(pwd)/bin/protoc MOMO_REQUIRE_INTEGRATION=1 go test -count=1 -run TestProtoGenerator ./grpc`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/grpc internal/integration/grpc && git commit -m "feat(grpc): server adapter verified with protoc-generated stubs and a PostgreSQL round trip"`.

- [ ] Step 2: Run `cd internal/integration && go test -count=1 -run TestProtoGenerator ./grpc`; expect FAIL with `undefined: ServerSource` or `PROTOC is required`.

- [ ] Step 3: Implement `gen/grpc/server.go` and `gen/grpc/plugin.go`. Implement until the protoc compile, the round trip and the rename check all pass. The generated code is never hand edited.

- [ ] Step 4: Run `cd internal/integration && PROTOC=$(pwd)/bin/protoc go test -count=1 -run TestProtoGenerator ./grpc`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/grpc internal/integration/grpc`, message `feat(grpc): server adapter verified with protoc-generated stubs and a bufconn round trip`.

## Task 13: OpenAPI 3.1 document (golden)

Delivers: the OpenAPI 3.1 document of the example models, golden-checked here and validated against the official meta-schema in Task 14. Acceptance: `TestOpenAPIDocumentGolden` (the document part of the spec's `TestOpenAPIGenerator`, whose full form is Task 14).

Files:
- Create: `gen/openapi/doc.go`
- Create: `gen/openapi/doc_test.go`
- Create: `gen/openapi/testdata/example.openapi.json` (golden, reviewed)

Interfaces:
- `func Document(g *gen.Graph, opts Options) ([]byte, []gen.Diagnostic)` with `type Options struct { Title, Version, BasePath string; Handlers bool }`. The output is JSON with `"openapi": "3.1.0"`, `info`, `paths` and `components.schemas`.
- Schemas: `int64` and `int32` map to `{"type":"integer","format":"int64"}` or `int32`; `string` to `{"type":"string"}`; `bool` to `boolean`; `float64` to `number` with `format: double`; `time.Time` to `{"type":"string","format":"date-time"}`; `[]byte` to `{"type":"string","contentEncoding":"base64"}`; `vector.Vector` to an array of numbers. Nullable (pointer) fields use the 3.1 form `"type": ["<t>", "null"]`; the `nullable` keyword is never emitted. Schema properties are the column fields only; relations are not properties.
- Paths: `BasePath/<plural>` with `get` (list: query parameters `limit`, `after`, and one `filter[<column>]` parameter per allowlisted column, operator `eq` only) and `post` (create); `BasePath/<plural>/{id}` with `get`, `put` and `delete`. The list response is `{"items": [M], "next_cursor": string or null}`.
- Output is deterministic: all maps are emitted from sorted keys.

Steps:

- [ ] Step 1: Write `gen/openapi/doc_test.go`:

```go
package openapi

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/azrtydxb/momo/gen"
	"github.com/azrtydxb/momo/internal/golden"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestOpenAPIDocumentGolden(t *testing.T) {
	root, err := filepath.Abs("../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	doc, ds := Document(g, Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1"})
	if len(ds) != 0 {
		t.Fatalf("emit diagnostics: %v", ds)
	}
	ok, err := golden.Diff("testdata/example.openapi.json", doc, *update)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("document differs from golden; review and rerun with -update")
	}
}
```

- [ ] Step 2: Run `go test ./gen/openapi -run TestOpenAPIDocumentGolden -count=1`; expect FAIL with `undefined: Document`.

- [ ] Step 3: Implement `gen/openapi/doc.go` as specified, using ordered structs or sorted maps.

- [ ] Step 4: Run `go test ./gen/openapi -run TestOpenAPIDocumentGolden -count=1 -update`, review `testdata/example.openapi.json` against the 3.1 rules above, then run the test without `-update`; expect `ok`.

- [ ] Step 5: Commit with `git add gen/openapi && git commit -m "feat(openapi): OpenAPI 3.1 document of the example models"`.

## Task 14: OpenAPI handlers and TestOpenAPIGenerator (validation and HTTP)

Delivers: optional `net/http` handlers over `*momo.DB`, and the single acceptance test `TestOpenAPIGenerator`. It validates the document against the official OpenAPI 3.1 meta-schema, checks the committed document and handlers, and exercises the handlers over HTTP on PostgreSQL. The document's Document schema and the response keys must agree.

Files:
- Create: `gen/openapi/handlers.go`
- Create: `gen/openapi/plugin.go` (writes `<Dir>/openapi.json` through `w.Text`, and `<Dir>/handlers_gen.go` through `w.File` only when `Options.Handlers` is true)
- Create: `internal/integration/openapi/openapi_test.go` (package `openapi_test`)
- Create: `internal/integration/openapi/example/handlers/handlers_gen.go` (committed generated output)
- Create: `internal/integration/openapi/testdata/oas31-schema.json` and `oas31-dialect-base.json` (vendored, SHA-256 pinned in `testdata/SHA256SUMS`)

Interfaces:
- `func Handlers(g *gen.Graph, pkg string, opts Options) ([]byte, []gen.Diagnostic)` emits, per model `M`, `type MHandler struct { DB *momo.DB }` and `func (h *MHandler) Routes(mux *http.ServeMux)`, with Go 1.22 patterns `GET /v1/<plural>`, `POST /v1/<plural>`, `GET /v1/<plural>/{id}`, `PUT /v1/<plural>/{id}`, `DELETE /v1/<plural>/{id}`.
- Handlers use only the generated typed builders: `momo.Select` for list and get, `momo.Insert` with `.Model(&m)` for create, `momo.Update` with `.Model(&m)` for put, `momo.Delete` filtered by the path id. List uses `apiquery.ClampLimit`, `apiquery.DecodeCursor` applied as the typed keyset `After` on the primary key, `OrderBy` the primary key ascending, and `apiquery.ParseFilter` over `ColumnByName` for `filter[<column>]=<value>`. The cursor encodes the last item's primary key.
- Response objects carry JSON keys equal to the column names, and exactly the column properties of the `M` schema.
- Errors: unknown filter column or unparseable value `400` with `{"error":"..."}`; missing row `404`; request bodies are limited with `http.MaxBytesReader` to a named 1 MiB constant.
- Test-only DB: `testopen.Postgres(t)` with `fixture.Apply(t, db)` (the shared fixture of plan momo-10 Task 9).

Steps:

- [ ] Step 1: Vendor the two schema documents into `internal/integration/openapi/testdata/` and pin their checksums:

```sh
cd internal/integration/openapi/testdata
curl -sSfL https://spec.openapis.org/oas/3.1/schema/2022-10-07 -o oas31-schema.json
curl -sSfL https://spec.openapis.org/oas/3.1/dialect/base -o oas31-dialect-base.json
shasum -a 256 oas31-schema.json oas31-dialect-base.json > SHA256SUMS
```

- [ ] Step 2: Write `internal/integration/openapi/openapi_test.go`:

```go
package openapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/azrtydxb/momo/gen"
	ggopenapi "github.com/azrtydxb/momo/gen/openapi"
	"github.com/azrtydxb/momo/internal/golden"
	"github.com/azrtydxb/momo/internal/integration/fixture"
	"github.com/azrtydxb/momo/internal/integration/openapi/example/handlers"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

const (
	metaURL    = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	dialectURL = "https://spec.openapis.org/oas/3.1/dialect/base"
)

func TestOpenAPIGenerator(t *testing.T) {
	models, err := filepath.Abs("../../../internal/testmodels/example")
	if err != nil {
		t.Fatal(err)
	}
	g, diags, err := gen.Load([]string{"./"}, gen.Config{Dir: models})
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("diagnostics: %v", diags)
	}
	opts := ggopenapi.Options{Title: "momo example", Version: "1.0.0", BasePath: "/v1"}
	doc, ds := ggopenapi.Document(g, opts)
	if len(ds) != 0 {
		t.Fatalf("document diagnostics: %v", ds)
	}

	// 1. the committed document is current
	ok, err := golden.Diff("../../../gen/openapi/testdata/example.openapi.json", doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("committed OpenAPI document is stale; regenerate it")
	}

	// 2. the document is valid OpenAPI 3.1 against the official meta-schema
	c := jsonschema.NewCompiler()
	for url, file := range map[string]string{dialectURL: "testdata/oas31-dialect-base.json", metaURL: "testdata/oas31-schema.json"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		schema, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.AddResource(url, schema); err != nil {
			t.Fatal(err)
		}
	}
	meta, err := c.Compile(metaURL)
	if err != nil {
		t.Fatal(err)
	}
	var doc31 any
	if err := json.Unmarshal(doc, &doc31); err != nil {
		t.Fatal(err)
	}
	if err := meta.Validate(doc31); err != nil {
		t.Fatalf("document is not OpenAPI 3.1: %v", err)
	}

	// 3. the committed handlers are current
	hsrc, hds := ggopenapi.Handlers(g, "handlers", opts)
	if len(hds) != 0 {
		t.Fatalf("handler diagnostics: %v", hds)
	}
	ok, err = golden.Diff("example/handlers/handlers_gen.go", hsrc, false)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("committed handlers are stale; regenerate them")
	}

	// 4. the handlers over HTTP, backed by PostgreSQL
	db := testopen.Postgres(t)
	fixture.Apply(t, db)
	mux := http.NewServeMux()
	(&handlers.DocumentHandler{DB: db}).Routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/v1/documents", "application/json",
		strings.NewReader(`{"tenant_id":1,"title":"openapi","status":"draft"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	decodeClose(t, resp, http.StatusCreated, &created)
	id := fmt.Sprint(created["id"])

	var got map[string]any
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents/"+id), http.StatusOK, &got)
	want := schemaProperties(t, doc, "Document")
	if have := keysOf(got); !equalStrings(have, want) {
		t.Fatalf("GET keys %v do not match Document schema properties %v", have, want)
	}

	var list struct {
		Items      []map[string]any `json:"items"`
		NextCursor *string          `json:"next_cursor"`
	}
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?filter[title]=openapi&limit=10"), http.StatusOK, &list)
	if len(list.Items) != 1 {
		t.Fatalf("filtered list has %d items, want 1", len(list.Items))
	}

	var bad map[string]string
	decodeClose(t, mustGet(t, srv.URL+"/v1/documents?filter[nope]=x"), http.StatusBadRequest, &bad)
	if bad["error"] == "" {
		t.Fatal("400 response lacks an error message")
	}

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/v1/documents/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	dresp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := dresp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	gone := mustGet(t, srv.URL+"/v1/documents/"+id)
	if err := gone.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("GET after DELETE: status %d, want 404", gone.StatusCode)
	}
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decodeClose(t *testing.T, resp *http.Response, status int, v any) {
	t.Helper()
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	if resp.StatusCode != status {
		t.Fatalf("status %d, want %d", resp.StatusCode, status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func schemaProperties(t *testing.T, doc []byte, name string) []string {
	t.Helper()
	var d struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(doc, &d); err != nil {
		t.Fatal(err)
	}
	s, ok := d.Components.Schemas[name]
	if !ok {
		t.Fatalf("schema %s missing", name)
	}
	out := make([]string, 0, len(s.Properties))
	for k := range s.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func equalStrings(a, b []string) bool {
	return bytes.Equal([]byte(strings.Join(a, "\x00")), []byte(strings.Join(b, "\x00")))
}
```

- [ ] Step 3: Run `cd internal/integration && go test ./openapi -run TestOpenAPIGenerator -count=1`; expect FAIL with `undefined: ggopenapi.Handlers` or `committed handlers are stale`.

- [ ] Step 4: Implement `gen/openapi/handlers.go` and `gen/openapi/plugin.go` as specified. Generate `internal/integration/openapi/example/handlers/handlers_gen.go` with the plugin and commit it; the test then checks it.

- [ ] Step 5: Run `cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 go test -count=1 ./openapi`; expect `ok`.

- [ ] Step 6: Commit with `git add gen/openapi internal/integration/openapi && git commit -m "feat(openapi): OpenAPI 3.1 validation and net/http handlers over PostgreSQL"`.

## Task 15: Plugin registration from momo.yaml

Delivers: the `plugins:` section of momo.yaml selects and configures the four ecosystem plugins, with no global registry. Acceptance: `TestPluginRegistration`.

Files:
- Create: `cmd/momo/plugins.go`
- Create: `cmd/momo/plugins_test.go`
- Modify: `cmd/momo/cmd.go` (`plugins()` returns the plugins configured in momo.yaml)

Interfaces:
- `func buildPlugins(cfg map[string]map[string]any, out string) ([]gen.Plugin, error)`. Keys: `graphql` (options `dir`, `package`, `models_package`), `grpc` (`dir`, `package`, `proto_package`), `openapi` (`dir`, `title`, `version`, `base_path`, `handlers` bool), `viz` (`format`, `dir`). Unknown plugin names are an error `unknown plugin "<name>"`; unknown option keys are an error `unknown option "<key>" for plugin "<name>"`.
- The list is built in the order of the sorted keys of `cfg`, so the output order is deterministic.

- [ ] Step 1: Write `cmd/momo/plugins_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestPluginRegistration(t *testing.T) {
	ps, err := buildPlugins(map[string]map[string]any{
		"viz":     {"format": "mermaid", "dir": "docs/er"},
		"openapi": {"dir": "api", "title": "t", "version": "1", "base_path": "/v1", "handlers": true},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name())
	}
	if strings.Join(names, ",") != "openapi,viz" {
		t.Fatalf("order=%v", names)
	}
	if _, err := buildPlugins(map[string]map[string]any{"soap": {}}, ""); err == nil || !strings.Contains(err.Error(), `unknown plugin "soap"`) {
		t.Fatalf("unknown plugin: %v", err)
	}
	if _, err := buildPlugins(map[string]map[string]any{"viz": {"colour": "red"}}, ""); err == nil || !strings.Contains(err.Error(), `unknown option "colour"`) {
		t.Fatalf("unknown option: %v", err)
	}
}
```

- [ ] Step 2: Run `go test ./cmd/momo -run TestPluginRegistration -count=1`; expect FAIL with `undefined: buildPlugins`.

- [ ] Step 3: Implement `cmd/momo/plugins.go` and make `plugins()` in `cmd/momo/cmd.go` read the `plugins:` map from the loaded `momo.yaml` and call `buildPlugins`.

- [ ] Step 4: Run `go test ./cmd/momo -count=1`; expect `ok`.

- [ ] Step 5: Commit with `git add cmd/momo`, message `feat(cli): configure ecosystem plugins from momo.yaml`.

## Task 16: Gate for the ecosystem generators

Delivers: one verification pass over every test named by S-63 to S-66, with the integration tests run under `MOMO_REQUIRE_INTEGRATION=1`. Acceptance: `TestGraphQLGenerator`, `TestProtoGenerator`, `TestOpenAPIGenerator`, `TestViz` pass with `-race`.

Files:
- Modify: none (verification; a failing test is fixed in the task that owns its file)

- [ ] Step 1: Run `go test -race -count=1 ./gen/apiquery ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi ./cmd/momo`; expect every package `ok`.

- [ ] Step 2: Run `cd internal/integration && MOMO_REQUIRE_INTEGRATION=1 PROTOC=$(pwd)/bin/protoc go test -race -count=1 ./...`; expect `ok`.

- [ ] Step 3: Run `go test -count=1 -v ./gen/viz ./gen/graphql ./gen/grpc ./gen/openapi -run 'TestViz|TestGraphQLSDL|TestGraphQLScaffold|TestProtoLockStableNumbers|TestOpenAPIDocumentGolden'` and confirm each named test shows `--- PASS`.

- [ ] Step 4: Run `go list -deps ./momo ./gen/apiquery | grep -E 'gqlgen|grpc|jsonschema|x/tools'`; expect no output, so the runtime and the shared helpers stay out of the framework graph.

- [ ] Step 5: Record the results in the pull request description for this plan. No commit is made by this task.
