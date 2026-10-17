-- 007_report_pending_unique.sql
-- 同一用户对同一对象只允许一条待处理举报：唯一索引兜底并发下的重复刷单
-- （服务层先查后插，并发时两笔都会插入）。

CREATE UNIQUE INDEX reports_pending_uniq
    ON reports (reporter_id, object_type, object_id)
    WHERE status = 'pending';
