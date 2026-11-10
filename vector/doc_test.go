// SPDX-License-Identifier: Apache-2.0

package vector

import (
	"os"
	"strings"
	"testing"
)

func TestVectorDocPins(t *testing.T) {
	raw, err := os.ReadFile("../docs/vectors.md")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(raw)
	for _, want := range []string{"sqlite-vec", "0.8.7", "vec_version", "momo.HybridSearch", "fewer than k rows", "hamming"} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/vectors.md lacks %q", want)
		}
	}
	readme, err := os.ReadFile("../docs/README.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(readme), "vectors.md") {
		t.Error("docs/README.md has no row for vectors.md")
	}
}
