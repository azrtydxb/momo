// SPDX-License-Identifier: Apache-2.0

// Command crud-zero-null shows insert, select, update and delete, and above
// all how Momo tells a zero value from NULL from "leave it alone".
//
// Run it against a database that has the migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/crud-zero-null
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/crud-zero-null/models"
	"github.com/azrtydxb/momo/examples/crud-zero-null/models/task"
)

func main() {
	if err := start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func start(ctx context.Context) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errors.New("DATABASE_URL is not set")
	}
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	return run(ctx, momo.New(pgxdriver.New(pool)), os.Stdout)
}

// run walks through every step and prints what it finds. The test compares
// the output, so the printed lines are the contract of this example.
func run(ctx context.Context, db *momo.DB, out io.Writer) error {
	steps := []func(context.Context, momo.Querier, io.Writer) error{
		insertZeroVersusDefault,
		updateToZeroAndNull,
		updateWholeModel,
		softDeleteAndDelete,
	}
	for _, step := range steps {
		if err := step(ctx, db, out); err != nil {
			return err
		}
	}
	return nil
}

// insertZeroVersusDefault shows the two ways to insert.
//
// Model writes every column of the struct, so a zero value is stored as
// zero. Values writes only the columns you name; the rest take the database
// default. Pick Model when the struct is the truth, Values when "unset" must
// mean "default".
func insertZeroVersusDefault(ctx context.Context, q momo.Querier, out io.Writer) error {
	zero := models.Task{Title: "zero"} // Priority 0, Done false, Note NULL
	if _, err := momo.Insert[models.Task](q).Model(&zero).Exec(ctx); err != nil {
		return err
	}
	if _, err := momo.Insert[models.Task](q).
		Values(task.Title.Set("defaults")).
		Exec(ctx); err != nil {
		return err
	}
	// ORDER BY makes the output deterministic; All never returns ErrNotFound.
	tasks, err := momo.Select[models.Task](q).OrderBy(task.ID.Asc()).All(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		fmt.Fprintf(out, "insert %-8s priority=%d done=%t note=%s\n", t.Title, t.Priority, t.Done, describe(t.Note))
	}
	return nil
}

// updateToZeroAndNull shows how a column goes to its zero value and to NULL.
//
// Set(v) writes exactly v, including 0, false and "". Only a NullColumn has
// SetNull, so NULL can never be written by accident to a NOT NULL column:
// the compiler rejects task.Priority.SetNull().
func updateToZeroAndNull(ctx context.Context, q momo.Querier, out io.Writer) error {
	one := func() (models.Task, error) {
		return momo.Select[models.Task](q).Where(task.Title.Eq("defaults")).One(ctx)
	}
	change := func(label string, assigns ...momo.Assignment) error {
		if _, err := momo.Update[models.Task](q).
			Set(assigns...).
			Where(task.Title.Eq("defaults")).
			Exec(ctx); err != nil {
			return err
		}
		t, err := one()
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "update %-12s priority=%d done=%t note=%s\n", label, t.Priority, t.Done, describe(t.Note))
		return nil
	}
	if err := change("non-zero", task.Priority.Set(5), task.Done.Set(true), task.Note.Set("call Ada")); err != nil {
		return err
	}
	if err := change("zero values", task.Priority.Set(0), task.Done.Set(false)); err != nil {
		return err
	}
	// The empty string is a value, not NULL.
	if err := change("empty string", task.Note.Set("")); err != nil {
		return err
	}
	if err := change("NULL", task.Note.SetNull()); err != nil {
		return err
	}
	// IsNull is how to find NULL; Eq(nil) is not expressible for a *string.
	n, err := momo.Select[models.Task](q).Where(task.Note.IsNull()).Count(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "tasks with NULL note: %d\n", n)
	return nil
}

// updateWholeModel updates a row from a struct. Model writes every writable
// column, zero values included, so it replaces the row: load the row first,
// change fields, then write it back. The version column makes this safe: a
// stale copy fails with ErrOptimisticLock instead of overwriting newer data.
func updateWholeModel(ctx context.Context, q momo.Querier, out io.Writer) error {
	mine, err := momo.Select[models.Task](q).Where(task.Title.Eq("zero")).One(ctx)
	if err != nil {
		return err
	}
	stale := mine // a second reader holding the old version

	mine.Done = true
	mine.Note = ptr("from the struct")
	if _, err := momo.Update[models.Task](q).Model(&mine).Exec(ctx); err != nil {
		return err
	}
	fmt.Fprintf(out, "model update ok, version %d -> %d\n", stale.Version, mine.Version)

	stale.Priority = 9
	_, err = momo.Update[models.Task](q).Model(&stale).Exec(ctx)
	fmt.Fprintf(out, "stale update: optimistic lock conflict=%t\n", errors.Is(err, momo.ErrOptimisticLock))
	return nil
}

// softDeleteAndDelete shows the two kinds of delete. The DeletedAt column is
// tagged soft_delete, so SoftDelete stamps it, every Select hides the row,
// and only WithDeleted or OnlyDeleted bring it back. A plain Delete on such a
// model is refused; ForceDelete removes the row for good.
func softDeleteAndDelete(ctx context.Context, q momo.Querier, out io.Writer) error {
	if _, err := momo.SoftDelete[models.Task](q).Where(task.Title.Eq("zero")).Exec(ctx); err != nil {
		return err
	}
	live, err := momo.Select[models.Task](q).Count(ctx)
	if err != nil {
		return err
	}
	all, err := momo.Select[models.Task](q).WithDeleted().Count(ctx)
	if err != nil {
		return err
	}
	gone, err := momo.Select[models.Task](q).OnlyDeleted().All(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "soft delete: live=%d with deleted=%d only deleted=%q\n", live, all, titles(gone))

	_, err = momo.Delete[models.Task](q).Where(task.Title.Eq("zero")).Exec(ctx)
	if !errors.Is(err, momo.ErrValidation) {
		return fmt.Errorf("plain Delete on a soft-delete model: want ErrValidation, got %v", err)
	}
	fmt.Fprintln(out, "plain Delete refused: ErrValidation")
	res, err := momo.ForceDelete[models.Task](q).Where(task.Title.Eq("zero")).Exec(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "force delete removed %d row\n", res.RowsAffected)
	return nil
}

func ptr[T any](v T) *T { return &v }

func describe(s *string) string {
	if s == nil {
		return "NULL"
	}
	return fmt.Sprintf("%q", *s)
}

func titles(ts []models.Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Title
	}
	return out
}
