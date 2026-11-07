// SPDX-License-Identifier: Apache-2.0

package pgxdriver

// blockingSource yields one row, then blocks in Next until released.
type blockingSource struct {
	started chan struct{}
	release chan struct{}
	i       int
}

func (s *blockingSource) Next() bool {
	s.i++
	if s.i == 2 {
		close(s.started)
		<-s.release
	}
	return s.i <= 2
}
func (s *blockingSource) Values() ([]any, error) { return []any{int64(s.i)}, nil }
func (s *blockingSource) Err() error             { return nil }
