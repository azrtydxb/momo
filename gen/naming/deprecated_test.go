// SPDX-License-Identifier: Apache-2.0

package naming

import (
	"testing"

	"github.com/azrtydxb/momo/internal/pluralize"
	"github.com/azrtydxb/momo/internal/snake"
)

// Catches: a deprecated forwarder drifting from the function it stands for.
func TestDeprecatedForwarders(t *testing.T) {
	for _, s := range []string{"", "UserID", "RoleIDs", "HTTPServer", "already_snake"} {
		if got, want := ToSnake(s), snake.ToSnake(s); got != want {
			t.Errorf("ToSnake(%q) = %q, want %q", s, got, want)
		}
	}
	for _, s := range []string{"user", "box", "city", "person", "audit_log"} {
		if got, want := Plural(s), pluralize.Plural(s); got != want {
			t.Errorf("Plural(%q) = %q, want %q", s, got, want)
		}
	}
}
