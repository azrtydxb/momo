// SPDX-License-Identifier: Apache-2.0

package tags

import "testing"

func TestIdentityKinds(t *testing.T) {
	for _, tc := range []struct {
		raw          string
		identity     bool
		always       bool
		wantProblems int
	}{
		{"identity", true, false, 0},
		{"identity:by_default", true, false, 0},
		{"identity:always", true, true, 0},
		{"identity:sometimes", false, false, 1},
		{"identity:", false, false, 1},
	} {
		got, probs := Parse("pk," + tc.raw)
		if len(probs) != tc.wantProblems || got.Identity != tc.identity || got.IdentityAlways != tc.always {
			t.Errorf("%q: identity=%v always=%v problems=%+v", tc.raw, got.Identity, got.IdentityAlways, probs)
		}
	}
	if _, probs := Parse("identity:always,serial"); len(probs) != 0 {
		t.Errorf("combination is checked by the schema builder, not the parser: %+v", probs)
	}
}
