// SPDX-License-Identifier: Apache-2.0

package migrate_test

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/azrtydxb/momo/migrate"
)

// The fold accepts a fork only when the two branches can be shown independent;
// these tests pin each rule.

// pTable is the table p with a primary key id of the given type.
func pTable(idType string) string {
	return `{"name":"p","columns":[{"name":"id","type":"` + idType + `"}],"primary_key":["id"]}`
}

func noKeyTable(name, idType string) string {
	return `{"name":"` + name + `","columns":[{"name":"id","type":"` + idType + `"}]}`
}

// childC is the table c with a foreign key to p(id).
func childC() string {
	return `{"name":"c","columns":[{"name":"id","type":"bigint"},{"name":"p_id","type":"bigint"}],` +
		`"foreign_keys":[{"name":"c_p","columns":["p_id"],"ref_table":"p","ref_columns":["id"]}]}`
}

// pbSnapshotWith is a snapshot with tables, views and functions given as JSON.
func pbSnapshotWith(version, parent string, tables []string, views, functions string) string {
	out := `{"format_version":1,"version":"` + version + `","parent":"` + parent +
		`","schema":{"format_version":1,"dialect":"postgres","tables":[` + strings.Join(tables, ",") + `]`
	if views != "" {
		out += `,"views":[` + views + `]`
	}
	if functions != "" {
		out += `,"functions":[` + functions + `]`
	}
	return out + `}}`
}

// pbTablesOnly is a snapshot with tables only.
func pbTablesOnly(version string, tables ...string) string {
	return pbSnapshotWith(version, pbV0, tables, "", "")
}

func pbBranchWith(base fstest.MapFS, version, name, snapshot string) fstest.MapFS {
	out := fstest.MapFS{}
	for k, v := range base {
		out[k] = v
	}
	out[version+"_"+name+".up.sql"] = pbFile("-- momo:generated\nSELECT 1;\n")
	out[version+"_"+name+".snapshot.json"] = pbFile(snapshot)
	return out
}

// rootWith is a directory whose root migration creates the tables.
func rootWith(tables ...string) fstest.MapFS {
	return fstest.MapFS{
		pbV0 + "_init.up.sql":        pbFile("-- momo:generated\nSELECT 1;\n"),
		pbV0 + "_init.snapshot.json": pbFile(pbSnapshotWith(pbV0, "", tables, "", "")),
	}
}

func wantConflict(t *testing.T, merged fstest.MapFS, parts ...string) {
	t.Helper()
	_, err := migrate.LatestState(merged)
	if !errors.Is(err, migrate.ErrBranchConflict) {
		t.Fatalf("want ErrBranchConflict, got %v", err)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("the error must contain %q: %v", p, err)
		}
	}
}

func wantMerge(t *testing.T, merged fstest.MapFS) {
	t.Helper()
	if _, err := migrate.LatestState(merged); err != nil {
		t.Fatalf("want a merge, got %v", err)
	}
	if p := pbProblems(t, merged); len(p) != 0 {
		t.Fatalf("validate: %v", p)
	}
}

// A hand-written migration whose snapshot is its merged-fork parent's own
// stored snapshot changes nothing and is valid, as is one that copies the
// derived state.
func TestCustomMigrationWithStoredParentSnapshot(t *testing.T) {
	custom := pbBranchWith(pbMergedIndependent(t), "20260103000000", "backfill", pbSnapshot("20260103000000", pbVB, pbTable("base"), pbTable("b")))
	custom["20260103000000_backfill.up.sql"] = pbFile("UPDATE b SET id = id;\n")
	custom = pbMerge(t, custom, fstest.MapFS{})
	wantMerge(t, custom)
	st, err := migrate.LatestState(custom)
	if err != nil || !sameStrings(tableNames(st.Schema), []string{"a", "b", "base"}) {
		t.Fatalf("a no-change migration keeps the derived state: %v %v", st, err)
	}
}

// pbSnapshotExtra is pbSnapshotWith with a raw JSON fragment, such as
// `"enums":[...]`, added to the schema.
func pbSnapshotExtra(version string, tables []string, extra string) string {
	s := pbSnapshotWith(version, pbV0, tables, "", "")
	return strings.TrimSuffix(s, "}}") + "," + extra + "}}"
}

// An enum, extension or schema that one branch removes or alters can be used
// by any table, view or function of the other, so the fork conflicts.
func TestForkSharedTypeRemovalConflictsWithTableChange(t *testing.T) {
	for name, tc := range map[string]struct{ extra, none string }{
		"enum":      {`"enums":[{"name":"e","values":["a","b"]}]`, `"enums":[]`},
		"extension": {`"extensions":[{"name":"citext"}]`, `"extensions":[]`},
		"schema":    {`"schemas":[{"name":"s"}]`, `"schemas":[]`},
	} {
		root := fstest.MapFS{
			pbV0 + "_init.up.sql":        pbFile("-- momo:generated\nSELECT 1;\n"),
			pbV0 + "_init.snapshot.json": pbFile(strings.Replace(pbSnapshotExtra(pbV0, []string{noKeyTable("t", "bigint")}, tc.extra), `"parent":"`+pbV0+`"`, `"parent":""`, 1)),
		}
		drop := pbBranchWith(root, pbVA, "drop_"+name, pbSnapshotExtra(pbVA, []string{noKeyTable("t", "bigint")}, tc.none))
		addTable := pbBranchWith(root, pbVB, "add_u", pbSnapshotExtra(pbVB, []string{noKeyTable("t", "bigint"), noKeyTable("u", "bigint")}, tc.extra))
		t.Run(name, func(t *testing.T) {
			wantConflict(t, pbMerge(t, drop, addTable), "enum, extension or schema")
		})
	}
}

// Adding an enum next to a table change is independent.
func TestForkAddingAnEnumMergesWithTableChange(t *testing.T) {
	root := rootWith(noKeyTable("t", "bigint"))
	add := pbBranchWith(root, pbVA, "add_e", pbSnapshotExtra(pbVA, []string{noKeyTable("t", "bigint")}, `"enums":[{"name":"e","values":["a"]}]`))
	table := pbBranchWith(root, pbVB, "add_u", pbTablesOnly(pbVB, noKeyTable("t", "bigint"), noKeyTable("u", "bigint")))
	wantMerge(t, pbMerge(t, add, table))
}

func TestForkFunctionAndFunctionConflict(t *testing.T) {
	root := rootWith(noKeyTable("t", "bigint"))
	fn := func(version, name string) fstest.MapFS {
		return pbBranchWith(root, version, "add_"+name, pbSnapshotWith(version, pbV0, []string{noKeyTable("t", "bigint")}, "",
			`{"name":"`+name+`","args":"","returns":"integer","language":"sql","volatility":"VOLATILE","body":"SELECT 1"}`))
	}
	wantConflict(t, pbMerge(t, fn(pbVA, "f"), fn(pbVB, "g")), "function, trigger or policy")
}

// A marked snapshot that equals the stored parent, not the derived one, changes
// the schema and is not a no-change migration.
func TestMarkedCustomMigrationMustEqualDerivedParent(t *testing.T) {
	merged := pbMergedIndependent(t)
	st, err := migrate.LatestState(merged)
	if err != nil {
		t.Fatal(err)
	}
	custom := pbBranchWith(merged, "20260103000000", "backfill", pbSnapshot("20260103000000", pbVB, pbTable("base"), pbTable("b")))
	custom["20260103000000_backfill.up.sql"] = pbFile("UPDATE b SET id = id;\n")
	custom["20260103000000_backfill.snapshot.json"] = pbFile(strings.Replace(string(custom["20260103000000_backfill.snapshot.json"].Data), `"parent":"`+pbVB+`"`, `"parent":"`+pbVB+`","base":"`+st.Base+`"`, 1))
	custom = pbMerge(t, custom, fstest.MapFS{})
	found := false
	for _, p := range pbProblems(t, custom) {
		found = found || strings.Contains(p, "must not change the schema")
	}
	if !found {
		t.Fatalf("a marked snapshot equal to the stored parent must be refused: %v", pbProblems(t, custom))
	}
}

// A base this momo does not know is refused, not read as the stored parent.
func TestReadSnapshotRefusesUnknownBase(t *testing.T) {
	good := pbSnapshot(pbV0, "", pbTable("base"))
	for name, base := range map[string]string{"unknown word": "future", "bare derived": "derived", "short hash": "derived:abc"} {
		src := strings.Replace(good, `"parent":""`, `"parent":"","base":"`+base+`"`, 1)
		if _, err := migrate.ReadSnapshot(strings.NewReader(src)); !errors.Is(err, migrate.ErrNewerFormat) {
			t.Errorf("%s: want ErrNewerFormat, got %v", name, err)
		}
	}
	if _, err := migrate.ReadSnapshot(strings.NewReader(good)); err != nil {
		t.Fatal(err)
	}
}

const viewOnT = `{"name":"v","query":"SELECT id FROM t","depends":["t"]}`

func TestForkForeignKeyToTableDroppedByOtherBranch(t *testing.T) {
	root := rootWith(pTable("bigint"))
	drop := pbBranchWith(root, pbVA, "drop_p", pbTablesOnly(pbVA))
	add := pbBranchWith(root, pbVB, "add_c", pbTablesOnly(pbVB, pTable("bigint"), childC()))
	wantConflict(t, pbMerge(t, drop, add), "c.c_p")
}

func TestForkForeignKeyReferencedColumnRetypedOrUnkeyed(t *testing.T) {
	root := rootWith(pTable("bigint"))
	add := pbBranchWith(root, pbVB, "add_c", pbTablesOnly(pbVB, pTable("bigint"), childC()))
	retype := pbBranchWith(root, pbVA, "retype_p", pbTablesOnly(pbVA, pTable("text")))
	wantConflict(t, pbMerge(t, retype, add), "c.c_p")
	unkey := pbBranchWith(root, pbVA, "unkey_p", pbTablesOnly(pbVA, noKeyTable("p", "bigint")))
	wantConflict(t, pbMerge(t, unkey, add), "c.c_p")
}

func TestForkWithValidForeignKeyMerges(t *testing.T) {
	root := rootWith(pTable("bigint"))
	add := pbBranchWith(root, pbVA, "add_c", pbTablesOnly(pbVA, pTable("bigint"), childC()))
	other := pbBranchWith(root, pbVB, "add_d", pbTablesOnly(pbVB, pTable("bigint"), noKeyTable("d", "bigint")))
	wantMerge(t, pbMerge(t, add, other))
}

func TestForkViewAndItsSourceTable(t *testing.T) {
	root := rootWith(noKeyTable("t", "bigint"), noKeyTable("u", "bigint"))
	view := pbBranchWith(root, pbVA, "add_v", pbSnapshotWith(pbVA, pbV0, []string{noKeyTable("t", "bigint"), noKeyTable("u", "bigint")}, viewOnT, ""))
	touchT := pbBranchWith(root, pbVB, "alter_t", pbTablesOnly(pbVB, noKeyTable("t", "text"), noKeyTable("u", "bigint")))
	wantConflict(t, pbMerge(t, view, touchT), `table "t"`)
	touchU := pbBranchWith(root, pbVB, "alter_u", pbTablesOnly(pbVB, noKeyTable("t", "bigint"), noKeyTable("u", "text")))
	wantMerge(t, pbMerge(t, view, touchU))
}

func TestForkViewWithUnknownSourcesConflictsWithAnyTableChange(t *testing.T) {
	root := rootWith(noKeyTable("t", "bigint"), noKeyTable("u", "bigint"))
	view := pbBranchWith(root, pbVA, "add_v", pbSnapshotWith(pbVA, pbV0, []string{noKeyTable("t", "bigint"), noKeyTable("u", "bigint")}, `{"name":"v","query":"SELECT 1"}`, ""))
	touchU := pbBranchWith(root, pbVB, "alter_u", pbTablesOnly(pbVB, noKeyTable("t", "bigint"), noKeyTable("u", "text")))
	wantConflict(t, pbMerge(t, view, touchU), "unknown")
}

func TestForkOpaqueBodiesConflictWithTableChanges(t *testing.T) {
	root := rootWith(noKeyTable("t", "bigint"))
	fn := pbBranchWith(root, pbVA, "add_f", pbSnapshotWith(pbVA, pbV0, []string{noKeyTable("t", "bigint")}, "",
		`{"name":"f","args":"","returns":"integer","language":"sql","volatility":"VOLATILE","body":"SELECT 1"}`))
	addTable := pbBranchWith(root, pbVB, "add_u", pbTablesOnly(pbVB, noKeyTable("t", "bigint"), noKeyTable("u", "bigint")))
	wantConflict(t, pbMerge(t, fn, addTable), "function, trigger or policy")
}
