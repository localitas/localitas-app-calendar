ALTER TABLE accounts ADD COLUMN oauth_client_id TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN oauth_client_secret TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN access_token TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN refresh_token TEXT DEFAULT '';
ALTER TABLE accounts ADD COLUMN token_expiry INTEGER DEFAULT 0;
