// SPDX-License-Identifier: Apache-2.0

package momo

import "testing"

func TestNormalizeShape(t *testing.T) {
	cases := []struct{ in, want string }{
		{`SELECT * FROM "a" WHERE "a"."id" IN (?, ?, ?)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" IN ($1, $2)`, `SELECT * FROM "a" WHERE "a"."id" IN (?)`},
		{`SELECT * FROM "a" WHERE "a"."id" = $3`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE "a"."id" = ?`, `SELECT * FROM "a" WHERE "a"."id" = ?`},
		{`SELECT * FROM "a" WHERE ("x", "y") IN ((?, ?), (?, ?), (?, ?))`, `SELECT * FROM "a" WHERE ("x", "y") IN ((?))`},
	}
	for _, c := range cases {
		if got := normalizeShape(c.in); got != c.want {
			t.Errorf("normalizeShape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
