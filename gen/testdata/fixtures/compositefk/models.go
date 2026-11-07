// SPDX-License-Identifier: Apache-2.0

// Package compositefk has a foreign key on a column that is part of the
// referencing table's composite primary key.
package compositefk

import "github.com/azrtydxb/momo"

type License struct {
	momo.Model `momo:"table:licenses"`
	LicenseID  int64  `momo:"pk,column:license_id"`
	Name       string `momo:"notnull"`

	Exams []Exam `momo:"has_many,fk:LicenseID"`
}

type Exam struct {
	momo.Model `momo:"table:exams"`
	LicenseID  int64  `momo:"pk,column:license_id"`
	ExamID     int64  `momo:"pk,column:exam_id"`
	Title      string `momo:"notnull"`

	License *License `momo:"belongs_to,fk:LicenseID"`
}
