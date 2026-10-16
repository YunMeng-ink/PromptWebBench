-- 004_audit_actions.sql（逆向）——还原为 002 建表时的取值集。
-- 若库中已存在 sensitive_content_access / account_delete 的日志行，还原会因 CHECK 约束失败，
-- 需要先删除这两类日志（操作日志只增不删的原则在逆向场景下让位于数据清理）。

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
    'favorite_delete', 'discrimination_recovery'));