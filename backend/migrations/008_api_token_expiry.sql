-- 008_api_token_expiry.sql
-- API Token 增加有效期：签发 90 天后失效（需求 §16.3 的泄露暴露面收敛），
-- 既有令牌按同一期限回填，避免历史令牌永久有效。

ALTER TABLE api_tokens ADD COLUMN expires_at TIMESTAMPTZ;
UPDATE api_tokens SET expires_at = now() + interval '90 days' WHERE expires_at IS NULL;
ALTER TABLE api_tokens ALTER COLUMN expires_at SET NOT NULL;
ALTER TABLE api_tokens ALTER COLUMN expires_at SET DEFAULT now() + interval '90 days';
