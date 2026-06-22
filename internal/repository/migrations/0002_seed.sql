-- 默认时间规则种子（对每个用户由代码在注册时插入，这里仅占位说明）
-- 实际种子数据在 repository.SeedDefaultRules 中按 user_id 写入，避免迁移期无用户上下文。
-- 此文件保留以演示迁移机制：未来需要全局升级时在此追加 SQL。
SELECT 1;
