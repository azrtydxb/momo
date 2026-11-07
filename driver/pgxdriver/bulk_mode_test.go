// SPDX-License-Identifier: Apache-2.0

package pgxdriver

import (
	"context"
)

// cancelSource streams rows and cancels the context after n of them.
type cancelSource struct {
	i, n   int
	cancel context.CancelFunc
}

func (s *cancelSource) Next() bool {
	s.i++
	if s.i == s.n {
		s.cancel()
	}
	return s.i <= 5_000_000
}
func (s *cancelSource) Values() ([]any, error) { return []any{int64(s.i)}, nil }
func (s *cancelSource) Err() error             { return nil }
