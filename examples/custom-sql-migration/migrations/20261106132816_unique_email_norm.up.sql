-- momo:generated
-- momo:no-transaction
-- Edited by hand after `momo migrate diff` wrote it. diff emits a plain CREATE
-- UNIQUE INDEX, which blocks writes to the table while it builds. CONCURRENTLY
-- does not, but it cannot run inside a transaction block, hence the directive
-- above. The snapshot is unchanged because it already lists this index; only
-- the way the index is built differs. One statement per no-transaction file.

CREATE UNIQUE INDEX CONCURRENTLY "accounts_email_norm_key" ON "accounts" ("email_norm");
