-- 009_classification.sql（逆向，覆盖 009–011 的分类体系切换）
-- 恢复 categories / abilities 与提示词上的旧字段，回填最近似取值后删除新字典。
-- 逆向映射是多对一的：correctness 统一回到 数学能力，robustness 统一回到 逻辑推理，
-- 无法一一还原的细分在平台数据层合并，属于分类体系变更的既定损失。

CREATE TABLE categories (
    id           BIGSERIAL PRIMARY KEY,
    slug         TEXT        NOT NULL UNIQUE,
    name_zh      TEXT        NOT NULL,
    name_en      TEXT        NOT NULL,
    is_high_risk BOOLEAN     NOT NULL DEFAULT false,
    is_enabled   BOOLEAN     NOT NULL DEFAULT true,
    sort_order   INTEGER     NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX categories_high_risk_idx ON categories (is_high_risk) WHERE is_enabled;

CREATE TABLE abilities (
    id         BIGSERIAL PRIMARY KEY,
    slug       TEXT        NOT NULL UNIQUE,
    name_zh    TEXT        NOT NULL,
    name_en    TEXT        NOT NULL,
    is_enabled BOOLEAN     NOT NULL DEFAULT true,
    sort_order INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO categories (slug, name_zh, name_en, is_high_risk, sort_order) VALUES
    ('reasoning',              '逻辑推理',   'Logical Reasoning',         false, 1),
    ('math',                   '数学能力',   'Mathematical Ability',      false, 2),
    ('code_generation',        '代码生成',   'Code Generation',           false, 3),
    ('long_context',           '长文本理解', 'Long-Context Understanding', false, 4),
    ('instruction_following',  '指令遵循',   'Instruction Following',     false, 5),
    ('knowledge_boundary',     '知识边界',   'Knowledge Boundary',        false, 6),
    ('hallucination',          '幻觉检测',   'Hallucination Detection',   false, 7),
    ('safety',                 '安全性',     'Safety',                    false, 8),
    ('jailbreak',              '越狱对抗',   'Jailbreak & Adversarial',   true,  9),
    ('multi_turn',             '多轮对话',   'Multi-turn Dialogue',       false, 10),
    ('tool_use',               '工具调用',   'Tool Use',                  false, 11),
    ('chinese',                '中文理解',   'Chinese Understanding',     false, 12),
    ('multilingual',           '多语言能力', 'Multilingual Ability',      false, 13);

INSERT INTO abilities (slug, name_zh, name_en, sort_order) VALUES
    ('reasoning',             '推理能力', 'Reasoning',                 1),
    ('knowledge',             '知识掌握', 'Knowledge',                 2),
    ('coding',                '代码能力', 'Coding',                    3),
    ('language',              '语言理解', 'Language Understanding',    4),
    ('instruction_following', '指令遵循', 'Instruction Following',     5),
    ('safety_alignment',      '安全对齐', 'Safety & Alignment',        6),
    ('creativity',            '创造性',   'Creativity',                7),
    ('math_calculation',      '数学计算', 'Mathematical Calculation',  8);

ALTER TABLE prompts
    ADD COLUMN category_id BIGINT REFERENCES categories (id),
    ADD COLUMN ability_id  BIGINT REFERENCES abilities (id);

ALTER TABLE prompt_versions
    ADD COLUMN category_id BIGINT REFERENCES categories (id),
    ADD COLUMN ability_id  BIGINT REFERENCES abilities (id);

ALTER TABLE library_questions
    ADD COLUMN category_id BIGINT REFERENCES categories (id),
    ADD COLUMN ability_id  BIGINT REFERENCES abilities (id);

UPDATE prompts p
SET category_id = c.id
FROM purposes pu, categories c
WHERE p.purpose_id = pu.id
  AND c.slug = CASE pu.slug
      WHEN 'capability_boundary' THEN 'reasoning'
      WHEN 'correctness'         THEN 'math'
      WHEN 'robustness'          THEN 'reasoning'
      WHEN 'compliance'          THEN 'instruction_following'
      WHEN 'safety'              THEN 'safety'
      WHEN 'honesty'             THEN 'knowledge_boundary'
      WHEN 'adversarial'         THEN 'jailbreak'
  END;

UPDATE prompts p
SET ability_id = a.id
FROM capabilities cap, abilities a
WHERE p.capability_id = cap.id
  AND a.slug = CASE cap.slug
      WHEN 'reasoning'                THEN 'reasoning'
      WHEN 'knowledge'                THEN 'knowledge'
      WHEN 'math'                     THEN 'math_calculation'
      WHEN 'language'                 THEN 'language'
      WHEN 'creativity'               THEN 'creativity'
      WHEN 'coding'                   THEN 'coding'
      WHEN 'instruction_following'    THEN 'instruction_following'
      WHEN 'long_context'             THEN 'reasoning'
      WHEN 'tool_use'                 THEN 'coding'
      WHEN 'multi_turn'               THEN 'reasoning'
      WHEN 'safety'                   THEN 'safety_alignment'
      WHEN 'hallucination_resistance' THEN 'knowledge'
  END;

UPDATE prompt_versions v
SET category_id = c.id
FROM purposes pu, categories c
WHERE v.purpose_id = pu.id
  AND c.slug = CASE pu.slug
      WHEN 'capability_boundary' THEN 'reasoning'
      WHEN 'correctness'         THEN 'math'
      WHEN 'robustness'          THEN 'reasoning'
      WHEN 'compliance'          THEN 'instruction_following'
      WHEN 'safety'              THEN 'safety'
      WHEN 'honesty'             THEN 'knowledge_boundary'
      WHEN 'adversarial'         THEN 'jailbreak'
  END;

UPDATE prompt_versions v
SET ability_id = a.id
FROM capabilities cap, abilities a
WHERE v.capability_id = cap.id
  AND a.slug = CASE cap.slug
      WHEN 'reasoning'                THEN 'reasoning'
      WHEN 'knowledge'                THEN 'knowledge'
      WHEN 'math'                     THEN 'math_calculation'
      WHEN 'language'                 THEN 'language'
      WHEN 'creativity'               THEN 'creativity'
      WHEN 'coding'                   THEN 'coding'
      WHEN 'instruction_following'    THEN 'instruction_following'
      WHEN 'long_context'             THEN 'reasoning'
      WHEN 'tool_use'                 THEN 'coding'
      WHEN 'multi_turn'               THEN 'reasoning'
      WHEN 'safety'                   THEN 'safety_alignment'
      WHEN 'hallucination_resistance' THEN 'knowledge'
  END;

UPDATE library_questions q
SET category_id = c.id
FROM purposes pu, categories c
WHERE q.purpose_id = pu.id
  AND c.slug = CASE pu.slug
      WHEN 'capability_boundary' THEN 'reasoning'
      WHEN 'correctness'         THEN 'math'
      WHEN 'robustness'          THEN 'reasoning'
      WHEN 'compliance'          THEN 'instruction_following'
      WHEN 'safety'              THEN 'safety'
      WHEN 'honesty'             THEN 'knowledge_boundary'
      WHEN 'adversarial'         THEN 'jailbreak'
  END;

UPDATE library_questions q
SET ability_id = a.id
FROM capabilities cap, abilities a
WHERE q.capability_id = cap.id
  AND a.slug = CASE cap.slug
      WHEN 'reasoning'                THEN 'reasoning'
      WHEN 'knowledge'                THEN 'knowledge'
      WHEN 'math'                     THEN 'math_calculation'
      WHEN 'language'                 THEN 'language'
      WHEN 'creativity'               THEN 'creativity'
      WHEN 'coding'                   THEN 'coding'
      WHEN 'instruction_following'    THEN 'instruction_following'
      WHEN 'long_context'             THEN 'reasoning'
      WHEN 'tool_use'                 THEN 'coding'
      WHEN 'multi_turn'               THEN 'reasoning'
      WHEN 'safety'                   THEN 'safety_alignment'
      WHEN 'hallucination_resistance' THEN 'knowledge'
  END;

ALTER TABLE prompts
    ALTER COLUMN category_id SET NOT NULL;

ALTER TABLE prompt_versions
    ALTER COLUMN category_id SET NOT NULL;

CREATE INDEX prompts_category_idx ON prompts (category_id) WHERE deleted_at IS NULL;

ALTER TABLE models
    ADD COLUMN ability_tags TEXT[] NOT NULL DEFAULT '{}';

UPDATE models m
SET ability_tags = COALESCE((
    SELECT array_agg(DISTINCT a.slug ORDER BY a.slug)
    FROM model_capabilities mc
    JOIN capabilities cap ON cap.id = mc.capability_id
    JOIN abilities a ON a.slug = CASE cap.slug
        WHEN 'reasoning'                THEN 'reasoning'
        WHEN 'knowledge'                THEN 'knowledge'
        WHEN 'math'                     THEN 'math_calculation'
        WHEN 'language'                 THEN 'language'
        WHEN 'creativity'               THEN 'creativity'
        WHEN 'coding'                   THEN 'coding'
        WHEN 'instruction_following'    THEN 'instruction_following'
        WHEN 'long_context'             THEN 'reasoning'
        WHEN 'tool_use'                 THEN 'coding'
        WHEN 'multi_turn'               THEN 'reasoning'
        WHEN 'safety'                   THEN 'safety_alignment'
        WHEN 'hallucination_resistance' THEN 'knowledge'
    END
    WHERE mc.model_id = m.id
), '{}');

DROP INDEX IF EXISTS prompts_purpose_idx;
DROP INDEX IF EXISTS prompts_capability_idx;
DROP INDEX IF EXISTS library_questions_purpose_idx;
DROP INDEX IF EXISTS library_questions_capability_idx;

ALTER TABLE prompts
    DROP COLUMN purpose_id,
    DROP COLUMN capability_id;

ALTER TABLE prompt_versions
    DROP COLUMN purpose_id,
    DROP COLUMN capability_id;

ALTER TABLE library_questions
    DROP COLUMN purpose_id,
    DROP COLUMN capability_id;

ALTER TABLE tags
    DROP COLUMN kind;

DROP TABLE model_capabilities;
DROP TABLE purposes;
DROP TABLE capabilities;
