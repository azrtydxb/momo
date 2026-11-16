// SPDX-License-Identifier: Apache-2.0

package companionsequence

import "github.com/azrtydxb/momo"

type Job struct {
	momo.Model `momo:"table:jobs"`
	ID         int64 `momo:"pk"`
}

func (j *Job) MomoConfig(c *momo.Config) {
	c.Default(&j.ID, "nextval('jobs_seq'::regclass)")
	c.Sequence("jobs_seq")
	c.Sequence("ticket_seq").Schema("app").As("integer").Start(1000).Increment(10).MinValue(1000).MaxValue(99999).Cache(20).Cycle()
	c.Sequence("counter_seq").Start(0).MinValue(0)
	c.Sequence("countdown_seq").Increment(-1).MaxValue(-1).MinValue(-9223372036854775808)
}
