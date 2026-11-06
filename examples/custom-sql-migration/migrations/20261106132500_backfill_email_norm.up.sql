-- Custom migration: written by hand, so there is no "-- momo:generated" line.
-- It changes data, not the schema, so its snapshot file is a copy of the
-- previous one. Momo runs every statement of the file in one transaction.

-- Rows that existed before email_norm was added hold the default ''.
UPDATE accounts SET email_norm = lower(email) WHERE email_norm = '';

-- A dollar-quoted body may hold semicolons. The splitter keeps it in one
-- statement, so it is sent to the server whole.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM accounts GROUP BY email_norm HAVING count(*) > 1) THEN
    RAISE EXCEPTION 'two accounts differ only in the case of their email';
  END IF;
END
$$;
