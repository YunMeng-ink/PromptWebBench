-- 009_classification_schema.sql
-- 提示词分类体系：考察维度（capabilities）与考察目的（purposes）。
-- 旧 categories / abilities 保留到 011 完成回填后删除，使回填语句能读取旧取值。

CREATE TABLE capabilities (
    id          BIGSERIAL PRIMARY KEY,
    slug        TEXT        NOT NULL UNIQUE,
    name_zh     TEXT        NOT NULL,
    name_en     TEXT        NOT NULL,
    group_key   TEXT        NOT NULL CHECK (group_key IN ('cognitive', 'execution', 'reliability')),
    description_zh TEXT     NOT NULL DEFAULT '',
    description_en TEXT     NOT NULL DEFAULT '',
    is_enabled  BOOLEAN     NOT NULL DEFAULT true,
    sort_order  INTEGER     NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX capabilities_group_idx ON capabilities (group_key, sort_order) WHERE is_enabled;

CREATE TABLE purposes (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT        NOT NULL UNIQUE,
    name_zh      TEXT        NOT NULL,
    name_en      TEXT        NOT NULL,
    description_zh TEXT      NOT NULL DEFAULT '',
    description_en TEXT      NOT NULL DEFAULT '',
    is_high_risk BOOLEAN     NOT NULL DEFAULT false,
    is_enabled   BOOLEAN     NOT NULL DEFAULT true,
    sort_order   INTEGER     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX purposes_high_risk_idx ON purposes (is_high_risk) WHERE is_enabled;

ALTER TABLE prompts
    ADD COLUMN purpose_id   BIGINT REFERENCES purposes (id),
    ADD COLUMN capability_id BIGINT REFERENCES capabilities (id);

ALTER TABLE prompt_versions
    ADD COLUMN purpose_id   BIGINT REFERENCES purposes (id),
    ADD COLUMN capability_id BIGINT REFERENCES capabilities (id);

ALTER TABLE library_questions
    ADD COLUMN purpose_id   BIGINT REFERENCES purposes (id),
    ADD COLUMN capability_id BIGINT REFERENCES capabilities (id);

-- 主题标签只承载领域与输出形态，不参与考察维度与考察目的的归类。
ALTER TABLE tags
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'theme' CHECK (kind IN ('theme', 'form'));

-- 模型适用能力改为引用考察维度字典，替代原有的自由文本数组。
CREATE TABLE model_capabilities (
    model_id      BIGINT NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    capability_id BIGINT NOT NULL REFERENCES capabilities (id) ON DELETE CASCADE,
    PRIMARY KEY (model_id, capability_id)
);

CREATE INDEX model_capabilities_capability_idx ON model_capabilities (capability_id);
