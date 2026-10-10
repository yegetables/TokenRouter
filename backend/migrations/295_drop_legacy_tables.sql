-- 清理零引用的遗留表：迁移时一次性留底的归档/备份表，删除前已导出结构与数据样本留档。
DROP TABLE IF EXISTS groups_video_price_backup_245;
DROP TABLE IF EXISTS platform_independent_group_archive;
DROP TABLE IF EXISTS platform_independent_pricing_archive;
DROP TABLE IF EXISTS platform_independent_setting_archive;
DROP TABLE IF EXISTS pricing_policy_migration_archive;
DROP TABLE IF EXISTS removed_platform_quota_archive;
