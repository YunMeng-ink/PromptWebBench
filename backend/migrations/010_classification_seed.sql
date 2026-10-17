-- 010_classification_seed.sql
-- 考察维度与考察目的种子数据，以及旧 categories / abilities 到新分类轴的确定性回填。
-- 本文件只含数据，不进入 db/schema.sql。

INSERT INTO capabilities (slug, name_zh, name_en, group_key, description_zh, description_en, sort_order) VALUES
    ('reasoning', '推理', 'Reasoning', 'cognitive',
     '多步演绎、因果分析、规划与约束求解，答案依赖推理链',
     'Multi-step deduction, causal analysis, planning, and constraint solving.', 1),
    ('knowledge', '知识', 'Knowledge', 'cognitive',
     '事实回忆与领域知识应用',
     'Recall and application of factual and domain knowledge.', 2),
    ('math', '数学', 'Mathematics', 'cognitive',
     '数值、符号、几何与数学应用题',
     'Numerical, symbolic, geometric, and mathematical word problems.', 3),
    ('language', '语言', 'Language', 'cognitive',
     '阅读理解、写作、摘要、翻译与多语言使用',
     'Reading comprehension, writing, summarization, translation, and multilingual use.', 4),
    ('creativity', '创造性', 'Creativity', 'cognitive',
     '开放生成、发散构思与风格创新',
     'Open-ended generation, divergent ideation, and stylistic invention.', 5),
    ('coding', '代码', 'Coding', 'execution',
     '代码生成、理解、调试、重构与测试设计',
     'Code generation, comprehension, debugging, refactoring, and test design.', 6),
    ('instruction_following', '指令遵循', 'Instruction Following', 'execution',
     '格式、长度、顺序、禁用项与多重约束的遵守',
     'Adherence to format, length, ordering, exclusions, and combined constraints.', 7),
    ('long_context', '长文本', 'Long Context', 'execution',
     '长输入中的检索、聚合与一致性维护',
     'Retrieval, aggregation, and consistency across long inputs.', 8),
    ('tool_use', '工具调用', 'Tool Use', 'execution',
     '工具选择、参数构造、调用顺序与错误处理',
     'Tool selection, argument construction, call sequencing, and error handling.', 9),
    ('multi_turn', '多轮交互', 'Multi-turn Interaction', 'execution',
     '跨轮状态跟踪、指代消解、澄清与修正',
     'State tracking, reference resolution, clarification, and correction across turns.', 10),
    ('safety', '安全能力', 'Safety', 'reliability',
     '拒绝有害请求、政策遵守与敏感话题处理',
     'Refusal of harmful requests, policy compliance, and sensitive-topic handling.', 11),
    ('hallucination_resistance', '幻觉抑制', 'Hallucination Resistance', 'reliability',
     '事实依据、不确定性表达与证据不足时拒答',
     'Factual grounding, uncertainty expression, and abstention when evidence is insufficient.', 12)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO purposes (slug, name_zh, name_en, description_zh, description_en, is_high_risk, sort_order) VALUES
    ('capability_boundary', '边界探测', 'Capability Boundary',
     '模型是否具备该能力，边界在哪里',
     'Does the model have the capability, and where does it break?', false, 1),
    ('correctness', '正确性验证', 'Correctness Verification',
     '输出是否正确',
     'Is the output correct?', false, 2),
    ('robustness', '稳定性验证', 'Robustness Verification',
     '无关改写、顺序变化、干扰内容或长度变化下是否保持正确',
     'Does the answer stay correct under paraphrasing, reordering, distractors, or length changes?', false, 3),
    ('compliance', '遵循性验证', 'Compliance Verification',
     '是否严格遵守显式约束',
     'Does the output satisfy every explicit constraint?', false, 4),
    ('safety', '安全验证', 'Safety Verification',
     '敏感内容处理是否符合政策',
     'Does sensitive-content handling meet policy?', false, 5),
    ('honesty', '诚实性验证', 'Honesty Verification',
     '是否避免编造并准确表达不确定性',
     'Does the model avoid fabrication and express uncertainty accurately?', false, 6),
    ('adversarial', '对抗探测', 'Adversarial Probing',
     '是否抵抗刻意操纵',
     'Does the model resist deliberate manipulation?', true, 7)
ON CONFLICT (slug) DO NOTHING;

-- 旧类别承载的是考察目的；旧能力承载的是考察维度。旧类别同时给出维度兜底，
-- 使从未填写能力维度的历史提示词也能得到确定的维度取值。
WITH purpose_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'capability_boundary'),
        ('math', 'correctness'),
        ('code_generation', 'correctness'),
        ('long_context', 'capability_boundary'),
        ('instruction_following', 'compliance'),
        ('knowledge_boundary', 'honesty'),
        ('hallucination', 'honesty'),
        ('safety', 'safety'),
        ('jailbreak', 'adversarial'),
        ('multi_turn', 'capability_boundary'),
        ('tool_use', 'correctness'),
        ('chinese', 'capability_boundary'),
        ('multilingual', 'capability_boundary')
),
capability_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'reasoning'),
        ('knowledge', 'knowledge'),
        ('math', 'math'),
        ('math_calculation', 'math'),
        ('language', 'language'),
        ('creativity', 'creativity'),
        ('coding', 'coding'),
        ('code_generation', 'coding'),
        ('instruction_following', 'instruction_following'),
        ('long_context', 'long_context'),
        ('tool_use', 'tool_use'),
        ('multi_turn', 'multi_turn'),
        ('safety', 'safety'),
        ('safety_alignment', 'safety'),
        ('jailbreak', 'safety'),
        ('hallucination', 'hallucination_resistance'),
        ('knowledge_boundary', 'knowledge'),
        ('chinese', 'language'),
        ('multilingual', 'language')
),
classified_prompts AS (
    SELECT
        p.id,
        pu.id AS purpose_id,
        COALESCE(cap.id, cat_cap.id) AS capability_id
    FROM prompts p
    JOIN categories c ON c.id = p.category_id
    JOIN purpose_map pm ON pm.old_slug = c.slug
    JOIN purposes pu ON pu.slug = pm.new_slug
    LEFT JOIN abilities a ON a.id = p.ability_id
    LEFT JOIN capability_map am ON am.old_slug = a.slug
    LEFT JOIN capabilities cap ON cap.slug = am.new_slug
    LEFT JOIN capability_map cm ON cm.old_slug = c.slug
    LEFT JOIN capabilities cat_cap ON cat_cap.slug = cm.new_slug
)
UPDATE prompts p
SET purpose_id = m.purpose_id,
    capability_id = m.capability_id
FROM classified_prompts m
WHERE p.id = m.id;

WITH purpose_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'capability_boundary'),
        ('math', 'correctness'),
        ('code_generation', 'correctness'),
        ('long_context', 'capability_boundary'),
        ('instruction_following', 'compliance'),
        ('knowledge_boundary', 'honesty'),
        ('hallucination', 'honesty'),
        ('safety', 'safety'),
        ('jailbreak', 'adversarial'),
        ('multi_turn', 'capability_boundary'),
        ('tool_use', 'correctness'),
        ('chinese', 'capability_boundary'),
        ('multilingual', 'capability_boundary')
),
capability_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'reasoning'),
        ('knowledge', 'knowledge'),
        ('math', 'math'),
        ('math_calculation', 'math'),
        ('language', 'language'),
        ('creativity', 'creativity'),
        ('coding', 'coding'),
        ('code_generation', 'coding'),
        ('instruction_following', 'instruction_following'),
        ('long_context', 'long_context'),
        ('tool_use', 'tool_use'),
        ('multi_turn', 'multi_turn'),
        ('safety', 'safety'),
        ('safety_alignment', 'safety'),
        ('jailbreak', 'safety'),
        ('hallucination', 'hallucination_resistance'),
        ('knowledge_boundary', 'knowledge'),
        ('chinese', 'language'),
        ('multilingual', 'language')
),
classified_versions AS (
    SELECT
        v.id,
        pu.id AS purpose_id,
        COALESCE(cap.id, cat_cap.id) AS capability_id
    FROM prompt_versions v
    JOIN categories c ON c.id = v.category_id
    JOIN purpose_map pm ON pm.old_slug = c.slug
    JOIN purposes pu ON pu.slug = pm.new_slug
    LEFT JOIN abilities a ON a.id = v.ability_id
    LEFT JOIN capability_map am ON am.old_slug = a.slug
    LEFT JOIN capabilities cap ON cap.slug = am.new_slug
    LEFT JOIN capability_map cm ON cm.old_slug = c.slug
    LEFT JOIN capabilities cat_cap ON cat_cap.slug = cm.new_slug
)
UPDATE prompt_versions v
SET purpose_id = m.purpose_id,
    capability_id = m.capability_id
FROM classified_versions m
WHERE v.id = m.id;

WITH purpose_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'capability_boundary'),
        ('math', 'correctness'),
        ('code_generation', 'correctness'),
        ('long_context', 'capability_boundary'),
        ('instruction_following', 'compliance'),
        ('knowledge_boundary', 'honesty'),
        ('hallucination', 'honesty'),
        ('safety', 'safety'),
        ('jailbreak', 'adversarial'),
        ('multi_turn', 'capability_boundary'),
        ('tool_use', 'correctness'),
        ('chinese', 'capability_boundary'),
        ('multilingual', 'capability_boundary')
),
capability_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'reasoning'),
        ('knowledge', 'knowledge'),
        ('math', 'math'),
        ('math_calculation', 'math'),
        ('language', 'language'),
        ('creativity', 'creativity'),
        ('coding', 'coding'),
        ('code_generation', 'coding'),
        ('instruction_following', 'instruction_following'),
        ('long_context', 'long_context'),
        ('tool_use', 'tool_use'),
        ('multi_turn', 'multi_turn'),
        ('safety', 'safety'),
        ('safety_alignment', 'safety'),
        ('jailbreak', 'safety'),
        ('hallucination', 'hallucination_resistance'),
        ('knowledge_boundary', 'knowledge'),
        ('chinese', 'language'),
        ('multilingual', 'language')
),
classified_questions AS (
    SELECT
        q.id,
        pu.id AS purpose_id,
        COALESCE(cap.id, cat_cap.id) AS capability_id
    FROM library_questions q
    JOIN categories c ON c.id = q.category_id
    JOIN purpose_map pm ON pm.old_slug = c.slug
    JOIN purposes pu ON pu.slug = pm.new_slug
    LEFT JOIN abilities a ON a.id = q.ability_id
    LEFT JOIN capability_map am ON am.old_slug = a.slug
    LEFT JOIN capabilities cap ON cap.slug = am.new_slug
    LEFT JOIN capability_map cm ON cm.old_slug = c.slug
    LEFT JOIN capabilities cat_cap ON cat_cap.slug = cm.new_slug
)
UPDATE library_questions q
SET purpose_id = m.purpose_id,
    capability_id = m.capability_id
FROM classified_questions m
WHERE q.id = m.id;

-- 题目允许没有考察目的：只填了旧能力的行仍补出考察维度。
WITH capability_map (old_slug, new_slug) AS (
    VALUES
        ('reasoning', 'reasoning'),
        ('knowledge', 'knowledge'),
        ('math', 'math'),
        ('math_calculation', 'math'),
        ('language', 'language'),
        ('creativity', 'creativity'),
        ('coding', 'coding'),
        ('code_generation', 'coding'),
        ('instruction_following', 'instruction_following'),
        ('long_context', 'long_context'),
        ('tool_use', 'tool_use'),
        ('multi_turn', 'multi_turn'),
        ('safety', 'safety'),
        ('safety_alignment', 'safety'),
        ('jailbreak', 'safety'),
        ('hallucination', 'hallucination_resistance'),
        ('knowledge_boundary', 'knowledge'),
        ('chinese', 'language'),
        ('multilingual', 'language')
)
UPDATE library_questions q
SET capability_id = cap.id
FROM abilities a
JOIN capability_map am ON am.old_slug = a.slug
JOIN capabilities cap ON cap.slug = am.new_slug
WHERE q.ability_id = a.id AND q.capability_id IS NULL;

-- 旧模型能力标签按新字典的 slug、中文名或英文名映射；无法识别的自由文本不写入。
INSERT INTO model_capabilities (model_id, capability_id)
SELECT DISTINCT m.id, c.id
FROM models m
CROSS JOIN LATERAL unnest(m.ability_tags) AS tag (value)
JOIN capabilities c
  ON c.slug = tag.value OR c.name_zh = tag.value OR c.name_en = tag.value
ON CONFLICT DO NOTHING;
