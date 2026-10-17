INSERT OR IGNORE INTO accounts (id, user_id, name, provider, color, is_active, created_at, updated_at)
SELECT 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24', '2b9af8b9-856a-4710-9ab6-58fe4eccdf24', 'Local', 'local', '#7d6b96', 1, CAST(strftime('%s','now') AS INTEGER), CAST(strftime('%s','now') AS INTEGER)
WHERE EXISTS (SELECT 1 FROM calendars WHERE account_id NOT IN (SELECT id FROM accounts))
   OR EXISTS (SELECT 1 FROM events WHERE COALESCE(calendar_id, '') NOT IN (SELECT c.id FROM calendars c JOIN accounts a ON a.id = c.account_id));

INSERT OR IGNORE INTO calendars (id, account_id, name, href, color, is_visible, created_at, updated_at)
SELECT id, id, 'Local', '', '#7d6b96', 1, CAST(strftime('%s','now') AS INTEGER), CAST(strftime('%s','now') AS INTEGER)
FROM accounts WHERE id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24';

UPDATE calendars SET account_id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24'
WHERE account_id NOT IN (SELECT id FROM accounts)
  AND EXISTS (SELECT 1 FROM accounts WHERE id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24');

UPDATE events SET calendar_id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24', account_id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24'
WHERE COALESCE(calendar_id, '') NOT IN (SELECT c.id FROM calendars c JOIN accounts a ON a.id = c.account_id)
  AND EXISTS (SELECT 1 FROM accounts WHERE id = 'local-2b9af8b9-856a-4710-9ab6-58fe4eccdf24');
