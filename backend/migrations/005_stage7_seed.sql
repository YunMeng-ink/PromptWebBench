-- 005_stage7_seed.sql
-- 统计缓存存活时长（技术可行性报告 §3.1：进程内缓存，TTL 60 s，容量上限 10000 条）。
-- 容量上限是固定指标，直接写在代码里；存活时长可配置，取值单位为秒。

INSERT INTO config (key, value, description) VALUES
    ('stats_cache_ttl_seconds', '60'::jsonb, '统计缓存存活时长（秒）')
ON CONFLICT (key) DO NOTHING;