-- 006_verification_attempts.sql
-- 邮箱验证码的失败次数计数：同一验证码错误次数达到上限即作废，必须重新申请，
-- 堵住 6 位数字验证码在有效期内被分布式穷举的通道（防暴力猜测，配合按地址限流）。

ALTER TABLE email_verifications ADD COLUMN failed_attempts INTEGER NOT NULL DEFAULT 0;
