-- The balance check below looks entries up by transaction. Postgres does not
-- index foreign-key columns on its own, so without this index every posting
-- would scan the whole entries table.
CREATE INDEX entries_transaction_idx ON entries (transaction_id);

CREATE FUNCTION check_entries_balanced() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    txn UUID;
    debits NUMERIC;
    credits NUMERIC;
BEGIN
    -- Check every transaction this row touches: the old one (UPDATE/DELETE)
    -- and the new one (INSERT/UPDATE); they differ only if an entry is moved.
    FOR txn IN
        SELECT DISTINCT t FROM unnest(
            CASE TG_OP
                WHEN 'INSERT' THEN ARRAY[NEW.transaction_id]
                WHEN 'DELETE' THEN ARRAY[OLD.transaction_id]
                ELSE ARRAY[OLD.transaction_id, NEW.transaction_id]
            END
        ) AS t
    LOOP
        SELECT COALESCE(SUM(amount) FILTER (WHERE direction = 'DEBIT'), 0),
               COALESCE(SUM(amount) FILTER (WHERE direction = 'CREDIT'), 0)
          INTO debits, credits
          FROM entries
         WHERE transaction_id = txn;

        IF debits <> credits THEN
            RAISE EXCEPTION 'transaction % is unbalanced: debits % <> credits %', txn, debits, credits
                USING ERRCODE = 'check_violation', CONSTRAINT = 'entries_balanced', TABLE = 'entries';
        END IF;
    END LOOP;
    RETURN NULL;
END;
$$;

-- DEFERRABLE INITIALLY DEFERRED: entries of one transaction are inserted one
-- by one, so balance is checked once, at COMMIT.
CREATE CONSTRAINT TRIGGER entries_balanced
    AFTER INSERT OR UPDATE OR DELETE ON entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION check_entries_balanced();