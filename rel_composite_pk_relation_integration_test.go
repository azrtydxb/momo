// SPDX-License-Identifier: Apache-2.0

//go:build integration

package momo_test

import (
	"context"
	"testing"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/internal/testdb/testopen"
)

// The models and descriptors below mirror what the generator emits for
// gen/testdata/fixtures/compositefk (issue 13): the generated descriptors live
// in a scratch package that is compiled by gen.TestCompositeFKGeneratedCodeBuilds,
// and cannot be imported from here, so they are written out by hand with the
// same keys. A child (cpExam) whose primary key (license_id, exam_id) contains
// its foreign key reaches its parent through a belongs_to, and the parent
// reaches the children through a has_many.

type cpLicense struct {
	LicenseID int64
	Name      string
	Exams     []cpExam
}

type cpExam struct {
	LicenseID int64
	ExamID    int64
	Title     string
	License   *cpLicense
}

var cpLicenseMeta = &momo.Meta[cpLicense]{
	Table:   "cp_licenses",
	Columns: []momo.ColumnMeta{{Name: "license_id", Flags: momo.FlagPK}, {Name: "name"}},
	Ptr: func(m *cpLicense, i int) any {
		return []any{&m.LicenseID, &m.Name}[i]
	},
	Value: func(m *cpLicense, i int) any {
		return []any{m.LicenseID, m.Name}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1, PK: []int{0},
}

func (*cpLicense) MomoMeta() *momo.Meta[cpLicense] { return cpLicenseMeta }

var cpExamMeta = &momo.Meta[cpExam]{
	Table:   "cp_exams",
	Columns: []momo.ColumnMeta{{Name: "license_id", Flags: momo.FlagPK}, {Name: "exam_id", Flags: momo.FlagPK}, {Name: "title"}},
	Ptr: func(m *cpExam, i int) any {
		return []any{&m.LicenseID, &m.ExamID, &m.Title}[i]
	},
	Value: func(m *cpExam, i int) any {
		return []any{m.LicenseID, m.ExamID, m.Title}[i]
	},
	Identity: -1, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1, PK: []int{0, 1},
}

func (*cpExam) MomoMeta() *momo.Meta[cpExam] { return cpExamMeta }

var cpExams = momo.Relation[cpLicense, cpExam]{
	Name: "Exams", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "cp_licenses"}, Child: momo.TableRef{Name: "cp_exams"},
	ParentMeta: cpLicenseMeta, ChildMeta: cpExamMeta,
	ParentCols: []string{"license_id"}, ChildCols: []string{"license_id"}, ChildPK: []string{"license_id", "exam_id"},
	ParentKey:   func(p *cpLicense) ([]any, bool) { return []any{p.LicenseID}, true },
	ChildKey:    func(c *cpExam) ([]any, bool) { return []any{c.LicenseID}, true },
	SetChildKey: func(c *cpExam, k []any) { c.LicenseID = k[0].(int64) },
	GetMany:     func(p *cpLicense) []cpExam { return p.Exams },
	SetMany:     func(p *cpLicense, cs []cpExam) { p.Exams = cs },
}

var cpLicenseRel = momo.Relation[cpExam, cpLicense]{
	Name: "License", Kind: momo.BelongsTo,
	Parent: momo.TableRef{Name: "cp_exams"}, Child: momo.TableRef{Name: "cp_licenses"},
	ParentMeta: cpExamMeta, ChildMeta: cpLicenseMeta,
	ParentCols: []string{"license_id"}, ChildCols: []string{"license_id"}, ChildPK: []string{"license_id"},
	ParentKey:    func(p *cpExam) ([]any, bool) { return []any{p.LicenseID}, true },
	ChildKey:     func(c *cpLicense) ([]any, bool) { return []any{c.LicenseID}, true },
	SetParentKey: func(p *cpExam, k []any) { p.LicenseID = k[0].(int64) },
	GetOne:       func(p *cpExam) *cpLicense { return p.License },
	SetOne:       func(p *cpExam, c *cpLicense) { p.License = c },
}

// TestCompositePKChildRelationsIntegration covers CreateGraph, Preload and a
// joined belongs_to over a child whose foreign key is part of its composite
// primary key.
func TestCompositePKChildRelationsIntegration(t *testing.T) {
	db := testopen.Postgres(t)
	ctx := context.Background()
	mustExecPG(t, db, `CREATE TABLE cp_licenses (license_id bigint PRIMARY KEY, name text NOT NULL)`)
	mustExecPG(t, db, `CREATE TABLE cp_exams (license_id bigint NOT NULL REFERENCES cp_licenses(license_id), exam_id bigint NOT NULL, title text NOT NULL, PRIMARY KEY (license_id, exam_id))`)

	lic := &cpLicense{LicenseID: 1, Name: "driving", Exams: []cpExam{{ExamID: 1, Title: "theory"}, {ExamID: 2, Title: "practice"}}}
	if err := momo.CreateGraph(ctx, db, lic, cpExams.Include()); err != nil {
		t.Fatal(err)
	}
	if lic.Exams[0].LicenseID != 1 || lic.Exams[1].LicenseID != 1 {
		t.Fatalf("child keys not written back: %+v", lic.Exams)
	}
	other := &cpLicense{LicenseID: 2, Name: "boat", Exams: []cpExam{{ExamID: 1, Title: "sea"}}}
	if err := momo.CreateGraph(ctx, db, other, cpExams.Include()); err != nil {
		t.Fatal(err)
	}

	ls, err := momo.Select[cpLicense, *cpLicense](db).Preload(cpExams).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[int64][]string{}
	for _, l := range ls {
		for _, e := range l.Exams {
			got[l.LicenseID] = append(got[l.LicenseID], e.Title)
		}
	}
	if len(got[1]) != 2 || len(got[2]) != 1 || got[2][0] != "sea" {
		t.Fatalf("preloaded exams: %v", got)
	}

	es, err := momo.Select[cpExam, *cpExam](db).Preload(cpLicenseRel.Joined()).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 {
		t.Fatalf("exams: %d", len(es))
	}
	for _, e := range es {
		if e.License == nil || e.License.LicenseID != e.LicenseID {
			t.Fatalf("joined license of %+v: %+v", e, e.License)
		}
	}

	// The foreign key is enforced: an exam for an unknown license is refused.
	if _, err := momo.Exec(ctx, db, `INSERT INTO cp_exams VALUES (99, 1, 'orphan')`); err == nil {
		t.Fatal("the foreign key must reject an exam without a license")
	}
}
