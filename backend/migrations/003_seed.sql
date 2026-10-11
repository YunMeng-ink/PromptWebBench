-- 003_seed.sql
-- 字典枚举（C-44）、配置项总表（C-45）与系统默认账户「沧海遗粟」（需求 §2.5）。
-- 语言无关的资源均带中英两列，键为英文 slug。

INSERT INTO categories (slug, name_zh, name_en, is_high_risk, sort_order) VALUES
    ('reasoning',              '逻辑推理',   'Logical Reasoning',       false,  1),
    ('math',                   '数学能力',   'Mathematical Ability',    false,  2),
    ('code_generation',        '代码生成',   'Code Generation',         false,  3),
    ('long_context',           '长文本理解', 'Long-Context Understanding', false, 4),
    ('instruction_following',  '指令遵循',   'Instruction Following',   false,  5),
    ('knowledge_boundary',     '知识边界',   'Knowledge Boundary',      false,  6),
    ('hallucination',          '幻觉检测',   'Hallucination Detection', false,  7),
    ('safety',                 '安全性',     'Safety',                  false,  8),
    ('jailbreak',              '越狱对抗',   'Jailbreak & Adversarial', true,   9),
    ('multi_turn',             '多轮对话',   'Multi-turn Dialogue',      false, 10),
    ('tool_use',               '工具调用',   'Tool Use',                 false, 11),
    ('chinese',                '中文理解',   'Chinese Understanding',    false, 12),
    ('multilingual',           '多语言能力', 'Multilingual Ability',     false, 13)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO abilities (slug, name_zh, name_en, sort_order) VALUES
    ('reasoning',             '推理能力',   'Reasoning',                  1),
    ('knowledge',             '知识掌握',   'Knowledge',                  2),
    ('coding',                '代码能力',   'Coding',                     3),
    ('language',              '语言理解',   'Language Understanding',     4),
    ('instruction_following', '指令遵循',   'Instruction Following',      5),
    ('safety_alignment',      '安全对齐',   'Safety & Alignment',         6),
    ('creativity',            '创造性',     'Creativity',                 7),
    ('math_calculation',      '数学计算',   'Mathematical Calculation',   8)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO difficulties (slug, name_zh, name_en, level) VALUES
    ('easy',   '简单', 'Easy',   1),
    ('medium', '中等', 'Medium', 2),
    ('hard',   '困难', 'Hard',   3)
ON CONFLICT (slug) DO NOTHING;

-- 配置项种子（C-45 的 26 键）。值为 JSONB，键名 snake_case。
INSERT INTO config (key, value, description) VALUES
    ('bayes_prior',                     '3'::jsonb,     '贝叶斯先验分'),
    ('bayes_k',                         '5'::jsonb,     '贝叶斯先验权重'),
    ('saturation_threshold',            '4.5'::jsonb,   '训练饱和阈值'),
    ('obsolete_discrimination_threshold','2.0'::jsonb,  '过时区分度阈值'),
    ('discrim_min_models',              '3'::jsonb,     '区分度有效模型数阈值'),
    ('discrim_min_ratings_per_model',   '3'::jsonb,     '每个模型最低评分数阈值'),
    ('discrim_min_total_ratings',       '10'::jsonb,    '区分度总评分数阈值'),
    ('discrim_model_count_target',      '4'::jsonb,     '数据充分系数中的模型数目标'),
    ('discrim_rating_count_target',     '20'::jsonb,    '数据充分系数中的评分数目标'),
    ('rating_max_edits_per_record',     '3'::jsonb,     '单条评分最多修改次数'),
    ('rating_daily_edit_limit',         '10'::jsonb,    '每用户每日最多修改评分次数'),
    ('rating_daily_delete_limit',       '5'::jsonb,     '每用户每日最多删除评分次数'),
    ('recycle_retention_days',          '14'::jsonb,    '回收站保留天数'),
    ('recent_window_days',              '7'::jsonb,     '近 N 日窗口天数'),
    ('private_prompt_quota_user',       '100'::jsonb,   '普通用户私有提示词上限'),
    ('private_prompt_quota_developer',  '200'::jsonb,   '开发者用户私有提示词上限'),
    ('library_quota_user',              '1'::jsonb,     '普通用户私有题库上限'),
    ('library_quota_developer',         '3'::jsonb,     '开发者用户私有题库上限'),
    ('library_question_limit',          '150'::jsonb,   '单个私有题库题目上限'),
    ('favorite_item_limit',             '300'::jsonb,   '单个收藏夹提示词上限'),
    ('favorite_count_limit_user',       '20'::jsonb,    '普通用户收藏夹数量上限'),
    ('favorite_count_limit_developer',  '30'::jsonb,    '开发者用户收藏夹数量上限'),
    ('bootstrap_iterations',            '500'::jsonb,   'Bootstrap 重复次数'),
    ('registration_policy',             '"open"'::jsonb, '注册策略：open / closed / invite_only'),
    ('email_domain_whitelist',          '[]'::jsonb,    '邮箱后缀白名单，空数组表示不限制'),
    ('collaborator_restore_allowed',    'true'::jsonb,  '协作者可否恢复题库回收站题目')
ON CONFLICT (key) DO NOTHING;

-- 系统默认账户「沧海遗粟」：承接被封禁用户的提示词，不参与注册流程与邮箱验证。
INSERT INTO users (username, email, role, status, is_system, email_verified_at)
SELECT '沧海遗粟', 'system@prompt-benchmark.local', 'user', 'normal', true, now()
WHERE NOT EXISTS (SELECT 1 FROM users WHERE is_system);
