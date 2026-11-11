// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"testing"

	"github.com/azrtydxb/momo/gen/naming"
)

type cachedPlan struct {
	ID    int64
	Title string
}

// Catches: the reflection plan rebuilt on every call instead of once per type
// (the per-row cost would then include tag parsing and field walking).
func TestStructMetaIsBuiltOnceAndCached(t *testing.T) {
	a, err := structMeta[cachedPlan]()
	if err != nil {
		t.Fatal(err)
	}
	b, err := structMeta[cachedPlan]()
	if err != nil || a != b {
		t.Fatalf("second call returned a different plan (%p, %p), err %v", a, b, err)
	}
	e1, err1 := structMeta[int]()
	e2, err2 := structMeta[int]()
	if err1 == nil || err2 == nil || e1 != nil || e2 != nil {
		t.Fatalf("a non-struct must fail every time: %v, %v", err1, err2)
	}
}

// Catches: a snake_case rule that splits acronyms, digits or single letters
// differently from the documented one.
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
	} {
		if got := snakeCase(in); got != want {
			t.Errorf("snakeCase(%q) = %q, want %q", in, got, want)
		}
		// snakeCase is a copy of naming.ToSnake, which the root package does
		// not import: the column a plain struct field gets must be the one
		// the generator gives the same field.
		if got := naming.ToSnake(in); got != want {
			t.Errorf("naming.ToSnake(%q) = %q, want %q: the two copies drifted", in, got, want)
		}
	}
}
