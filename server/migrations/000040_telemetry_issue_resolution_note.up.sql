-- 000040 telemetry_issue_resolution 加 note 列 —— 标记解决时的备注落库。
-- 背景: 管理端「标记问题已解决」对话框收集 note(备注)并随 resolve 请求提交,
-- 但 issueReq/实体/表结构均无该字段, 服务端静默丢弃, 前端契约 IssueResolution.note 永远为空。
ALTER TABLE telemetry_issue_resolution
    ADD COLUMN note VARCHAR(500) NOT NULL DEFAULT '' AFTER resolved_commit_id;
