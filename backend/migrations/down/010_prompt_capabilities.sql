-- 回滚 012_prompt_capabilities.sql：删除关联表。prompts.capability_id
-- 与 prompt_versions.capability_id 未改动，单维度读取仍然可用。
DROP TABLE IF EXISTS prompt_version_capabilities;
DROP TABLE IF EXISTS prompt_capabilities;
