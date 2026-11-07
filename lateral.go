// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"errors"
	"fmt"

	"github.com/azrtydxb/momo/dialect"
)

type lateralSource struct {
	q     Subquery
	alias string
}

// Lateral uses a subquery as a LATERAL FROM/JOIN source under alias. The
// subquery may reference columns of the FROM and JOIN sources before it:
//
//	momo.Select[row](db).
//		From(accounts).
//		CrossJoin(momo.Lateral(latestOrder, "lo"))
//
// renders CROSS JOIN LATERAL (...) AS "lo". A LEFT JOIN needs an ON
// condition, so pass momo.SQL("true") when there is nothing to join on.
// Reference the subquery's columns with ColumnOf. PostgreSQL and MySQL 8
// support LATERAL; SQLite and MariaDB fail the build with ErrUnsupported.
func Lateral(q Subquery, alias string) TableSource { return lateralSource{q, alias} }

func (s lateralSource) Qualifier() string { return s.alias }

func (s lateralSource) appendFrom(b *Builder) {
	if b.d.Name == dialect.SQLite || (b.d.Name == dialect.MySQL && b.d.Flavor == dialect.FlavorMariaDB) {
		b.unsupported("LATERAL")
		return
	}
	if s.q == nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: errors.New("Lateral needs a subquery")})
		return
	}
	if err := validName(s.alias); err != nil {
		b.SetErr(&Error{Kind: ErrValidation, Op: "build", Err: fmt.Errorf("LATERAL alias %q: %w", s.alias, err)})
		return
	}
	b.WriteString("LATERAL ")
	s.q.AppendSQL(b)
	b.WriteString(" AS ")
	b.Ident(s.alias)
}
