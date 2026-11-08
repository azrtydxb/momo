// SPDX-License-Identifier: Apache-2.0

package momovet

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// TestMomovet runs the analyzer over testdata/src/raw: constants and
// constant expressions pass; concatenation, a variable and a format call are
// reported at the SQL argument of each raw-SQL function.
func TestMomovet(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "raw")
}
