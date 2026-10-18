-- 014_rating_output_unique.sql
-- 每条评分只保留一条活跃的模型输出：重复提交覆盖内容，管理员删除后可以重新提交。

CREATE UNIQUE INDEX rating_outputs_rating_uniq ON rating_outputs (rating_id) WHERE deleted_at IS NULL;
