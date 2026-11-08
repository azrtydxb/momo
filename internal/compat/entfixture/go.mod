module github.com/azrtydxb/momo/internal/compat/entfixture

go 1.26.10

require (
	entgo.io/ent v0.14.6
	github.com/azrtydxb/momo v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.12.0
)

replace github.com/azrtydxb/momo => ../../..
