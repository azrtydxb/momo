// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import "testing"

// Catches: a WithOnReconnect that does not store its function, and a deprecated
// OnReconnect that stops forwarding to it.
func TestOnReconnectOptions(t *testing.T) {
	for name, opt := range map[string]func(func()) ListenOption{
		"WithOnReconnect":   WithOnReconnect,
		"OnReconnect (old)": OnReconnect,
	} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			var c listenConfig
			opt(func() { calls++ })(&c)
			if c.onReconnect == nil {
				t.Fatal("the option did not set the reconnect function")
			}
			c.onReconnect()
			if calls != 1 {
				t.Fatalf("reconnect function ran %d times, want 1", calls)
			}
		})
	}
}
