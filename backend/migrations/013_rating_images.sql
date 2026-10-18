-- 013_rating_images.sql
-- 模型结果展示区：评分附图（用户在打分时提交的模型输出截图，上传到外部图床，
-- 平台数据库只保存外链）与展示区条目（模型输出的文本内容）。

CREATE TABLE rating_images (
    id             BIGSERIAL PRIMARY KEY,
    rating_id      BIGINT      NOT NULL REFERENCES ratings (id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    prompt_id      BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    model_id       BIGINT      NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    url            TEXT        NOT NULL,
    filename       TEXT        NOT NULL DEFAULT '',
    deleted_at     TIMESTAMPTZ,
    deleted_by     BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX rating_images_rating_idx ON rating_images (rating_id) WHERE deleted_at IS NULL;
CREATE INDEX rating_images_prompt_model_idx ON rating_images (prompt_id, model_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX rating_images_model_idx ON rating_images (model_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX rating_images_user_idx ON rating_images (user_id, created_at DESC) WHERE deleted_at IS NULL;

CREATE TABLE rating_outputs (
    id             BIGSERIAL PRIMARY KEY,
    rating_id      BIGINT      NOT NULL REFERENCES ratings (id) ON DELETE CASCADE,
    user_id        BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    prompt_id      BIGINT      NOT NULL REFERENCES prompts (id) ON DELETE CASCADE,
    model_id       BIGINT      NOT NULL REFERENCES models (id) ON DELETE CASCADE,
    content        TEXT        NOT NULL,
    deleted_at     TIMESTAMPTZ,
    deleted_by     BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX rating_outputs_rating_idx ON rating_outputs (rating_id) WHERE deleted_at IS NULL;
CREATE INDEX rating_outputs_prompt_model_idx ON rating_outputs (prompt_id, model_id, created_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX rating_outputs_model_idx ON rating_outputs (model_id, created_at DESC) WHERE deleted_at IS NULL;

-- 管理员删除违规图片与模型输出是两类新的审计动作，
-- 002 建表时的 CHECK 约束在此以同名约束重建。
ALTER TABLE operation_logs DROP CONSTRAINT operation_logs_action_check;
ALTER TABLE operation_logs ADD CONSTRAINT operation_logs_action_check CHECK (action IN (
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
    'favorite_delete', 'discrimination_recovery',
    'sensitive_content_access', 'account_delete',
    'showcase_image_delete', 'showcase_output_delete'));
