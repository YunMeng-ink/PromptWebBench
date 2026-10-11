-- 001_extensions.sql
-- 关键词检索依赖 pg_trgm。该扩展自 PostgreSQL 13 起为受信扩展，库所有者可直接创建；
-- 若运行账号不是库所有者，需要提前由超级用户创建，否则启动会在本迁移失败。
CREATE EXTENSION IF NOT EXISTS pg_trgm;
