-- 统计域的读写：prompt_stats、prompt_model_stats、recommend_tags。
--
-- 时间窗口边界一律由应用层按 UTC 自然日计算后作为参数传入（C-03、开发方案 §3.4）：
-- SQL 中不出现 date_trunc 与任何依赖会话时区的表达式。
--
-- COALESCE 纪律（开发方案 §6.4 专项回归）：本文件的每一个聚合都被 COALESCE 包住。
-- sqlc 的空值分析器不读 FILTER 子句的语义，裸的 count(*) FILTER (WHERE ...) 与
-- avg(...) FILTER (WHERE ...) 会被映射为非空类型，而「模型有历史评分但近 N 日无评分」
-- 这一必然形态下 PostgreSQL 对它们返回 NULL，扫描即以
-- "cannot scan NULL into *int64" 失败。

-- 某提示词在一个时间窗口内、按模型聚合的评分原始数据。
--
-- 这是全部统计的单一数据来源：完成度、标准差、区间半宽都由这四组聚合量算出，
-- 由 formula 包做纯计算。
--
-- 用 LEFT JOIN 从 models 出发而不是从 ratings 出发：后一种写法会直接丢掉无评分的
-- 模型，服务层就无法区分「该模型没有评分记录」与「该模型已不在启用列表中」。
--
-- name: ListPromptModelAggregates :many
SELECT m.id AS model_id,
       m.name AS model_name,
       count(r.id)::bigint               AS rating_count,
       count(DISTINCT r.user_id)::bigint AS user_count,
       COALESCE(sum(r.score), 0)::float8 AS score_sum,
       COALESCE(sum(r.score::float8 * r.score), 0)::float8 AS score_square
FROM models m
LEFT JOIN ratings r
       ON r.model_id = m.id
      AND r.prompt_id = sqlc.arg(prompt_id)
      AND r.deleted_at IS NULL
      AND (
          sqlc.arg(window_start)::timestamptz IS NULL
          OR r.created_at >= sqlc.arg(window_start)::timestamptz
      )
      AND (
          sqlc.arg(window_end)::timestamptz IS NULL
          OR r.created_at < sqlc.arg(window_end)::timestamptz
      )
WHERE m.deleted_at IS NULL AND m.is_enabled
GROUP BY m.id, m.name
ORDER BY m.id;

-- 某提示词在一个时间窗口内的独立评分用户数。
--
-- 单个模型的用户集合互相重叠，跨模型去重后的真实值只能在这里取：
-- 服务层若把各模型的用户数相加，重复评分同一提示词的用户会被计多次，
-- 置信度因此被高估（C-05 的同源约束）。
--
-- name: CountPromptWindowUsers :one
SELECT count(DISTINCT r.user_id)::bigint
FROM ratings r
WHERE r.prompt_id = sqlc.arg(prompt_id)
  AND r.deleted_at IS NULL
  AND (
      sqlc.arg(window_start)::timestamptz IS NULL
      OR r.created_at >= sqlc.arg(window_start)::timestamptz
  )
  AND (
      sqlc.arg(window_end)::timestamptz IS NULL
      OR r.created_at < sqlc.arg(window_end)::timestamptz
  );

-- 区分度置信度的 Bootstrap 需要逐条评分样本，重抽样单元就是单条评分。
-- 一次取回两个窗口的样本，避免两次查询之间出现评分变动导致的口径不一致。
--
-- name: ListPromptWindowScores :many
SELECT r.model_id, r.score
FROM ratings r
WHERE r.prompt_id = sqlc.arg(prompt_id)
  AND r.deleted_at IS NULL
  AND (
      sqlc.arg(window_start)::timestamptz IS NULL
      OR r.created_at >= sqlc.arg(window_start)::timestamptz
  )
  AND (
      sqlc.arg(window_end)::timestamptz IS NULL
      OR r.created_at < sqlc.arg(window_end)::timestamptz
  )
ORDER BY r.model_id, r.id;

-- 统计快照写入。两个窗口的字段在同一行内更新，写入由服务层放在单个事务中
-- （开发方案 §6.4：统计快照写入在单事务内，无半写状态）。
--
-- 两处区分度可空：无数据时写 NULL 而非 0。写 0 会被展示层读成「区分度为零」，
-- 与 C-06 要求的「数据不足，暂不计算区分度」矛盾。参数类型保持 numeric 的可空形态。
--
-- name: UpsertPromptStats :one
INSERT INTO prompt_stats (
    prompt_id, history_discrimination, history_confidence,
    history_valid_model_count, history_total_rating_count, history_insufficient,
    recent_discrimination, recent_confidence,
    recent_valid_model_count, recent_total_rating_count, recent_insufficient,
    data_sufficient, sort_discrimination, window_start, computed_at, updated_at
)
VALUES (
    sqlc.arg(prompt_id),
    sqlc.arg(history_discrimination)::numeric,
    sqlc.arg(history_confidence)::text,
    sqlc.arg(history_valid_model_count)::integer,
    sqlc.arg(history_total_rating_count)::integer,
    sqlc.arg(history_insufficient)::boolean,
    sqlc.arg(recent_discrimination)::numeric,
    sqlc.arg(recent_confidence)::text,
    sqlc.arg(recent_valid_model_count)::integer,
    sqlc.arg(recent_total_rating_count)::integer,
    sqlc.arg(recent_insufficient)::boolean,
    sqlc.arg(data_sufficient)::boolean,
    sqlc.arg(sort_discrimination)::numeric,
    sqlc.arg(window_start)::timestamptz,
    sqlc.arg(computed_at)::timestamptz,
    sqlc.arg(computed_at)::timestamptz
)
ON CONFLICT (prompt_id) DO UPDATE SET
    history_discrimination     = EXCLUDED.history_discrimination,
    history_confidence         = EXCLUDED.history_confidence,
    history_valid_model_count  = EXCLUDED.history_valid_model_count,
    history_total_rating_count = EXCLUDED.history_total_rating_count,
    history_insufficient       = EXCLUDED.history_insufficient,
    recent_discrimination      = EXCLUDED.recent_discrimination,
    recent_confidence          = EXCLUDED.recent_confidence,
    recent_valid_model_count   = EXCLUDED.recent_valid_model_count,
    recent_total_rating_count  = EXCLUDED.recent_total_rating_count,
    recent_insufficient        = EXCLUDED.recent_insufficient,
    data_sufficient            = EXCLUDED.data_sufficient,
    sort_discrimination        = EXCLUDED.sort_discrimination,
    window_start               = EXCLUDED.window_start,
    computed_at                = EXCLUDED.computed_at,
    updated_at                 = EXCLUDED.updated_at
RETURNING *;

-- name: UpsertPromptModelStat :one
INSERT INTO prompt_model_stats (
    prompt_id, model_id, prompt_version_id,
    history_mean, history_adjusted, history_rating_count, history_user_count,
    history_sd, history_ci_half, history_confidence_score, history_confidence_level,
    recent_mean, recent_adjusted, recent_rating_count, recent_user_count,
    recent_sd, recent_ci_half, recent_confidence_score, recent_confidence_level,
    computed_at, updated_at
)
VALUES (
    sqlc.arg(prompt_id),
    sqlc.arg(model_id),
    sqlc.arg(prompt_version_id),
    sqlc.arg(history_mean)::numeric,
    sqlc.arg(history_adjusted)::numeric,
    sqlc.arg(history_rating_count)::integer,
    sqlc.arg(history_user_count)::integer,
    sqlc.arg(history_sd)::numeric,
    sqlc.arg(history_ci_half)::numeric,
    sqlc.arg(history_confidence_score)::numeric,
    sqlc.arg(history_confidence_level)::text,
    sqlc.arg(recent_mean)::numeric,
    sqlc.arg(recent_adjusted)::numeric,
    sqlc.arg(recent_rating_count)::integer,
    sqlc.arg(recent_user_count)::integer,
    sqlc.arg(recent_sd)::numeric,
    sqlc.arg(recent_ci_half)::numeric,
    sqlc.arg(recent_confidence_score)::numeric,
    sqlc.arg(recent_confidence_level)::text,
    sqlc.arg(computed_at)::timestamptz,
    sqlc.arg(computed_at)::timestamptz
)
ON CONFLICT (prompt_id, model_id) DO UPDATE SET
    prompt_version_id         = EXCLUDED.prompt_version_id,
    history_mean               = EXCLUDED.history_mean,
    history_adjusted           = EXCLUDED.history_adjusted,
    history_rating_count       = EXCLUDED.history_rating_count,
    history_user_count         = EXCLUDED.history_user_count,
    history_sd                 = EXCLUDED.history_sd,
    history_ci_half            = EXCLUDED.history_ci_half,
    history_confidence_score   = EXCLUDED.history_confidence_score,
    history_confidence_level   = EXCLUDED.history_confidence_level,
    recent_mean                = EXCLUDED.recent_mean,
    recent_adjusted            = EXCLUDED.recent_adjusted,
    recent_rating_count        = EXCLUDED.recent_rating_count,
    recent_user_count          = EXCLUDED.recent_user_count,
    recent_sd                  = EXCLUDED.recent_sd,
    recent_ci_half             = EXCLUDED.recent_ci_half,
    recent_confidence_score    = EXCLUDED.recent_confidence_score,
    recent_confidence_level    = EXCLUDED.recent_confidence_level,
    computed_at                = EXCLUDED.computed_at,
    updated_at                 = EXCLUDED.updated_at
RETURNING *;

-- 删除某提示词下已不在启用模型列表中的统计行，避免快照无限增长。
-- name: DeletePromptModelStatsExcept :exec
DELETE FROM prompt_model_stats
WHERE prompt_id = sqlc.arg(prompt_id)
  AND NOT (model_id = ANY(sqlc.arg(model_ids)::bigint[]));

-- 快照是否过期：取该提示词自身与它全部评分的最后更新时刻。
--
-- 评分写入不触发统计重算，而是在下一次读取时由这一个比较判定过期，
-- 使评分提交保持单次写入（P95 指标，G-04），统计计算退化为读放大。
--
-- name: GetPromptStatsWatermark :one
SELECT GREATEST(
    COALESCE((SELECT p.updated_at FROM prompts p WHERE p.id = sqlc.arg(prompt_id)), now()),
    COALESCE((SELECT max(r.updated_at) FROM ratings r
              WHERE r.prompt_id = sqlc.arg(prompt_id)), '-infinity'::timestamptz)
)::timestamptz AS watermark;

-- 快照重算的事务入口：锁定该提示词的评分写入，使快照与它反映的评分集合一致。
-- 评分提交路径在写评分前同样先取本锁，两侧因此互斥。
-- 锁键 = 命名空间 << 32 + 对象标识。命名空间：1 收藏夹、2 统计重算、3 题库。
-- name: LockPromptStatsRecompute :exec
SELECT pg_advisory_xact_lock((2::bigint << 32) + sqlc.arg(prompt_id)::bigint);

-- 统计快照记录的提示词当前版本，使快照与它反映的评分所属版本可对照（需求 §7.1）。
-- name: GetPromptStatsVersion :one
SELECT current_version_id
FROM prompts
WHERE id = sqlc.arg(prompt_id)::bigint AND deleted_at IS NULL;

-- name: GetPromptStats :one
SELECT * FROM prompt_stats WHERE prompt_id = $1;

-- name: ListPromptModelStats :many
SELECT * FROM prompt_model_stats
WHERE prompt_id = $1
ORDER BY model_id;