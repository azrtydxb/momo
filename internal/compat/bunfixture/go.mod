module github.com/azrtydxb/momo/internal/compat/bunfixture

go 1.26.10

require (
	github.com/azrtydxb/momo v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.12.0
	github.com/uptrace/bun v1.3.0
	github.com/uptrace/bun/dialect/pgdialect v1.3.0
)

replace github.com/azrtydxb/momo => ../../..
