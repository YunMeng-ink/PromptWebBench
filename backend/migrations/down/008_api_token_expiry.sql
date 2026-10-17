-- 008_api_token_expiry.sql（逆向）
ALTER TABLE api_tokens ALTER COLUMN expires_at DROP DEFAULT;
ALTER TABLE api_tokens DROP COLUMN expires_at;
