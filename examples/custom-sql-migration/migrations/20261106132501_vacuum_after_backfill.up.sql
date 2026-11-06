-- momo:no-transaction
-- VACUUM cannot run inside a transaction block. This directive, in the leading
-- comment block, makes Momo run the file's statements outside one. Keep such a
-- file to one statement: if a later statement failed, the earlier ones would
-- stay applied and the migration would be marked failed (dirty).
VACUUM (ANALYZE) accounts;
