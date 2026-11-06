// SPDX-License-Identifier: Apache-2.0

// Command quickstart is the compiled source of the snippets in the root
// README. TestReadmeSnippetsAreCompiled keeps the two in step, so edit the
// code here and the README together.
//
// Run it against a database that has the migrations applied:
//
//	DATABASE_URL=postgres://... go run ./examples/quickstart
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/azrtydxb/momo"
	"github.com/azrtydxb/momo/driver/pgxdriver"
	"github.com/azrtydxb/momo/examples/quickstart/models"
	"github.com/azrtydxb/momo/examples/quickstart/models/author"
	"github.com/azrtydxb/momo/examples/quickstart/models/post"
	"github.com/azrtydxb/momo/vector"
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
	db, closeDB, err := openDB(ctx, url)
	if err != nil {
		return err
	}
	defer closeDB()
	return run(ctx, db)
}

func openDB(ctx context.Context, url string) (*momo.DB, func(), error) {
	pool, err := pgxdriver.NewPool(ctx, url)
	if err != nil {
		return nil, nil, err
	}
	return momo.New(pgxdriver.New(pool)), pool.Close, nil
}

func run(ctx context.Context, db *momo.DB) error {
	ada, err := createAuthor(ctx, db, "Ada")
	if err != nil {
		return err
	}
	if err := createPosts(ctx, db, ada.ID); err != nil {
		return err
	}
	if err := readAndChange(ctx, db, ada.ID); err != nil {
		return err
	}
	if err := loadRelations(ctx, db, ada.ID); err != nil {
		return err
	}
	if err := similarPosts(ctx, db, vector.Vector{1, 0, 0}); err != nil {
		return err
	}
	return renameInTx(ctx, db, ada.ID, "Ada Lovelace")
}

func createAuthor(ctx context.Context, q momo.Querier, name string) (models.Author, error) {
	a := models.Author{Name: name}
	rows, err := momo.Insert[models.Author](q).Model(&a).Returning(author.ID).Rows(ctx)
	if err != nil {
		return a, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := rows.ScanModel(&a); err != nil {
			return a, err
		}
	}
	return a, rows.Err()
}

func createPosts(ctx context.Context, q momo.Querier, authorID int64) error {
	_, err := momo.Insert[models.Post](q).Models([]models.Post{
		{AuthorID: authorID, Title: "Notes on the engine", Embedding: vector.Vector{1, 0, 0}},
		{AuthorID: authorID, Title: "Poetical science", Embedding: vector.Vector{0, 1, 0}},
		{AuthorID: authorID, Title: "Bernoulli numbers", Embedding: vector.Vector{0.9, 0.1, 0}},
	}).Exec(ctx)
	return err
}

func readAndChange(ctx context.Context, q momo.Querier, authorID int64) error {
	posts, err := momo.Select[models.Post](q).
		Where(post.AuthorID.Eq(authorID)).
		OrderBy(post.Title.Asc()).
		All(ctx)
	if err != nil {
		return err
	}
	fmt.Println("posts:", len(posts))

	first, err := momo.Select[models.Post](q).Where(post.Title.Eq("Poetical science")).One(ctx)
	if err != nil {
		return err
	}

	if _, err := momo.Update[models.Post](q).
		Set(post.Title.Set("Poetical science, revised")).
		Where(post.ID.Eq(first.ID)).
		Exec(ctx); err != nil {
		return err
	}
	_, err = momo.Delete[models.Post](q).Where(post.ID.Eq(first.ID)).Exec(ctx)
	return err
}

func loadRelations(ctx context.Context, q momo.Querier, authorID int64) error {
	authors, err := momo.Select[models.Author](q).Where(author.ID.Eq(authorID)).All(ctx)
	if err != nil {
		return err
	}
	if err := momo.Load(ctx, q, authors, author.Posts); err != nil {
		return err
	}
	posts := authors[0].Posts
	if err := momo.Load(ctx, q, posts, post.Author); err != nil {
		return err
	}
	for _, p := range posts {
		fmt.Println(p.Title, "by", p.Author.Name)
	}
	return nil
}

func similarPosts(ctx context.Context, db *momo.DB, query vector.Vector) error {
	settings := momo.SearchSettings{HNSWEfSearch: 100}
	return db.WithSearch(ctx, settings, func(tx *momo.Tx) error {
		nearest, err := momo.Select[models.Post](tx).
			OrderBy(momo.CosineDistance(post.Embedding, query).Asc()).
			Limit(5).
			All(ctx)
		if err != nil {
			return err
		}
		for _, p := range nearest {
			fmt.Println("near:", p.Title)
		}
		return nil
	})
}

func renameInTx(ctx context.Context, db *momo.DB, id int64, name string) error {
	return db.Tx(ctx, momo.TxOptions{}, func(tx *momo.Tx) error {
		if _, err := momo.Update[models.Author](tx).
			Set(author.Name.Set(name)).
			Where(author.ID.Eq(id)).
			Exec(ctx); err != nil {
			return err
		}
		n, err := momo.Select[models.Author](tx).Where(author.Name.Eq(name)).Count(ctx)
		if err != nil {
			return err
		}
		if n != 1 {
			return errors.New("rename did not take effect") // rolls the transaction back
		}
		return nil
	})
}
