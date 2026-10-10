-- 清理不再使用的遗留表：迁移时一次性留底的归档/备份表，以及已被新机制取代的旧审计表。
-- 这些表没有生产代码引用，删除前已导出结构与数据样本留档。
DROP TABLE IF EXISTS groups_video_price_backup_245;
DROP TABLE IF EXISTS platform_independent_group_archive;
DROP TABLE IF EXISTS platform_independent_pricing_archive;
DROP TABLE IF EXISTS platform_independent_setting_archive;
DROP TABLE IF EXISTS pricing_policy_migration_archive;
DROP TABLE IF EXISTS removed_platform_quota_archive;
DROP TABLE IF EXISTS billing_usage_entries;
DROP TABLE IF EXISTS orphan_allowed_groups_audit;
DROP TABLE IF EXISTS auth_identity_migration_reports;
