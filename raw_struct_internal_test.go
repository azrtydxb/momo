// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/internal/snake"
)

type cachedPlan struct {
	ID    int64
	Title string
}

// Catches: the reflection plan rebuilt on every call instead of once per type
// (the per-row cost would then include tag parsing and field walking).
func TestStructMetaIsBuiltOnceAndCached(t *testing.T) {
	a, err := structMeta[cachedPlan](nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := structMeta[cachedPlan](nil)
	if err != nil || a != b {
		t.Fatalf("second call returned a different plan (%p, %p), err %v", a, b, err)
	}
	e1, err1 := structMeta[int](nil)
	e2, err2 := structMeta[int](nil)
	if err1 == nil || err2 == nil || e1 != nil || e2 != nil {
		t.Fatalf("a non-struct must fail every time: %v, %v", err1, err2)
	}
}

// Catches: the default column name of a plain struct field drifting from the
// generator's snake.ToSnake (acronyms, digits, single letters, plural initialisms).
func TestSnakeCase(t *testing.T) {
	for in, want := range map[string]string{
		"ID":         "id",
		"UserID":     "user_id",
		"Title":      "title",
		"HTTPServer": "http_server",
		"CreatedAt":  "created_at",
		"Title2":     "title2",
		"Line2Text":  "line2_text",
		"A":          "a",
		"URL":        "url",
		"OAuthLogin": "o_auth_login",
		"RoleIDs":    "role_ids",
	} {
		if got := snake.ToSnake(in); got != want {
			t.Errorf("ToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}
