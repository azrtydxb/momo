// SPDX-License-Identifier: Apache-2.0

package companionsequencebad

import "github.com/azrtydxb/momo"

const step = 5

type Job struct {
	momo.Model `momo:"table:jobs"`
	ID         int64 `momo:"pk"`
}

func (j *Job) MomoConfig(c *momo.Config) {
	c.Sequence("dup_seq").Cache(2).Cache(3)
	c.Sequence("named_step_seq").Increment(step)
	c.Sequence("zero_seq").Increment(0)
	c.Sequence("same_seq")
	c.Sequence("same_seq")
}
