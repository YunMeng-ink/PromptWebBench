-- 本文件由 cmd/schemagen 从 backend/migrations/*.sql 生成，禁止手工编辑。
-- 重新生成：在 backend 目录执行 make schema

-- 001_extensions.sql
-- 关键词检索依赖 pg_trgm。该扩展自 PostgreSQL 13 起为受信扩展，库所有者可直接创建；
-- 若运行账号不是库所有者，需要提前由超级用户创建，否则启动会在本迁移失败。
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 002_schema.sql
-- Prompt Benchmark 全部业务表。约定：
--   命名 snake_case；枚举一律 TEXT + CHECK；时间 timestamptz DEFAULT now()；
--   软删除用 deleted_at 配合部分唯一索引；快照走 JSONB；无触发器无存储过程。

-- ============================================================
-- 账户域
-- ============================================================

CREATE TABLE users (
    id                          BIGSERIAL PRIMARY KEY,
    username                    TEXT        NOT NULL,
    email                       TEXT        NOT NULL,
    password_hash               TEXT,
    role                        TEXT        NOT NULL DEFAULT 'user'
                                            CHECK (role IN ('user', 'developer', 'admin')),
    status                      TEXT        NOT NULL DEFAULT 'unverified'
                                            CHECK (status IN ('normal', 'unverified', 'banned')),
    is_system                   BOOLEAN     NOT NULL DEFAULT false,
    bio                         TEXT        NOT NULL DEFAULT '',
    avatar_url                  TEXT        NOT NULL DEFAULT '',
    token_version               INTEGER     NOT NULL DEFAULT 1,
    email_verified_at           TIMESTAMPTZ,
    last_read_announcement_at   TIMESTAMPTZ,
    banned_at                   TIMESTAMPTZ,
    banned_reason               TEXT        NOT NULL DEFAULT '',
    deleted_at                  TIMESTAMPTZ,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_active_uniq    ON users (lower(email))    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_username_active_uniq ON users (lower(username)) WHERE deleted_at IS NULL;
CREATE INDEX users_status_idx  ON users (status);
CREATE INDEX users_role_idx    ON users (role);
CREATE INDEX users_username_trgm_idx ON users USING gin (username gin_trgm_ops);
CREATE INDEX users_email_trgm_idx    ON users USING gin (email gin_trgm_ops);

CREATE TABLE email_verifications (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT      REFERENCES users (id) ON DELETE CASCADE,
    email       TEXT        NOT NULL,
    code        TEXT        NOT NULL,
    purpose     TEXT        NOT NULL CHECK (purpose IN ('register', 'reset_password')),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX email_verifications_lookup_idx ON email_verifications (email, purpose, created_at DESC);

CREATE TABLE oauth_accounts (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         TEXT        NOT NULL CHECK (provider IN ('github')),
    provider_user_id TEXT        NOT NULL,
    provider_login   TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_user_id)
);

CREATE INDEX oauth_accounts_user_idx ON oauth_accounts (user_id);

CREATE TABLE api_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   TEXT        NOT NULL UNIQUE,
    token_prefix TEXT        NOT NULL DEFAULT '',
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 每个用户同一时间只允许一个有效 Token
CREATE UNIQUE INDEX api_tokens_user_active_uniq ON api_tokens (user_id) WHERE revoked_at IS NULL;

CREATE TABLE invite_codes (
    id         BIGSERIAL PRIMARY KEY,
    code       TEXT        NOT NULL UNIQUE,
    kind       TEXT        NOT NULL CHECK (kind IN ('normal', 'developer')),
    created_by BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    expires_at TIMESTAMPTZ,
    max_uses   INTEGER     NOT NULL DEFAULT 1 CHECK (max_uses >= 0),
    used_count INTEGER     NOT NULL DEFAULT 0 CHECK (used_count >= 0),
    disabled   BOOLEAN     NOT NULL DEFAULT false,
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX invite_codes_kind_idx ON invite_codes (kind, disabled);

CREATE TABLE invite_code_usages (
    id             BIGSERIAL PRIMARY KEY,
    invite_code_id BIGINT      NOT NULL REFERENCES invite_codes (id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    used_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (invite_code_id, user_id)
);

-- ============================================================
-- 字典与模型域
-- ============================================================

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

CREATE TABLE difficulties (
    id         BIGSERIAL PRIMARY KEY,
    slug       TEXT        NOT NULL UNIQUE,
    name_zh    TEXT        NOT NULL,
    name_en    TEXT        NOT NULL,
    level      SMALLINT    NOT NULL CHECK (level BETWEEN 1 AND 3),
    is_enabled BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE models (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT        NOT NULL,
    model_key        TEXT        NOT NULL,
    provider_key     TEXT        NOT NULL DEFAULT '',
    provider_name    TEXT        NOT NULL DEFAULT '',
    input_modalities TEXT[]      NOT NULL DEFAULT '{}',
    aliases          TEXT[]      NOT NULL DEFAULT '{}',
    description      TEXT        NOT NULL DEFAULT '',
    release_date     DATE,
    context_length   BIGINT      CHECK (context_length IS NULL OR context_length >= 0),
    is_enabled       BOOLEAN     NOT NULL DEFAULT true,
    ability_tags     TEXT[]      NOT NULL DEFAULT '{}',
    note             TEXT        NOT NULL DEFAULT '',
    import_source    TEXT        NOT NULL DEFAULT 'manual'
                                 CHECK (import_source IN ('manual', 'models_dev')),
    deleted_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 重复判断基于「模型标识 + 模型厂商」
CREATE UNIQUE INDEX models_identity_active_uniq ON models (model_key, provider_key) WHERE deleted_at IS NULL;
CREATE INDEX models_provider_idx ON models (provider_key) WHERE deleted_at IS NULL;
CREATE INDEX models_name_trgm_idx ON models USING gin (name gin_trgm_ops);

CREATE TABLE model_snapshots (
    id         BIGSERIAL PRIMARY KEY,
    model_id   BIGINT      NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    source     TEXT        NOT NULL DEFAULT 'models_dev',
    raw        JSONB       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX model_snapshots_model_idx ON model_snapshots (model_id, created_at DESC);

CREATE TABLE model_import_logs (
    id          BIGSERIAL PRIMARY KEY,
    model_id    BIGINT      REFERENCES models (id) ON DELETE SET NULL,
    action      TEXT        NOT NULL CHECK (action IN ('import', 'overwrite', 'skip')),
    provider_key TEXT       NOT NULL DEFAULT '',
    model_key   TEXT        NOT NULL DEFAULT '',
    payload     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    operator_id BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX model_import_logs_created_idx ON model_import_logs (created_at DESC);

-- ============================================================
-- 提示词域
-- ============================================================

CREATE TABLE prompts (
    id                   BIGSERIAL PRIMARY KEY,
    title                TEXT        NOT NULL,
    body                 TEXT        NOT NULL,
    description          TEXT        NOT NULL DEFAULT '',
    category_id          BIGINT      NOT NULL REFERENCES categories (id),
    ability_id           BIGINT      REFERENCES abilities (id),
    difficulty_id        BIGINT      REFERENCES difficulties (id),
    visibility           TEXT        NOT NULL DEFAULT 'public'
                                     CHECK (visibility IN ('public', 'private', 'developer')),
    sensitive_level      TEXT        NOT NULL DEFAULT 'low'
                                     CHECK (sensitive_level IN ('low', 'medium', 'high')),
    uploader_id          BIGINT      NOT NULL REFERENCES users (id),
    original_uploader_id BIGINT      REFERENCES users (id),
    current_version_id   BIGINT,
    latest_version_no    TEXT        NOT NULL DEFAULT 'v1.0',
    major_modified       BOOLEAN     NOT NULL DEFAULT false,
    comment_count        INTEGER     NOT NULL DEFAULT 0 CHECK (comment_count >= 0),
    favorite_count       INTEGER     NOT NULL DEFAULT 0 CHECK (favorite_count >= 0),
    deleted_at           TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX prompts_created_idx        ON prompts (created_at DESC)  WHERE deleted_at IS NULL;
CREATE INDEX prompts_updated_idx        ON prompts (updated_at DESC)  WHERE deleted_at IS NULL;
CREATE INDEX prompts_category_idx       ON prompts (category_id)      WHERE deleted_at IS NULL;
CREATE INDEX prompts_visibility_idx     ON prompts (visibility)       WHERE deleted_at IS NULL;
CREATE INDEX prompts_uploader_idx       ON prompts (uploader_id)      WHERE deleted_at IS NULL;
CREATE INDEX prompts_title_trgm_idx     ON prompts USING gin (title gin_trgm_ops);

CREATE TABLE prompt_versions (
    id              BIGSERIAL PRIMARY KEY,
    prompt_id       BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    version_no      TEXT        NOT NULL,
    major           INTEGER     NOT NULL CHECK (major >= 1),
    minor           INTEGER     NOT NULL CHECK (minor >= 0),
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL,
    description     TEXT        NOT NULL DEFAULT '',
    category_id     BIGINT      NOT NULL REFERENCES categories (id),
    ability_id      BIGINT      REFERENCES abilities (id),
    difficulty_id   BIGINT      REFERENCES difficulties (id),
    visibility      TEXT        NOT NULL CHECK (visibility IN ('public', 'private', 'developer')),
    sensitive_level TEXT        NOT NULL CHECK (sensitive_level IN ('low', 'medium', 'high')),
    is_major        BOOLEAN     NOT NULL DEFAULT false,
    change_note     TEXT        NOT NULL DEFAULT '',
    author_id       BIGINT      NOT NULL REFERENCES users (id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (prompt_id, version_no)
);

CREATE INDEX prompt_versions_prompt_idx ON prompt_versions (prompt_id, major DESC, minor DESC);

ALTER TABLE prompts
    ADD CONSTRAINT prompts_current_version_fk
    FOREIGN KEY (current_version_id) REFERENCES prompt_versions (id) ON DELETE SET NULL;

-- 历史版本评分归档：重大修改时从 ratings 迁移而来，保留原始评分时间与创建时间
CREATE TABLE prompt_version_ratings (
    id                BIGSERIAL PRIMARY KEY,
    source_rating_id  BIGINT      NOT NULL,
    user_id           BIGINT      NOT NULL,
    prompt_id         BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    prompt_version_id BIGINT      NOT NULL REFERENCES prompt_versions (id) ON DELETE CASCADE,
    model_id          BIGINT      NOT NULL,
    score             SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 5),
    edit_count        INTEGER     NOT NULL DEFAULT 0 CHECK (edit_count >= 0),
    rated_at          TIMESTAMPTZ NOT NULL,
    archived_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    archive_reason    TEXT        NOT NULL DEFAULT 'major_modification'
);

CREATE INDEX prompt_version_ratings_version_idx ON prompt_version_ratings (prompt_version_id, model_id);
CREATE INDEX prompt_version_ratings_prompt_idx  ON prompt_version_ratings (prompt_id);

CREATE TABLE tags (
    id          BIGSERIAL PRIMARY KEY,
    slug        TEXT        NOT NULL UNIQUE,
    name_zh     TEXT        NOT NULL,
    name_en     TEXT        NOT NULL,
    color       TEXT        NOT NULL DEFAULT '#909399',
    description TEXT        NOT NULL DEFAULT '',
    priority    INTEGER     NOT NULL DEFAULT 0,
    is_enabled  BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE prompt_tags (
    id         BIGSERIAL PRIMARY KEY,
    prompt_id  BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    tag_id     BIGINT      NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (prompt_id, tag_id)
);

CREATE INDEX prompt_tags_tag_idx ON prompt_tags (tag_id);

-- ============================================================
-- 评分与互动域
-- ============================================================

CREATE TABLE ratings (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    prompt_id         BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    prompt_version_id BIGINT      NOT NULL REFERENCES prompt_versions (id) ON DELETE CASCADE,
    model_id          BIGINT      NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    score             SMALLINT    NOT NULL CHECK (score BETWEEN 1 AND 5),
    edit_count        INTEGER     NOT NULL DEFAULT 0 CHECK (edit_count >= 0),
    deleted_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 评分唯一性：同一用户对同一「提示词 × 模型」只允许一条活跃评分
CREATE UNIQUE INDEX ratings_active_uniq ON ratings (user_id, prompt_id, model_id) WHERE deleted_at IS NULL;
CREATE INDEX ratings_prompt_model_idx ON ratings (prompt_id, model_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX ratings_user_idx         ON ratings (user_id, created_at DESC)             WHERE deleted_at IS NULL;

CREATE TABLE rating_change_logs (
    id               BIGSERIAL PRIMARY KEY,
    rating_id        BIGINT      REFERENCES ratings (id) ON DELETE SET NULL,
    user_id          BIGINT      NOT NULL,
    prompt_id        BIGINT      NOT NULL,
    model_id         BIGINT      NOT NULL,
    action           TEXT        NOT NULL CHECK (action IN ('create', 'update', 'delete')),
    before_score     SMALLINT,
    after_score      SMALLINT,
    before_edit_count INTEGER,
    after_edit_count INTEGER,
    ip               TEXT        NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX rating_change_logs_user_day_idx ON rating_change_logs (user_id, action, created_at DESC);

CREATE TABLE comments (
    id          BIGSERIAL PRIMARY KEY,
    prompt_id   BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    parent_id   BIGINT      REFERENCES comments (id) ON DELETE CASCADE,
    body        TEXT        NOT NULL,
    like_count  INTEGER     NOT NULL DEFAULT 0 CHECK (like_count >= 0),
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX comments_prompt_idx ON comments (prompt_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX comments_parent_idx ON comments (parent_id) WHERE deleted_at IS NULL;

CREATE TABLE comment_likes (
    id         BIGSERIAL PRIMARY KEY,
    comment_id BIGINT      NOT NULL REFERENCES comments (id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (comment_id, user_id)
);

CREATE TABLE reports (
    id          BIGSERIAL PRIMARY KEY,
    reporter_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    object_type TEXT        NOT NULL CHECK (object_type IN ('prompt', 'comment')),
    object_id   BIGINT      NOT NULL,
    reason      TEXT        NOT NULL CHECK (reason IN (
                                'bad_content', 'illegal', 'porn', 'attack',
                                'privacy', 'spam', 'dangerous', 'other')),
    detail      TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'ignored', 'resolved')),
    handler_id  BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    handle_note TEXT        NOT NULL DEFAULT '',
    handled_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX reports_status_idx ON reports (status, created_at DESC);
CREATE INDEX reports_object_idx ON reports (object_type, object_id);

-- ============================================================
-- 统计域
-- ============================================================

CREATE TABLE prompt_stats (
    prompt_id                 BIGINT PRIMARY KEY REFERENCES prompts (id) ON DELETE CASCADE,
    history_discrimination    NUMERIC(4, 2),
    history_confidence        TEXT CHECK (history_confidence IN ('high', 'medium', 'low')),
    history_valid_model_count INTEGER     NOT NULL DEFAULT 0,
    history_total_rating_count INTEGER    NOT NULL DEFAULT 0,
    history_insufficient      BOOLEAN     NOT NULL DEFAULT true,
    recent_discrimination     NUMERIC(4, 2),
    recent_confidence         TEXT CHECK (recent_confidence IN ('high', 'medium', 'low')),
    recent_valid_model_count  INTEGER     NOT NULL DEFAULT 0,
    recent_total_rating_count INTEGER     NOT NULL DEFAULT 0,
    recent_insufficient       BOOLEAN     NOT NULL DEFAULT true,
    data_sufficient           BOOLEAN     NOT NULL DEFAULT false,
    sort_discrimination       NUMERIC(4, 2),
    window_start              TIMESTAMPTZ,
    computed_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 区分度排序：数据充足的排在前面，再按区分度降序
CREATE INDEX prompt_stats_sort_idx ON prompt_stats (data_sufficient DESC, sort_discrimination DESC NULLS LAST);

CREATE TABLE prompt_model_stats (
    prompt_id                     BIGINT  NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    model_id                      BIGINT  NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    prompt_version_id             BIGINT  NOT NULL,
    history_mean                  NUMERIC(4, 2),
    history_adjusted              NUMERIC(4, 2),
    history_rating_count          INTEGER NOT NULL DEFAULT 0,
    history_user_count            INTEGER NOT NULL DEFAULT 0,
    history_sd                    NUMERIC(6, 3) NOT NULL DEFAULT 0,
    history_ci_half               NUMERIC(6, 3) NOT NULL DEFAULT 0,
    history_confidence_score      NUMERIC(5, 4) NOT NULL DEFAULT 0,
    history_confidence_level      TEXT    NOT NULL CHECK (history_confidence_level IN ('high', 'medium', 'low')),
    recent_mean                   NUMERIC(4, 2),
    recent_adjusted               NUMERIC(4, 2),
    recent_rating_count           INTEGER NOT NULL DEFAULT 0,
    recent_user_count             INTEGER NOT NULL DEFAULT 0,
    recent_sd                     NUMERIC(6, 3) NOT NULL DEFAULT 0,
    recent_ci_half                NUMERIC(6, 3) NOT NULL DEFAULT 0,
    recent_confidence_score       NUMERIC(5, 4) NOT NULL DEFAULT 0,
    recent_confidence_level       TEXT    NOT NULL CHECK (recent_confidence_level IN ('high', 'medium', 'low')),
    computed_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (prompt_id, model_id)
);

CREATE INDEX prompt_model_stats_model_idx ON prompt_model_stats (model_id);

CREATE TABLE recommend_tags (
    id           BIGSERIAL PRIMARY KEY,
    prompt_id    BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    model_id     BIGINT      NOT NULL DEFAULT 0,
    scope        TEXT        NOT NULL CHECK (scope IN ('prompt', 'prompt_model')),
    kind         TEXT        NOT NULL CHECK (kind IN (
                                 'obsolete', 'suspected_obsolete',
                                 'saturation', 'suspected_saturation')),
    state        TEXT        NOT NULL DEFAULT 'pending'
                             CHECK (state IN ('pending', 'confirmed', 'ignored', 'expired', 'review')),
    label_zh     TEXT        NOT NULL,
    label_en     TEXT        NOT NULL,
    confidence   TEXT        NOT NULL CHECK (confidence IN ('high', 'medium', 'low')),
    reason       TEXT        NOT NULL DEFAULT '',
    snapshot     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by  BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 同一「提示词 × 模型 × 标签类型」同时只保留一条未终结的推荐标签
CREATE UNIQUE INDEX recommend_tags_live_uniq
    ON recommend_tags (prompt_id, model_id, kind)
    WHERE state IN ('pending', 'confirmed', 'review');

CREATE INDEX recommend_tags_state_idx ON recommend_tags (state, created_at DESC);

-- ============================================================
-- 社交与协作域
-- ============================================================

CREATE TABLE favorites (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    visibility  TEXT        NOT NULL DEFAULT 'private'
                            CHECK (visibility IN ('public', 'private')),
    item_count  INTEGER     NOT NULL DEFAULT 0 CHECK (item_count >= 0),
    deleted_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX favorites_user_idx ON favorites (user_id) WHERE deleted_at IS NULL;
CREATE INDEX favorites_public_idx ON favorites (updated_at DESC) WHERE deleted_at IS NULL AND visibility = 'public';

CREATE TABLE favorite_items (
    id          BIGSERIAL PRIMARY KEY,
    favorite_id BIGINT      NOT NULL REFERENCES favorites (id) ON DELETE CASCADE,
    prompt_id   BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (favorite_id, prompt_id)
);

CREATE INDEX favorite_items_prompt_idx ON favorite_items (prompt_id);

CREATE TABLE libraries (
    id             BIGSERIAL PRIMARY KEY,
    owner_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    question_count INTEGER     NOT NULL DEFAULT 0 CHECK (question_count >= 0),
    deleted_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX libraries_owner_idx ON libraries (owner_id) WHERE deleted_at IS NULL;

CREATE TABLE library_members (
    id         BIGSERIAL PRIMARY KEY,
    library_id BIGINT      NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role       TEXT        NOT NULL CHECK (role IN ('owner', 'collaborator')),
    invited_by BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (library_id, user_id)
);

CREATE INDEX library_members_user_idx ON library_members (user_id);

CREATE TABLE library_questions (
    id                      BIGSERIAL PRIMARY KEY,
    library_id              BIGINT      NOT NULL REFERENCES libraries (id) ON DELETE CASCADE,
    kind                    TEXT        NOT NULL CHECK (kind IN ('reference', 'derived')),
    source_prompt_id        BIGINT      REFERENCES prompts (id) ON DELETE SET NULL,
    source_prompt_version_id BIGINT,
    title                   TEXT,
    body                    TEXT,
    description             TEXT        NOT NULL DEFAULT '',
    category_id             BIGINT      REFERENCES categories (id),
    ability_id              BIGINT      REFERENCES abilities (id),
    difficulty_id           BIGINT      REFERENCES difficulties (id),
    sensitive_level         TEXT        CHECK (sensitive_level IN ('low', 'medium', 'high')),
    sort_order              INTEGER     NOT NULL DEFAULT 0,
    deleted_at              TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- 引用题读取来源提示词全文，不存副本；派生题自带全文
    CONSTRAINT library_questions_shape CHECK (
        (kind = 'reference' AND source_prompt_id IS NOT NULL AND title IS NULL AND body IS NULL)
        OR
        (kind = 'derived' AND title IS NOT NULL AND body IS NOT NULL)
    )
);

CREATE INDEX library_questions_library_idx ON library_questions (library_id, sort_order) WHERE deleted_at IS NULL;
CREATE INDEX library_questions_source_idx  ON library_questions (source_prompt_id);

CREATE TABLE library_question_tags (
    id          BIGSERIAL PRIMARY KEY,
    question_id BIGINT      NOT NULL REFERENCES library_questions (id) ON DELETE CASCADE,
    tag         TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (question_id, tag)
);

CREATE TABLE library_question_revisions (
    id         BIGSERIAL PRIMARY KEY,
    question_id BIGINT     NOT NULL REFERENCES library_questions (id) ON DELETE CASCADE,
    editor_id  BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    before     JSONB       NOT NULL,
    after      JSONB       NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX library_question_revisions_question_idx ON library_question_revisions (question_id, created_at DESC);

CREATE TABLE follows (
    id          BIGSERIAL PRIMARY KEY,
    follower_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    followee_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);

CREATE INDEX follows_followee_idx ON follows (followee_id);

-- ============================================================
-- 治理域
-- ============================================================

-- 完整版支持 5 类对象，初版启用 prompt / library / library_question 三类
CREATE TABLE recycle_bin (
    id          BIGSERIAL PRIMARY KEY,
    object_type TEXT        NOT NULL CHECK (object_type IN (
                                'prompt', 'comment', 'favorite', 'library', 'library_question')),
    object_id   BIGINT      NOT NULL,
    owner_id    BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    library_id  BIGINT,
    snapshot    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    deleted_by  BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    deleted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    restored_at TIMESTAMPTZ,
    purged_at   TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX recycle_bin_live_uniq
    ON recycle_bin (object_type, object_id)
    WHERE restored_at IS NULL AND purged_at IS NULL;
CREATE INDEX recycle_bin_expiry_idx ON recycle_bin (expires_at) WHERE restored_at IS NULL AND purged_at IS NULL;
CREATE INDEX recycle_bin_owner_idx  ON recycle_bin (owner_id)    WHERE restored_at IS NULL AND purged_at IS NULL;

CREATE TABLE transfer_logs (
    id           BIGSERIAL PRIMARY KEY,
    prompt_id    BIGINT      NOT NULL,
    from_user_id BIGINT      NOT NULL,
    to_user_id   BIGINT      NOT NULL,
    reason       TEXT        NOT NULL DEFAULT 'user_banned',
    operator_id  BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX transfer_logs_prompt_idx ON transfer_logs (prompt_id, created_at DESC);

CREATE TABLE operation_logs (
    id            BIGSERIAL PRIMARY KEY,
    operator_id   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    operator_name TEXT        NOT NULL DEFAULT '',
    action        TEXT        NOT NULL CHECK (action IN (
                                  'admin_login', 'prompt_delete', 'prompt_restore', 'prompt_purge',
                                  'prompt_tag_update', 'prompt_major_mark',
                                  'recommend_tag_confirm', 'recommend_tag_ignore',
                                  'user_ban', 'user_unban', 'user_transfer_prompts',
                                  'developer_grant', 'developer_revoke',
                                  'registration_policy_update', 'invite_code_generate',
                                  'model_import', 'model_overwrite',
                                  'report_handle', 'announcement_publish', 'announcement_withdraw',
                                  'library_question_delete', 'library_question_restore',
                                  'api_token_reset', 'config_update', 'email_test',
                                  'recycle_restore', 'recycle_purge',
                                  'rating_update', 'rating_delete', 'comment_delete',
                                  'favorite_delete', 'discrimination_recovery')),
    object_type   TEXT        NOT NULL DEFAULT '',
    object_id     BIGINT,
    before_data   JSONB,
    after_data    JSONB,
    note          TEXT        NOT NULL DEFAULT '',
    ip            TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX operation_logs_created_idx  ON operation_logs (created_at DESC);
CREATE INDEX operation_logs_operator_idx ON operation_logs (operator_id, created_at DESC);
CREATE INDEX operation_logs_action_idx   ON operation_logs (action, created_at DESC);

CREATE TABLE notifications (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL CHECK (kind IN (
                               'announcement', 'comment_reply',
                               'library_question_deleted', 'system')),
    title      TEXT        NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    link       TEXT        NOT NULL DEFAULT '',
    payload    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    read_at    TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE deleted_at IS NULL AND read_at IS NULL;

CREATE TABLE announcements (
    id           BIGSERIAL PRIMARY KEY,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL,
    published_by BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ,
    withdrawn_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX announcements_live_idx ON announcements (published_at DESC) WHERE withdrawn_at IS NULL;

CREATE TABLE config (
    key               TEXT PRIMARY KEY,
    value             JSONB,
    secret_ciphertext TEXT,
    is_secret         BOOLEAN     NOT NULL DEFAULT false,
    description       TEXT        NOT NULL DEFAULT '',
    updated_by        BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT config_value_shape CHECK (is_secret = (secret_ciphertext IS NOT NULL))
);

