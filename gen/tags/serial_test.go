// SPDX-License-Identifier: Apache-2.0

package tags

import "testing"

func TestParseSerial(t *testing.T) {
	got, probs := Parse("pk,serial")
	if len(probs) != 0 || !got.PK || !got.Serial || got.Identity {
		t.Fatalf("pk,serial = %+v %+v", got, probs)
	}
	if _, probs := Parse("serial:5"); len(probs) == 0 {
		t.Fatal("serial takes no value")
	}
	if _, probs := Parse("serial,serial"); len(probs) == 0 {
		t.Fatal("duplicate serial must be reported")
	}
}
