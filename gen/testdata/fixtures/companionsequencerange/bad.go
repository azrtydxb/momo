// SPDX-License-Identifier: Apache-2.0

package companionsequencerange

import "github.com/azrtydxb/momo"

type Job struct {
	momo.Model `momo:"table:jobs"`
	ID         int64 `momo:"pk"`
}

func (j *Job) MomoConfig(c *momo.Config) {
	c.Sequence("bad_type_seq").As("text")
	c.Sequence("min_above_max_seq").MinValue(10).MaxValue(5)
	c.Sequence("start_low_seq").MinValue(10).Start(3)
	c.Sequence("start_high_seq").MaxValue(10).Start(11)
	c.Sequence("max_over_type_seq").As("smallint").MaxValue(40000)
	c.Sequence("min_under_type_seq").As("integer").Increment(-1).MinValue(-3000000000)
	c.Sequence("max_below_default_min_seq").MaxValue(0)
	c.Sequence("fine_seq").As("integer").MinValue(5).MaxValue(50).Start(7)
}
