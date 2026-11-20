// SPDX-License-Identifier: Apache-2.0

package outbox

import "time"

// WithRand replaces the random source of the retry jitter, a function that
// returns a value in [0, 1), so a test can pin the delay.
func WithRand(fn func() float64) RelayOption { return func(c *relayConfig) { c.rand = fn } }

// Backoff returns the delay after the attempt-th failed attempt.
func (r *Relay) Backoff(attempt int) time.Duration { return r.backoff(attempt) }
