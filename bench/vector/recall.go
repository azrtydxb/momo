// SPDX-License-Identifier: Apache-2.0

package vector

// Recall returns the fraction of approximate ids that appear in the exact ids,
// averaged over queries. exact and approx are parallel: entry i is the result
// of query i. Queries whose exact result is empty are skipped; if none remain
// the recall is 1.
func Recall(exact, approx [][]int64) float64 {
	var sum float64
	var n int
	for i := range exact {
		if len(exact[i]) == 0 {
			continue
		}
		want := make(map[int64]struct{}, len(exact[i]))
		for _, id := range exact[i] {
			want[id] = struct{}{}
		}
		var hit int
		if i < len(approx) {
			for _, id := range approx[i] {
				if _, ok := want[id]; ok {
					hit++
				}
			}
		}
		sum += float64(hit) / float64(len(exact[i]))
		n++
	}
	if n == 0 {
		return 1
	}
	return sum / float64(n)
}
