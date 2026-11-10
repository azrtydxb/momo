-- +goose Up
CREATE TABLE a (x int);

-- +goose Down
DROP TABLE a;
