-- 回滚 014_rating_output_unique.sql：移除活跃输出唯一索引。
DROP INDEX IF EXISTS rating_outputs_rating_uniq;
