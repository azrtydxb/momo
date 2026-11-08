// SPDX-License-Identifier: Apache-2.0

package momo

import (
	"sync"
	"testing"
)

// TestIDGeneratorsConcurrent runs the built-in generators from many goroutines;
// under -race it proves their shared state is guarded.
func TestIDGeneratorsConcurrent(t *testing.T) {
	for name, gen := range map[string]func() string{"cuid": newCUID, "ulid": newULID, "uuid": newUUID} {
		const workers, each = 16, 400
		var wg sync.WaitGroup
		out := make([][]string, workers)
		for w := range out {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ids := make([]string, 0, each)
				for range each {
					ids = append(ids, gen())
				}
				out[w] = ids
			}()
		}
		wg.Wait()
		seen := map[string]bool{}
		for _, ids := range out {
			for i, id := range ids {
				if id == "" || seen[id] {
					t.Fatalf("%s: empty or duplicate id %q", name, id)
				}
				seen[id] = true
				if name == "ulid" && i > 0 && ids[i-1] >= id {
					t.Fatalf("ulid %q does not sort after %q within one goroutine", id, ids[i-1])
				}
			}
		}
	}
}

func TestULIDOverflowReturnsEmpty(t *testing.T) {
	ulidState.Lock()
	savedMs, savedRand := ulidState.ms, ulidState.rand
	ulidState.ms = 1 << 46 // far in the future, so the clock never passes it
	for i := range ulidState.rand {
		ulidState.rand[i] = 0xff
	}
	ulidState.Unlock()
	defer func() {
		ulidState.Lock()
		ulidState.ms, ulidState.rand = savedMs, savedRand
		ulidState.Unlock()
	}()
	if id := newULID(); id != "" {
		t.Fatalf("overflow produced %q", id)
	}
	if id := newULID(); id != "" {
		t.Fatalf("overflow must not wrap on the next call: %q", id)
	}
}
