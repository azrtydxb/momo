// SPDX-License-Identifier: Apache-2.0

package momo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/dialect"
	"github.com/azrtydxb/momo/internal/fakedriver"
	"github.com/azrtydxb/momo/schema"
)

// The models below are hand-written metas with a generated identity key, the
// shape MySQL and MariaDB read back with LAST_INSERT_ID or INSERT ... RETURNING.

type mrParent struct {
	ID   int64
	Name string
	Kids []mrKid
}

func (*mrParent) MomoMeta() *momo.Meta[mrParent] {
	return &momo.Meta[mrParent]{
		Table: "mr_parents", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
		Ptr:     func(m *mrParent, i int) any { return [...]any{&m.ID, &m.Name}[i] },
		Value:   func(m *mrParent, i int) any { return [...]any{m.ID, m.Name}[i] },
	}
}

type mrKid struct {
	ID       int64
	ParentID int64
	Body     string
}

func (*mrKid) MomoMeta() *momo.Meta[mrKid] {
	return &momo.Meta[mrKid]{
		Table: "mr_kids", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "parent_id"}, {Name: "body"}},
		Ptr:     func(m *mrKid, i int) any { return [...]any{&m.ID, &m.ParentID, &m.Body}[i] },
		Value:   func(m *mrKid, i int) any { return [...]any{m.ID, m.ParentID, m.Body}[i] },
	}
}

var mrKids = momo.Relation[mrParent, mrKid]{
	Name: "Kids", Kind: momo.HasMany,
	Parent: momo.TableRef{Name: "mr_parents"}, Child: momo.TableRef{Name: "mr_kids"},
	ParentMeta: (*mrParent)(nil).MomoMeta(), ChildMeta: (*mrKid)(nil).MomoMeta(),
	ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
	ParentKey:   func(p *mrParent) ([]any, bool) { return []any{p.ID}, true },
	ChildKey:    func(c *mrKid) ([]any, bool) { return []any{c.ParentID}, true },
	SetChildKey: func(c *mrKid, k []any) { c.ParentID = k[0].(int64) },
	GetMany:     func(p *mrParent) []mrKid { return p.Kids },
	SetMany:     func(p *mrParent, cs []mrKid) { p.Kids = cs },
}

type mrTag struct {
	ID   int64
	Name string
}

func (*mrTag) MomoMeta() *momo.Meta[mrTag] {
	return &momo.Meta[mrTag]{
		Table: "mr_tags", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
		Ptr:     func(m *mrTag, i int) any { return [...]any{&m.ID, &m.Name}[i] },
		Value:   func(m *mrTag, i int) any { return [...]any{m.ID, m.Name}[i] },
	}
}

var mrTagName = momo.NewColumn[string]("mr_tags", "name", 0)

// mrUserT has an auto-increment primary key and UNIQUE(email); mrHandleT adds
// a second unique key, on name, that an insert can reach.
type mrUserT struct {
	ID    int64
	Email string
	Name  string
}

type mrHandleT struct {
	ID    int64
	Email string
	Name  string
}

var (
	mrUserEmail  = momo.NewColumn[string]("mr_users", "email", 0)
	mrUserID     = momo.NewColumn[int64]("mr_users", "id", momo.FlagPK)
	mrUserSchema = &schema.Table{Name: "mr_users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "mr_users_email", Columns: []string{"email"}}}}
	mrHandleSchema = &schema.Table{Name: "mr_users", PrimaryKey: []string{"id"},
		Uniques: []schema.Unique{{Name: "mr_users_email", Columns: []string{"email"}}, {Name: "mr_users_name", Columns: []string{"name"}}}}
	mrUserColumns = []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "email"}, {Name: "name"}}
)

func (*mrUserT) MomoMeta() *momo.Meta[mrUserT] {
	return &momo.Meta[mrUserT]{
		Table: "mr_users", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		TableSchema: mrUserSchema, Columns: mrUserColumns,
		Ptr:   func(m *mrUserT, i int) any { return [...]any{&m.ID, &m.Email, &m.Name}[i] },
		Value: func(m *mrUserT, i int) any { return [...]any{m.ID, m.Email, m.Name}[i] },
	}
}

func (*mrHandleT) MomoMeta() *momo.Meta[mrHandleT] {
	return &momo.Meta[mrHandleT]{
		Table: "mr_users", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		TableSchema: mrHandleSchema, Columns: mrUserColumns,
		Ptr:   func(m *mrHandleT, i int) any { return [...]any{&m.ID, &m.Email, &m.Name}[i] },
		Value: func(m *mrHandleT, i int) any { return [...]any{m.ID, m.Email, m.Name}[i] },
	}
}

// lastIDDriver is a MySQL-family fake. It answers the auto-increment probe,
// hands out generated keys from ExecInsertID (LAST_INSERT_ID: the first key of
// the statement, then step per row) and RETURNING rows for MariaDB, and can
// fail every INSERT with a unique violation.
type lastIDDriver struct {
	*fakedriver.Driver
	mode, step int64
	last       int64 // last key handed out
	dup        bool  // every INSERT fails with a unique violation
	found      [][]any
}

func newLastIDDriver(f dialect.Flavor, mode, step int64) *lastIDDriver {
	return &lastIDDriver{Driver: fakedriver.NewDialect(dialect.MySQLFlavor(f)), mode: mode, step: step}
}

func (d *lastIDDriver) answer(sql string, args []any) error {
	switch {
	case strings.HasPrefix(sql, "SELECT @@"):
		d.SetRows([][]any{{d.mode, d.step}})
	case strings.HasPrefix(sql, "INSERT INTO"):
		if d.dup {
			return &momo.Error{Kind: momo.ErrUniqueViolation, Op: "insert"}
		}
		if d.Dialect().InsertReturning { // MariaDB reads the keys from RETURNING rows; MySQL from ExecInsertID
			d.SetRows(d.returning(sql, args))
		}
	case isShareLock(sql):
		d.SetRows(d.found)
	default:
		d.SetRows(nil)
	}
	return nil
}

// returning builds the RETURNING rows of a MariaDB insert: the generated key
// first, then the bound columns (the key column itself is never bound).
func (d *lastIDDriver) returning(sql string, args []any) [][]any {
	width := map[string]int{"`mr_parents`": 1, "`mr_kids`": 2, "`mr_tags`": 1}
	for table, w := range width {
		if !strings.Contains(sql, "INSERT INTO "+table) {
			continue
		}
		var rows [][]any
		for i := 0; i+w <= len(args); i += w {
			d.last += d.step
			rows = append(rows, append([]any{d.last}, args[i:i+w]...))
		}
		return rows
	}
	return nil
}

func (d *lastIDDriver) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if err := d.answer(sql, args); err != nil {
		return nil, err
	}
	return d.Driver.Query(ctx, sql, args)
}

func (d *lastIDDriver) execInsertID(ctx context.Context, sql string, args []any) (int64, int64, error) {
	if _, err := d.Exec(ctx, sql, args); err != nil {
		return 0, 0, err
	}
	if err := d.answer(sql, args); err != nil {
		return 0, 0, err
	}
	rows := int64(strings.Count(sql, "), (") + 1)
	first := d.last + d.step
	d.last += rows * d.step
	return rows, first, nil
}

func (d *lastIDDriver) ExecInsertID(ctx context.Context, sql string, args []any) (int64, int64, error) {
	return d.execInsertID(ctx, sql, args)
}

func (d *lastIDDriver) Begin(ctx context.Context, o momo.TxOptions) (momo.DriverTx, error) {
	tx, err := d.Driver.Begin(ctx, o)
	return &lastIDTx{DriverTx: tx, d: d}, err
}

type lastIDTx struct {
	momo.DriverTx
	d *lastIDDriver
}

func (t *lastIDTx) Query(ctx context.Context, sql string, args []any) (momo.Rows, error) {
	if err := t.d.answer(sql, args); err != nil {
		return nil, err
	}
	return t.DriverTx.Query(ctx, sql, args)
}

func (t *lastIDTx) ExecInsertID(ctx context.Context, sql string, args []any) (int64, int64, error) {
	return t.d.execInsertID(ctx, sql, args)
}

// isShareLock reports whether sql is a shared-lock read: FOR SHARE on MySQL,
// LOCK IN SHARE MODE on MariaDB.
func isShareLock(sql string) bool {
	return strings.Contains(sql, " FOR SHARE") || strings.Contains(sql, " LOCK IN SHARE MODE")
}

func countCalls(d *fakedriver.Driver, substr string) int {
	n := 0
	for _, c := range d.Calls() {
		if strings.Contains(c.SQL, substr) {
			n++
		}
	}
	return n
}

func newMRParent(kids int) *mrParent {
	p := &mrParent{Name: "p"}
	for range kids {
		p.Kids = append(p.Kids, mrKid{Body: "b"})
	}
	return p
}

func requireKids(t *testing.T, p *mrParent, first, step int64) {
	t.Helper()
	for i, k := range p.Kids {
		if want := first + int64(i)*step; k.ID != want || k.ParentID != p.ID {
			t.Fatalf("kid %d: id=%d parent=%d, want id=%d parent=%d", i, k.ID, k.ParentID, want, p.ID)
		}
	}
}

// Catches: CreateGraph on MariaDB refusing with ErrUnsupported (or probing
// auto-increment settings) because it checks Dialect().Returning instead of
// InsertReturning.
func TestCreateGraphMariaDBUsesInsertReturning(t *testing.T) {
	d := newLastIDDriver(dialect.FlavorMariaDB, 2, 1)
	db := momo.New(d)
	p := newMRParent(3)
	if err := momo.CreateGraph(context.Background(), db, p, mrKids.Include()); err != nil {
		t.Fatal(err)
	}
	if p.ID != 1 {
		t.Fatalf("parent id = %d, want the RETURNING key 1", p.ID)
	}
	requireKids(t, p, 2, 1)
	if got := countCalls(d.Driver, "RETURNING"); got != 2 {
		t.Fatalf("%d statements with RETURNING, want 2 (parent, then the children in one bulk insert)", got)
	}
	if countCalls(d.Driver, "@@") != 0 {
		t.Fatal("MariaDB read the auto-increment settings; it has RETURNING and needs none")
	}
}

// Catches: MySQL CreateGraph guessing consecutive keys under innodb_autoinc_lock_mode
// 2, where the keys of one multi-row INSERT are not guaranteed consecutive.
func TestCreateGraphMySQLInterleavedInsertsOneRowPerStatement(t *testing.T) {
	d := newLastIDDriver(dialect.FlavorMySQL, 2, 1)
	db := momo.New(d)
	p := newMRParent(3)
	if err := momo.CreateGraph(context.Background(), db, p, mrKids.Include()); err != nil {
		t.Fatal(err)
	}
	if got := countCalls(d.Driver, "INSERT INTO `mr_kids`"); got != 3 {
		t.Fatalf("%d child INSERT statements under lock mode 2, want one per child (3)", got)
	}
	if countCalls(d.Driver, "), (") != 0 || countCalls(d.Driver, "@@") != 0 {
		t.Fatal("a multi-row INSERT or an auto-increment probe ran")
	}
	if p.ID != 1 {
		t.Fatalf("parent id = %d, want LAST_INSERT_ID 1", p.ID)
	}
	requireKids(t, p, 2, 1)
	if d.Commits() != 1 || d.Rollbacks() != 0 {
		t.Fatalf("commits=%d rollbacks=%d, want the graph in one transaction", d.Commits(), d.Rollbacks())
	}
}

// Catches: MySQL CreateGraph inferring keys from a multi-row statement (first
// key plus increment) or probing the server's auto-increment settings, whatever
// the lock mode: it must insert one row per statement and read each key.
func TestCreateGraphMySQLNeverInfersKeys(t *testing.T) {
	for _, mode := range []int64{0, 1, 2} {
		d := newLastIDDriver(dialect.FlavorMySQL, mode, 3)
		db := momo.New(d)
		p := newMRParent(4)
		if err := momo.CreateGraph(context.Background(), db, p, mrKids.Include()); err != nil {
			t.Fatalf("mode %d: %v", mode, err)
		}
		if got := countCalls(d.Driver, "INSERT INTO `mr_kids`"); got != 4 {
			t.Fatalf("mode %d: %d child INSERT statements, want one per child (4)", mode, got)
		}
		if countCalls(d.Driver, "@@") != 0 || countCalls(d.Driver, "), (") != 0 {
			t.Fatalf("mode %d: the graph probed auto-increment settings or sent a multi-row INSERT", mode)
		}
		if p.ID != 3 {
			t.Fatalf("mode %d: parent id = %d, want 3", mode, p.ID)
		}
		requireKids(t, p, 6, 3)
	}
}

// Catches: MySQL CreateGraph accepting children that mix explicit and zero
// keys, whose generated keys cannot be read back.
func TestCreateGraphMySQLMixedKeysIsValidationError(t *testing.T) {
	d := newLastIDDriver(dialect.FlavorMySQL, 1, 1)
	db := momo.New(d)
	p := newMRParent(2)
	p.Kids[0].ID = 50
	err := momo.CreateGraph(context.Background(), db, p, mrKids.Include())
	if !errors.Is(err, momo.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
	if countCalls(d.Driver, "INSERT INTO `mr_kids`") != 0 || d.Rollbacks() == 0 {
		t.Fatalf("kid inserts=%d rollbacks=%d, want none and a rolled back graph", countCalls(d.Driver, "INSERT INTO `mr_kids`"), d.Rollbacks())
	}
}

// Catches: FindOrCreate on MySQL or MariaDB reporting created for a row that
// already existed, or failing instead of re-selecting it, or not writing the
// stored row into the model.
func TestFindOrCreateMySQLFamily(t *testing.T) {
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		key := []momo.ColumnRef{mrTagName}
		ctx := context.Background()

		d := newLastIDDriver(f, 2, 1)
		m := &mrTag{Name: "go"}
		created, err := momo.FindOrCreate[mrTag, *mrTag](ctx, momo.New(d), key, m)
		if err != nil || !created || m.ID != 1 {
			t.Fatalf("%s created: created=%v id=%d err=%v, want true, 1, nil", f, created, m.ID, err)
		}
		if countCalls(d.Driver, "SELECT") != 0 {
			t.Fatalf("%s: a created row must not be selected", f)
		}
		if (f == dialect.FlavorMariaDB) != (countCalls(d.Driver, "RETURNING") == 1) {
			t.Fatalf("%s: RETURNING calls = %d; MariaDB reads the key with RETURNING, MySQL with LAST_INSERT_ID", f, countCalls(d.Driver, "RETURNING"))
		}

		d = newLastIDDriver(f, 2, 1)
		d.dup = true
		d.found = [][]any{{int64(9), "go"}}
		m = &mrTag{Name: "go"}
		created, err = momo.FindOrCreate[mrTag, *mrTag](ctx, momo.New(d), key, m)
		if err != nil || created || m.ID != 9 {
			t.Fatalf("%s found: created=%v id=%d err=%v, want false, 9, nil", f, created, m.ID, err)
		}
		shared := 0
		for _, c := range d.Calls() {
			if isShareLock(c.SQL) {
				shared++
			}
		}
		if shared != 1 || countCalls(d.Driver, "FOR UPDATE") != 0 || d.Commits() != 1 {
			t.Fatalf("%s found: want one shared-lock select (never FOR UPDATE, which deadlocks losers of the same race) and a committed scope; got shared=%d commits=%d", f, shared, d.Commits())
		}

		// The violation was on another unique key: nothing matches the key.
		d = newLastIDDriver(f, 2, 1)
		d.dup = true
		_, err = momo.FindOrCreate[mrTag, *mrTag](ctx, momo.New(d), key, &mrTag{Name: "go"})
		if !errors.Is(err, momo.ErrUniqueViolation) || d.Rollbacks() != 1 {
			t.Fatalf("%s other key: err=%v rollbacks=%d, want ErrUniqueViolation and a rollback", f, err, d.Rollbacks())
		}
	}
}

// Catches: an upsert on UNIQUE(email) next to an auto-increment primary key
// being refused on MySQL and MariaDB although the insert omits the key, and
// the key being written when it is left out.
func TestUpsertSecondaryTargetAllowedWhenIdentityOmitted(t *testing.T) {
	for _, f := range []dialect.Flavor{dialect.FlavorMySQL, dialect.FlavorMariaDB} {
		db := momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(f)))
		u := mrUserT{Email: "a@example.com", Name: "a"}
		c, err := momo.Upsert[mrUserT](db).Model(&u).OnConflict(mrUserEmail).DoUpdate().SQL()
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if !strings.Contains(c.SQL, "ON DUPLICATE KEY UPDATE") || strings.Contains(c.SQL, "(`id`") {
			t.Fatalf("%s: SQL = %s, want ON DUPLICATE KEY UPDATE and no id column", f, c.SQL)
		}
	}
}

// Catches: the relaxed upsert target rule accepting a statement that can
// conflict on a key the caller did not name: the primary key as target next to
// UNIQUE(email), or a second unique key made of ordinary columns. Both stay ErrUnsupported with the
// reason in the message.
func TestUpsertSecondaryTargetRefusedWhenOtherKeyReachable(t *testing.T) {
	db := momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(dialect.FlavorMySQL)))

	// The primary key is the target and the insert writes it: UNIQUE(email) is reachable.
	explicit := mrUserT{ID: 7, Email: "a@example.com", Name: "a"}
	_, err := momo.Upsert[mrUserT](db).Model(&explicit).OnConflict(mrUserID).DoUpdate().SQL()
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "[email]") {
		t.Fatalf("primary key target: err = %v, want ErrUnsupported naming [email]", err)
	}

	two := mrHandleT{Email: "a@example.com", Name: "a"}
	_, err = momo.Upsert[mrHandleT](db).Model(&two).OnConflict(mrUserEmail).DoUpdate().SQL()
	if !errors.Is(err, momo.ErrUnsupported) || !strings.Contains(err.Error(), "[name]") {
		t.Fatalf("second ordinary unique key: err = %v, want ErrUnsupported naming [name]", err)
	}
}

// mrStamp is a graph child with an automatic created_at.
type mrStamp struct {
	ID        int64
	ParentID  int64
	CreatedAt time.Time
}

func (*mrStamp) MomoMeta() *momo.Meta[mrStamp] {
	return &momo.Meta[mrStamp]{
		Table: "mr_stamps", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: 2, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "parent_id"}, {Name: "created_at", Flags: momo.FlagCreatedAt}},
		Ptr:     func(m *mrStamp, i int) any { return [...]any{&m.ID, &m.ParentID, &m.CreatedAt}[i] },
		Value:   func(m *mrStamp, i int) any { return [...]any{m.ID, m.ParentID, m.CreatedAt}[i] },
	}
}

// Catches: the one-row-per-statement MySQL path reading the clock per
// statement, so the rows of one has-many get different created_at values
// where a bulk insert gives them all the same one.
func TestCreateGraphMySQLRowsShareOneClock(t *testing.T) {
	var stamps []mrStamp
	rel := momo.Relation[mrParent, mrStamp]{
		Name: "Stamps", Kind: momo.HasMany,
		Parent: momo.TableRef{Name: "mr_parents"}, Child: momo.TableRef{Name: "mr_stamps"},
		ParentMeta: (*mrParent)(nil).MomoMeta(), ChildMeta: (*mrStamp)(nil).MomoMeta(),
		ParentCols: []string{"id"}, ChildCols: []string{"parent_id"}, ChildPK: []string{"id"},
		ParentKey:   func(p *mrParent) ([]any, bool) { return []any{p.ID}, true },
		ChildKey:    func(c *mrStamp) ([]any, bool) { return []any{c.ParentID}, true },
		SetChildKey: func(c *mrStamp, k []any) { c.ParentID = k[0].(int64) },
		GetMany:     func(*mrParent) []mrStamp { return stamps },
		SetMany:     func(_ *mrParent, cs []mrStamp) { stamps = cs },
	}
	stamps = make([]mrStamp, 4)
	d := newLastIDDriver(dialect.FlavorMySQL, 2, 1)
	if err := momo.CreateGraph(context.Background(), momo.New(d), &mrParent{Name: "p"}, rel.Include()); err != nil {
		t.Fatal(err)
	}
	var seen []time.Time
	for _, c := range d.Calls() {
		if !strings.Contains(c.SQL, "INSERT INTO `mr_stamps`") {
			continue
		}
		for _, a := range c.Args {
			if ts, ok := a.(time.Time); ok {
				seen = append(seen, ts)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("%d created_at values bound, want 4", len(seen))
	}
	for i, ts := range seen {
		if !ts.Equal(seen[0]) {
			t.Fatalf("row %d created_at %v differs from row 0 %v", i, ts, seen[0])
		}
	}
}

// mrStrTag has a generated identity key on a string field, which LAST_INSERT_ID
// cannot fill.
type mrStrTag struct {
	ID   string
	Name string
}

func (*mrStrTag) MomoMeta() *momo.Meta[mrStrTag] {
	return &momo.Meta[mrStrTag]{
		Table: "mr_str_tags", PK: []int{0}, Identity: 0, Version: -1, SoftDelete: -1, CreatedAt: -1, UpdatedAt: -1,
		Columns: []momo.ColumnMeta{{Name: "id", Flags: momo.FlagPK | momo.FlagIdentity}, {Name: "name"}},
		Ptr:     func(m *mrStrTag, i int) any { return [...]any{&m.ID, &m.Name}[i] },
		Value:   func(m *mrStrTag, i int) any { return [...]any{m.ID, m.Name}[i] },
	}
}

// Catches: MySQL silently leaving a generated key unread (a zero foreign key
// for the children) when the identity field is not an integer.
func TestCreateGraphMySQLNonIntegerIdentityIsUnsupported(t *testing.T) {
	d := newLastIDDriver(dialect.FlavorMySQL, 2, 1)
	err := momo.CreateGraph[mrStrTag, *mrStrTag](context.Background(), momo.New(d), &mrStrTag{Name: "x"})
	if !errors.Is(err, momo.ErrUnsupported) {
		t.Fatalf("err = %v, want ErrUnsupported", err)
	}
	if countCalls(d.Driver, "INSERT") != 0 {
		t.Fatal("the insert ran although its key could not be read back")
	}
}

var (
	mrUserName = momo.NewColumn[string]("mr_users", "name", 0)
)

// Catches: the secondary-target rule judging "the insert leaves the
// auto-increment key out" from builder state instead of the columns the
// statement writes: a Values or FromSelect statement that lists no identity
// column leaves the key to the database, so the target is accepted.
func TestUpsertSecondaryTargetUsesWrittenColumns(t *testing.T) {
	db := momo.New(fakedriver.NewDialect(dialect.MySQLFlavor(dialect.FlavorMySQL)))
	// Values refuses an identity column outright (ErrValidation), so the written
	// list can only gain the key through the model path (an explicit identity).
	c, err := momo.Upsert[mrUserT](db).Values(mrUserEmail.Set("a@example.com"), mrUserName.Set("a")).
		OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
	if err != nil || !strings.Contains(c.SQL, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("id left out of Values: %v, %q", err, c.SQL)
	}
	// FromSelect never writes the identity column, so its target list leaves
	// the key to the database.
	src := momo.SelectExprs(momo.Value("a@example.com"), momo.Value("a"))
	c, err = momo.Upsert[mrUserT](db).FromSelect(src, mrUserEmail, mrUserName).
		OnConflict(mrUserEmail).DoUpdate(mrUserName.SetExcluded()).SQL()
	if err != nil || !strings.Contains(c.SQL, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("FromSelect without the id: %v, %q", err, c.SQL)
	}
}
