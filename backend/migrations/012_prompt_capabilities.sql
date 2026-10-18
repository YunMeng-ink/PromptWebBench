-- 012_prompt_capabilities.sql
-- 提示词支持一至五个考察维度。prompts.capability_id 与 prompt_versions.capability_id
-- 继续保存首个维度，供既有列表、筛选与统计读取；完整集合存入两张关联表。
-- 角色分工：
--   prompt_capabilities          当前版本的维度集合，编辑时整体替换；
--   prompt_version_capabilities  每个历史版本发布时的维度集合，只增不改。

CREATE TABLE prompt_capabilities (
    prompt_id     BIGINT   NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    capability_id BIGINT   NOT NULL REFERENCES capabilities (id) ON DELETE RESTRICT,
    sort_order    SMALLINT NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 4),
    PRIMARY KEY (prompt_id, capability_id)
);

CREATE INDEX prompt_capabilities_capability_idx ON prompt_capabilities (capability_id);

CREATE TABLE prompt_version_capabilities (
    prompt_version_id BIGINT   NOT NULL REFERENCES prompt_versions (id) ON DELETE CASCADE,
    capability_id     BIGINT   NOT NULL REFERENCES capabilities (id) ON DELETE RESTRICT,
    sort_order        SMALLINT NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 4),
    PRIMARY KEY (prompt_version_id, capability_id)
);

CREATE INDEX prompt_version_capabilities_capability_idx
    ON prompt_version_capabilities (capability_id);

-- 回填已有数据：每条提示词与每个历史版本都只有一个考察维度。
INSERT INTO prompt_capabilities (prompt_id, capability_id, sort_order)
SELECT id, capability_id, 0
FROM prompts
ON CONFLICT DO NOTHING;

INSERT INTO prompt_version_capabilities (prompt_version_id, capability_id, sort_order)
SELECT id, capability_id, 0
FROM prompt_versions
ON CONFLICT DO NOTHING;
