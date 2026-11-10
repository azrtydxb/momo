-- +goose Up
-- +goose StatementBegin
CREATE TABLE b (x int);
INSERT INTO b VALUES (1);
-- +goose StatementEnd
