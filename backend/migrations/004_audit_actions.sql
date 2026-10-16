-- 004_audit_actions.sql
-- 操作日志新增两个审计动作：
--   sensitive_content_access —— 高风险类别内容的每次访问（G-12）；
--   account_delete           —— 用户的账号注销（G-09）。
-- 002 建表时的匿名 CHECK 约束在此以同名约束重建，取值集与 oplog 包的常量保持一致。

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
    'sensitive_content_access', 'account_delete'));