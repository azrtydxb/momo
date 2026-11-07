# Supported versions

What Momo v0.1.0 is built and tested against. "Tested" means a job in
`.github/workflows/ci.yml` runs it. Anything not listed there is untested, not
necessarily broken.

## Module

| Item        | Value                                                                      |
| ----------- | -------------------------------------------------------------------------- |
| Module path | `github.com/azrtydxb/momo`                                                 |
| Version     | v0.1.0 (tags use a `v` prefix)                                             |
| Install     | `go get github.com/azrtydxb/momo@v0.1.0`                                   |
| CLI         | `go run github.com/azrtydxb/momo/cmd/momo` (or `go install` the same path) |
| Databases   | PostgreSQL with pgvector only; SQLite and MySQL are not usable yet         |

## Go

| Item                         | Value                                                                                                                                   |
| ---------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| Minimum (`go.mod` `go` line) | 1.26.0                                                                                                                                  |
| CI matrix                    | 1.26 and 1.27 for unit, compile-fail, race, fuzz smoke and integration jobs                                                             |
| Single-version CI jobs       | lint (1.26); bench smoke, govulncheck, Windows and macOS (1.27)                                                                         |
| Older than 1.26              | not supported                                                                                                                           |
| cgo                          | the macOS job sets `CGO_ENABLED=1`; Linux and Windows jobs set nothing; CONTRIBUTING lists a C toolchain for the future SQLite packages |

## Dependencies (`go.mod`)

| Module                           | Version                                                                                                        |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `github.com/jackc/pgx/v5`        | v5.11.0                                                                                                        |
| `go.yaml.in/yaml/v3`             | v3.0.6 (`momo.yaml`)                                                                                           |
| `golang.org/x/tools`             | v0.52.0 (generator)                                                                                            |
| `github.com/go-sql-driver/mysql` | v1.10.1; required by the module but used only by test harness code (`internal/testdb`). No MySQL driver ships. |

## Operating systems

| OS      | CI job           | What runs                                               |
| ------- | ---------------- | ------------------------------------------------------- |
| Linux   | `ubuntu-latest`  | every job, including PostgreSQL integration tests       |
| macOS   | `macos-latest`   | `CGO_ENABLED=1 go test ./...` (unit tests, no database) |
| Windows | `windows-latest` | `go test ./...` (unit tests, no database)               |

Integration tests (real PostgreSQL, pgvector, PgBouncer) run on Linux only.

## PostgreSQL

| Item                    | Value                                                                                                                                                 |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tested major versions   | 16 and 17 (images `pgvector/pgvector:0.8.7-pg16` and `0.8.7-pg17`)                                                                                    |
| Other majors            | untested; no minimum is enforced in code and none is documented                                                                                       |
| `MERGE` gate            | the driver exposes `server_version_num` (`ServerVersioner`) so a `MERGE` gate can require PostgreSQL 15; Momo has no public `MERGE` builder in v0.1.0 |
| Authentication in tests | password (`postgres` and `momo_app` roles); TLS is off in CI (`sslmode=disable`)                                                                      |
| Driver                  | pgx v5 pool through `pgxdriver.NewPool` or `pgxdriver.New`; `database/sql` through `pgxdriver.FromSQLDB`                                              |

## pgvector

| Item                                                                                     | Value                                                                                                                                                                                             |
| ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tested version                                                                           | 0.8.7 (pinned; `TestPgvectorVersionPinned` fails on any other version in CI)                                                                                                                      |
| Iterative scan, `hnsw.max_scan_tuples`, `hnsw.scan_mem_multiplier`, `ivfflat.max_probes` | 0.8.0 or newer; enforced: `SetSearch` returns `ErrUnsupported` on an older or missing extension                                                                                                   |
| `vector`, `halfvec`, `sparsevec`, `bit` types and the l1, hamming, jaccard operators     | no version check in Momo. Upstream pgvector added halfvec, sparsevec, l1 and jaccard in 0.7.0; that is not asserted by this repository, so treat 0.7.0 as an unverified lower bound and use 0.8.x |
| Installation                                                                             | server-side package or image; Momo never runs `CREATE EXTENSION` at connect time; a migration creates it and a missing extension surfaces as `ErrUnsupported`                                     |

## PgBouncer

| Item                  | Value                                                                                                                                                                                                                               |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Tested version        | 1.26.1 (image `edoburu/pgbouncer:v1.26.1-p0`)                                                                                                                                                                                       |
| Pool mode             | transaction                                                                                                                                                                                                                         |
| Client authentication | `scram-sha-256` (`AUTH_TYPE`)                                                                                                                                                                                                       |
| Prepared statements   | CI sets `MAX_PREPARED_STATEMENTS=0`; Momo's default exec mode creates no named prepared statements (checked on the wire by `TestPgBouncerNoNamedPreparedStatements`)                                                                |
| Exec modes            | default `pgx.QueryExecModeExec` is the tested one; `WithExecMode` allows `CacheStatement`, `CacheDescribe`, `DescribeExec`, which create or cache statements and are not tested through PgBouncer here; simple protocol is rejected |
| Session state         | the test image cannot track `search_path` set at startup; set it per transaction. `SearchSettings` and `Tx.SetLocal` are transaction-local and safe                                                                                 |
| Migrations            | do not run `momo migrate` through a transaction pooler: the lock is a session advisory lock; use a direct connection                                                                                                                |
| Other poolers         | untested (pgcat, Odyssey, RDS Proxy, Supavisor)                                                                                                                                                                                     |

## Feature version floors

| Feature                                               | Floor                                                                                                                          | Source                                  |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------- |
| Iterative vector scan settings                        | pgvector 0.8.0                                                                                                                 | `search.go` (`requireIterativeSupport`) |
| `MERGE` (gate only, no builder)                       | PostgreSQL 15                                                                                                                  | `driver_optional.go`                    |
| Window functions (`LoadPerParentLimit`)               | none enforced; tested on PostgreSQL 16 and 17                                                                                  | `load.go`, CI                           |
| `RETURNING`, `ON CONFLICT`, row locking, savepoints   | none enforced; tested on PostgreSQL 16 and 17                                                                                  | `dialect/dialect.go`, CI                |
| Generated identity columns, `OVERRIDING SYSTEM VALUE` | none enforced; tested on PostgreSQL 16 and 17                                                                                  | CI                                      |
| `momo.yaml`, migration files, `momo.sum`, snapshots   | format versions `1` (`FormatVersion`, `SnapshotFormat`, `SumFormat`); a newer snapshot format is refused with `ErrNewerFormat` | `migrate/`                              |

## Stability policy

| Item                | Statement                                                                                                                                                       |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pre-1.0             | the README states: "v0.1.0, pre-v1, with no compatibility promise between releases"                                                                             |
| Versioning          | release-please with `bump-minor-pre-major: true` and Conventional Commits; breaking changes are marked `feat!:` or a `BREAKING CHANGE:` footer (CONTRIBUTING)   |
| Deprecation process | none documented                                                                                                                                                 |
| Security fixes      | latest minor only (`SECURITY.md`)                                                                                                                               |
| Pin your dependency | use an exact version in `go.mod` and read `CHANGELOG.md` before each upgrade                                                                                    |
| Generated code      | regenerate with `momo generate` after every upgrade; generated files carry the generator version in their header and `momo generate --check` reports stale ones |
