-- 011_classification_enforce.sql
-- 分类轴切换的收尾：提示词与版本必须同时具备考察目的与考察维度，
-- 随后移除旧字典、旧字段与模型能力自由文本。

ALTER TABLE prompts
    ALTER COLUMN purpose_id SET NOT NULL,
    ALTER COLUMN capability_id SET NOT NULL;

ALTER TABLE prompt_versions
    ALTER COLUMN purpose_id SET NOT NULL,
    ALTER COLUMN capability_id SET NOT NULL;

CREATE INDEX prompts_purpose_idx    ON prompts (purpose_id)    WHERE deleted_at IS NULL;
CREATE INDEX prompts_capability_idx ON prompts (capability_id) WHERE deleted_at IS NULL;
CREATE INDEX library_questions_purpose_idx    ON library_questions (purpose_id)    WHERE deleted_at IS NULL;
CREATE INDEX library_questions_capability_idx ON library_questions (capability_id) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS prompts_category_idx;

ALTER TABLE prompts
    DROP COLUMN category_id,
    DROP COLUMN ability_id;

ALTER TABLE prompt_versions
    DROP COLUMN category_id,
    DROP COLUMN ability_id;

ALTER TABLE library_questions
    DROP COLUMN category_id,
    DROP COLUMN ability_id;

ALTER TABLE models
    DROP COLUMN ability_tags;

DROP TABLE categories;
DROP TABLE abilities;
