ALTER TABLE accounts ADD COLUMN user_id TEXT NOT NULL DEFAULT '';
UPDATE accounts SET user_id = '2b9af8b9-856a-4710-9ab6-58fe4eccdf24' WHERE user_id = '';
CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts(user_id);
