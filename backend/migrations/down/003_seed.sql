-- 003_seed.sql（逆向）
-- 存在外键引用时删除种子数据会失败，需要先清理业务数据（开发方案 §8.4）。
DELETE FROM config;
DELETE FROM users WHERE is_system AND username = '沧海遗粟';
DELETE FROM difficulties;
DELETE FROM abilities;
DELETE FROM categories;
