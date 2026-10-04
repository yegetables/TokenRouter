-- 请求载荷捕获表（fork 专属，self_ 前缀与上游 schema 解耦，不改动任何上游表）。
-- 记录每条网关请求的请求头/请求体/响应头/响应体快照，供使用记录详情弹窗展示。
CREATE TABLE IF NOT EXISTS self_request_payloads (
    id BIGSERIAL PRIMARY KEY,
    -- 内部关联 ID，对应 usage_logs.request_id 去掉 "client:" 前缀后的值，
    -- 也等于 X-TokenRouter-Request-ID 响应头。
    client_request_id VARCHAR(64) NOT NULL UNIQUE,
    -- 请求头与响应头快照，JSON 对象，敏感头已脱敏。
    request_headers JSONB,
    response_headers JSONB,
    -- 请求体与响应体快照，UTF-8 文本（JSON 或 SSE），写入前截断。
    request_body TEXT,
    response_body TEXT,
    -- 各部分是否因超过上限被截断。
    request_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    response_body_truncated BOOLEAN NOT NULL DEFAULT FALSE,
    -- 客户端可见状态码与请求路径，便于弹窗在没有用量行时也能给出基本上下文。
    status_code INT NOT NULL DEFAULT 0,
    request_path VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 详情弹窗按关联 ID 精确查询，唯一索引已覆盖；清理按 created_at 扫描。
CREATE INDEX IF NOT EXISTS idx_self_request_payloads_created_at ON self_request_payloads (created_at);
