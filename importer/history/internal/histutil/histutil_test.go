// SPDX-License-Identifier: Apache-2.0

package histutil

import "testing"

func TestCheckDuplicates(t *testing.T) {
	if err := CheckDuplicates([]string{"1", "2", "10"}); err != nil {
		t.Fatal(err)
	}
	for _, vs := range [][]string{{"1", "01"}, {"00001", "1"}, {"0", "000"}} {
		if err := CheckDuplicates(vs); err == nil {
			t.Errorf("%v: want a duplicate error", vs)
		}
	}
}
